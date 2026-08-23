// Package room implements the persistent agent chat room.
package room

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
	"unicode/utf8"
)

const (
	defaultSpeaker = "agent"
	defaultKind    = "say"
)

type Policy struct {
	MessageRuneLimit         int
	SpeakerRuneLimit         int
	KindRuneLimit            int
	CompactionThreshold      int
	CompactionRetainMessages int
	FeedDefaultLimit         int
	FeedMaxLimit             int
	MemberWindow             time.Duration
	ScannerInitialBytes      int
	ScannerMaxBytes          int
}

func (policy Policy) Validate() error {
	if policy.MessageRuneLimit <= 0 || policy.SpeakerRuneLimit <= 0 || policy.KindRuneLimit <= 0 || policy.CompactionThreshold <= 0 || policy.CompactionRetainMessages <= 0 || policy.FeedDefaultLimit <= 0 || policy.FeedMaxLimit <= 0 || policy.MemberWindow <= 0 || policy.ScannerInitialBytes <= 0 || policy.ScannerMaxBytes <= 0 {
		return errors.New("room policy values must be positive")
	}
	if policy.CompactionRetainMessages > policy.CompactionThreshold {
		return errors.New("room compaction retain exceeds threshold")
	}
	if policy.FeedDefaultLimit > policy.FeedMaxLimit {
		return errors.New("room feed default exceeds maximum")
	}
	if policy.ScannerInitialBytes > policy.ScannerMaxBytes {
		return errors.New("room scanner initial exceeds maximum")
	}
	return nil
}

type Message struct {
	ID        int     `json:"id"`
	Timestamp float64 `json:"ts"`
	Who       string  `json:"who"`
	Text      string  `json:"text"`
	Kind      string  `json:"kind"`
}
type Member struct {
	Who   string  `json:"who"`
	Last  float64 `json:"last"`
	Count int     `json:"n"`
}
type SayResult struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
	Message *Message `json:"message,omitempty"`
}

type Store struct {
	Directory string
	mutex     sync.Mutex
	now       func() time.Time
	policy    Policy
}

func New(directory string, policy Policy) (*Store, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("room directory required")
	}
	if operationError := policy.Validate(); operationError != nil {
		return nil, operationError
	}
	return &Store{Directory: directory, policy: policy}, nil
}
func (store *Store) Policy() Policy { return store.policy }
func (store *Store) Path() (string, error) {
	if operationError := os.MkdirAll(store.Directory, 0o700); operationError != nil {
		return "", operationError
	}
	return filepath.Join(store.Directory, "room.jsonl"), nil
}
func (store *Store) nowTime() time.Time {
	if store.now != nil {
		return store.now()
	}
	return time.Now()
}
func clean(text string, limit int) string {
	normalizedText := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(normalizedText) <= limit {
		return normalizedText
	}
	return string([]rune(normalizedText)[:limit])
}
func cleanWithDefault(text, defaultValue string, limit int) string {
	normalizedText := clean(text, limit)
	if normalizedText == "" {
		return defaultValue
	}
	return normalizedText
}
func (store *Store) readAllLocked() ([]Message, error) {
	path, operationError := store.Path()
	if operationError != nil {
		return nil, operationError
	}
	file, operationError := os.Open(path)
	if errors.Is(operationError, os.ErrNotExist) {
		return nil, nil
	}
	if operationError != nil {
		return nil, operationError
	}
	defer file.Close()
	messages := []Message{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, store.policy.ScannerInitialBytes), store.policy.ScannerMaxBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var message Message
		if json.Unmarshal([]byte(line), &message) == nil && strings.TrimSpace(message.Text) != "" {
			messages = append(messages, message)
		}
	}
	return messages, scanner.Err()
}
func nextID(messages []Message) int {
	nextIdentifier := 0
	for _, message := range messages {
		if message.ID > nextIdentifier {
			nextIdentifier = message.ID
		}
	}
	return nextIdentifier + 1
}
func (store *Store) compactLocked(messages []Message) error {
	if len(messages) < store.policy.CompactionThreshold {
		return nil
	}
	messages = messages[len(messages)-store.policy.CompactionRetainMessages:]
	path, operationError := store.Path()
	if operationError != nil {
		return operationError
	}
	var data strings.Builder
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	for _, message := range messages {
		if operationError := encoder.Encode(message); operationError != nil {
			return operationError
		}
	}
	return atomicfile.WritePrivate(path, []byte(data.String()))
}

func (store *Store) Say(who, text, kind string) SayResult {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	normalizedText := clean(text, store.policy.MessageRuneLimit)
	if normalizedText == "" {
		return SayResult{OK: false, Error: "empty message"}
	}
	messages, operationError := store.readAllLocked()
	if operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	if operationError = store.compactLocked(messages); operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	message := Message{ID: nextID(messages), Timestamp: float64(store.nowTime().UnixNano()) / float64(time.Second), Who: cleanWithDefault(who, defaultSpeaker, store.policy.SpeakerRuneLimit), Text: normalizedText, Kind: cleanWithDefault(kind, defaultKind, store.policy.KindRuneLimit)}
	path, operationError := store.Path()
	if operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	file, operationError := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if operationError = encoder.Encode(message); operationError != nil {
		_ = file.Close()
		return SayResult{OK: false, Error: operationError.Error()}
	}
	if operationError = file.Close(); operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	return SayResult{OK: true, Message: &message}
}
func (store *Store) Feed(since, limit int) ([]Message, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if since < 0 {
		since = 0
	}
	if limit <= 0 {
		limit = store.policy.FeedDefaultLimit
	}
	if limit > store.policy.FeedMaxLimit {
		limit = store.policy.FeedMaxLimit
	}
	messages, operationError := store.readAllLocked()
	if operationError != nil {
		return nil, operationError
	}
	output := []Message{}
	for _, message := range messages {
		if message.ID > since {
			output = append(output, message)
		}
	}
	if len(output) > limit {
		output = output[len(output)-limit:]
	}
	return output, nil
}
func (store *Store) FeedString(since, limit string) ([]Message, error) {
	sinceIndex, _ := strconv.Atoi(strings.TrimSpace(since))
	limitValue, operationError := strconv.Atoi(strings.TrimSpace(limit))
	if operationError != nil {
		limitValue = store.policy.FeedDefaultLimit
	}
	return store.Feed(sinceIndex, limitValue)
}
func (store *Store) Members() ([]Member, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	messages, operationError := store.readAllLocked()
	if operationError != nil {
		return nil, operationError
	}
	cutoff := float64(store.nowTime().Add(-store.policy.MemberWindow).UnixNano()) / float64(time.Second)
	seen := map[string]*Member{}
	for _, message := range messages {
		if message.Timestamp < cutoff || strings.TrimSpace(message.Who) == "" {
			continue
		}
		current := seen[message.Who]
		if current == nil {
			seen[message.Who] = &Member{Who: message.Who, Last: message.Timestamp, Count: 1}
		} else {
			current.Count++
			if message.Timestamp > current.Last {
				current.Last = message.Timestamp
			}
		}
	}
	output := make([]Member, 0, len(seen))
	for _, member := range seen {
		output = append(output, *member)
	}
	sort.Slice(output, func(leftIndex, rightIndex int) bool { return output[leftIndex].Last > output[rightIndex].Last })
	return output, nil
}
func (store *Store) Clear() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	path, operationError := store.Path()
	if operationError != nil {
		return operationError
	}
	if operationError = os.Remove(path); operationError != nil && !errors.Is(operationError, os.ErrNotExist) {
		return operationError
	}
	return nil
}
