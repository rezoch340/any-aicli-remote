// Package room implements the small persistent agent chat room used by Any AI CLI Remote.
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
	"unicode/utf8"

	"github.com/rezoch340/any-aicli-remote/backend/internal/compat"
)

const (
	// Limit matches the Python room.py one-line message cap.
	Limit = 240
	// Keep matches the Python room.py compaction threshold.
	Keep = 2000
)

// Message is one short line in room.jsonl.
type Message struct {
	ID        int     `json:"id"`
	Timestamp float64 `json:"ts"`
	Who       string  `json:"who"`
	Text      string  `json:"text"`
	Kind      string  `json:"kind"`
}

// Member summarizes a recent speaker.
type Member struct {
	Who   string  `json:"who"`
	Last  float64 `json:"last"`
	Count int     `json:"n"`
}

// SayResult mirrors the JSON returned by the old Python API.
type SayResult struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
	Message *Message `json:"message,omitempty"`
}

// Store persists the room as JSONL. It is safe for concurrent use in-process.
type Store struct {
	Directory string
	mutex     sync.Mutex
	now       func() time.Time
}

// New returns a Store rooted at dir. Empty dir uses DataDir().
func New(directory string) *Store { return &Store{Directory: directory} }

// DataDirectory returns the configured Any AI CLI Remote data directory.
func DataDirectory() (string, error) {
	base := compat.Environment("ANY_AI_CLI_REMOTE_DATA_DIR", "")
	if strings.TrimSpace(base) == "" {
		home, operationError := os.UserHomeDir()
		if operationError != nil {
			return "", operationError
		}
		base = filepath.Join(home, ".any-aicli-remote")
	}
	if operationError := os.MkdirAll(base, 0o755); operationError != nil {
		return "", operationError
	}
	return base, nil
}

// Path returns the room.jsonl path, creating the store directory.
func (store *Store) Path() (string, error) {
	directory := store.Directory
	var operationError error
	if strings.TrimSpace(directory) == "" {
		directory, operationError = DataDirectory()
		if operationError != nil {
			return "", operationError
		}
	} else if operationError = os.MkdirAll(directory, 0o755); operationError != nil {
		return "", operationError
	}
	return filepath.Join(directory, "room.jsonl"), nil
}

func (store *Store) nowTime() time.Time {
	if store.now != nil {
		return store.now()
	}
	return time.Now()
}

func clean(text string, limit int) string {
	normalizedText := strings.Join(strings.Fields(text), " ")
	if limit <= 0 || utf8.RuneCountInString(normalizedText) <= limit {
		return normalizedText
	}
	runes := []rune(normalizedText)
	return string(runes[:limit])
}

// Clean normalizes whitespace and truncates to Limit runes.
func Clean(text string) string { return clean(text, Limit) }

func cleanWithDefault(text, def string, limit int) string {
	normalizedText := clean(text, limit)
	if normalizedText == "" {
		return def
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

	out := make([]Message, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var message Message
		if operationError := json.Unmarshal([]byte(line), &message); operationError != nil || strings.TrimSpace(message.Text) == "" {
			continue
		}
		out = append(out, message)
	}
	return out, scanner.Err()
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
	if len(messages) < Keep {
		return nil
	}
	if len(messages) > Keep/2 {
		messages = messages[len(messages)-(Keep/2):]
	}
	path, operationError := store.Path()
	if operationError != nil {
		return operationError
	}
	temporaryPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".tmp"
	file, operationError := os.OpenFile(temporaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if operationError != nil {
		return operationError
	}
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	for _, old := range messages {
		if operationError := enc.Encode(old); operationError != nil {
			_ = file.Close()
			return operationError
		}
	}
	if operationError := file.Close(); operationError != nil {
		return operationError
	}
	return os.Rename(temporaryPath, path)
}

// Say appends one short message. Empty messages return OK=false like room.py.
func (store *Store) Say(who, text, kind string) SayResult {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	normalizedText := Clean(text)
	if normalizedText == "" {
		return SayResult{OK: false, Error: "empty message"}
	}
	speaker := cleanWithDefault(who, "agent", 32)
	messageKind := cleanWithDefault(kind, "say", 12)

	messages, operationError := store.readAllLocked()
	if operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	if operationError := store.compactLocked(messages); operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	message := Message{ID: nextID(messages), Timestamp: float64(store.nowTime().UnixNano()) / 1e9, Who: speaker, Text: normalizedText, Kind: messageKind}
	path, operationError := store.Path()
	if operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	file, operationError := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	if operationError := enc.Encode(message); operationError != nil {
		_ = file.Close()
		return SayResult{OK: false, Error: operationError.Error()}
	}
	if operationError := file.Close(); operationError != nil {
		return SayResult{OK: false, Error: operationError.Error()}
	}
	return SayResult{OK: true, Message: &message}
}

// Feed returns messages with id > since, capped to limit [1,500] and newest last.
func (store *Store) Feed(since, limit int) ([]Message, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if since < 0 {
		since = 0
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	messages, operationError := store.readAllLocked()
	if operationError != nil {
		return nil, operationError
	}
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.ID > since {
			out = append(out, message)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// FeedString accepts Python-like loose string query values.
func (store *Store) FeedString(since, limit string) ([]Message, error) {
	sinceIndex, _ := strconv.Atoi(strings.TrimSpace(since))
	limitValue, operationError := strconv.Atoi(strings.TrimSpace(limit))
	if operationError != nil {
		limitValue = 200
	}
	return store.Feed(sinceIndex, limitValue)
}

// Members returns speakers active within window, newest first.
func (store *Store) Members(window time.Duration) ([]Member, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if window <= 0 {
		window = 900 * time.Second
	}
	messages, operationError := store.readAllLocked()
	if operationError != nil {
		return nil, operationError
	}
	cutoff := float64(store.nowTime().Add(-window).UnixNano()) / 1e9
	seen := map[string]*Member{}
	for _, message := range messages {
		if message.Timestamp < cutoff || strings.TrimSpace(message.Who) == "" {
			continue
		}
		cur := seen[message.Who]
		if cur == nil {
			seen[message.Who] = &Member{Who: message.Who, Last: message.Timestamp, Count: 1}
			continue
		}
		cur.Count++
		if message.Timestamp > cur.Last {
			cur.Last = message.Timestamp
		}
	}
	out := make([]Member, 0, len(seen))
	for _, message := range seen {
		out = append(out, *message)
	}
	sort.Slice(out, func(leftIndex, rightIndex int) bool { return out[leftIndex].Last > out[rightIndex].Last })
	return out, nil
}

// Clear removes room.jsonl. Missing file is OK.
func (store *Store) Clear() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	path, operationError := store.Path()
	if operationError != nil {
		return operationError
	}
	if operationError := os.Remove(path); operationError != nil && !errors.Is(operationError, os.ErrNotExist) {
		return operationError
	}
	return nil
}

var defaultStore = New("")

// Say appends to the default Any AI CLI Remote room.
func Say(who, text, kind string) SayResult { return defaultStore.Say(who, text, kind) }

// Feed reads from the default Any AI CLI Remote room.
func Feed(since, limit int) ([]Message, error) { return defaultStore.Feed(since, limit) }

// Members reads active speakers from the default Any AI CLI Remote room.
func Members(window time.Duration) ([]Member, error) { return defaultStore.Members(window) }

// Clear removes the default Any AI CLI Remote room.
func Clear() error { return defaultStore.Clear() }
