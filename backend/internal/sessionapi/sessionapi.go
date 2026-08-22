// Package sessionapi implements the filesystem-backed session management API
// exposed by the original grok-remote Python server.
package sessionapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/grok-remote/grok-remote-app/backend/internal/history"
)

var (
	SessionRequiredError = errors.New("sessionId required")
	NotFoundError        = errors.New("session dir not found")
	TitleRequiredError   = errors.New("title required")
	BadRequestError      = errors.New("bad request")
)

// Service provides the non-HTTP session operations used by the server router.
type Service struct {
	Store                   *history.Store
	DataDirectory           string
	DefaultWorkingDirectory func() string

	mutex sync.Mutex
	now   func() time.Time
}

func New(store *history.Store, dataDirectory string, defaultWorkingDirectory func() string) *Service {
	if store == nil {
		store = history.NewStore("")
	}
	if strings.TrimSpace(dataDirectory) == "" {
		dataDirectory = defaultDataDirectory()
	}
	return &Service{Store: store, DataDirectory: dataDirectory, DefaultWorkingDirectory: defaultWorkingDirectory, now: time.Now}
}

type HistoryQuery struct {
	SessionID        string
	WorkingDirectory string
	Live             bool
	Limit            int
	SinceBytes       int64
	BeforeBytes      *int64
	MaxBytes         int64
	ChatOnly         bool
}

type HistoryResult struct {
	OK               bool            `json:"ok"`
	Error            string          `json:"error,omitempty"`
	SessionID        string          `json:"sessionId"`
	WorkingDirectory string          `json:"cwd"`
	Title            string          `json:"title"`
	Directory        string          `json:"dir"`
	Events           []history.Event `json:"events"`
	Meta             history.Meta    `json:"meta"`
	Count            int             `json:"count"`
}

func (service *Service) History(operationContext context.Context, query HistoryQuery) (HistoryResult, error) {
	if operationContext == nil {
		operationContext = context.Background()
	}
	if operationError := operationContext.Err(); operationError != nil {
		return HistoryResult{}, operationError
	}

	sessionID := strings.TrimSpace(query.SessionID)
	if sessionID == "" {
		return HistoryResult{}, SessionRequiredError
	}
	workingDirectory := service.workingDirectory(query.WorkingDirectory)
	directory, valid := service.store().FindSessionDirectory(sessionID, workingDirectory)
	if !valid {
		return HistoryResult{
			OK:               false,
			Error:            NotFoundError.Error(),
			SessionID:        sessionID,
			WorkingDirectory: workingDirectory,
			Events:           []history.Event{},
			Meta:             history.Meta{"has_more": false},
		}, NotFoundError
	}

	limit := query.Limit
	if limit == 0 {
		if query.Live {
			limit = 400
		} else {
			limit = 100
		}
	}
	limit = min(4000, max(20, limit))
	maxBytes := query.MaxBytes
	if maxBytes == 0 {
		switch {
		case query.Live:
			maxBytes = 512_000
		case query.BeforeBytes != nil:
			maxBytes = 1_200_000
		default:
			maxBytes = 400_000
		}
	}
	maxBytes = min(int64(12_000_000), max(int64(64_000), maxBytes))

	events, meta := history.ReadSessionUpdates(directory, history.ReadOptions{
		Limit:       limit,
		MaxBytes:    maxBytes,
		SinceBytes:  query.SinceBytes,
		Live:        query.Live,
		BeforeBytes: query.BeforeBytes,
		ChatOnly:    query.ChatOnly,
	})
	if operationError := operationContext.Err(); operationError != nil {
		return HistoryResult{}, operationError
	}
	if meta == nil {
		meta = history.Meta{}
	}
	info := history.ReadSessionInfo(directory)
	meta["resolvedSid"] = sessionID
	meta["resolvedDir"] = directory
	if info.WorkingDirectory != "" {
		meta["resolvedCwd"] = info.WorkingDirectory
	}
	return HistoryResult{
		OK:               true,
		SessionID:        sessionID,
		WorkingDirectory: workingDirectory,
		Title:            info.Title,
		Directory:        directory,
		Events:           events,
		Meta:             meta,
		Count:            len(events),
	}, nil
}

type TitleInfo struct {
	Title            string `json:"title"`
	WorkingDirectory string `json:"cwd"`
	Directory        string `json:"dir"`
	ModificationTime int64  `json:"mtime"`
	UpdatedAt        int64  `json:"updatedAt"`
}

type TitlesResult struct {
	OK     bool                 `json:"ok"`
	Titles map[string]TitleInfo `json:"titles"`
	Count  int                  `json:"count"`
}

func (service *Service) Titles(ids []string, workingDirectory string) (TitlesResult, error) {
	out := make(map[string]TitleInfo)
	workingDirectory = service.workingDirectory(workingDirectory)
	for itemIndex, raw := range ids {
		if itemIndex >= 250 {
			break
		}
		sessionID := strings.TrimSpace(raw)
		if sessionID == "" {
			continue
		}
		if _, exists := out[sessionID]; exists {
			continue
		}
		directory, valid := service.store().FindSessionDirectory(sessionID, workingDirectory)
		if !valid && workingDirectory != "" {
			directory, valid = service.store().FindSessionDirectory(sessionID, "")
		}
		if !valid {
			continue
		}
		info := history.ReadSessionInfo(directory)
		modificationTime := sessionMTimeMillis(directory)
		if info.Title == "" && info.WorkingDirectory == "" && modificationTime == 0 {
			continue
		}
		out[sessionID] = TitleInfo{
			Title:            info.Title,
			WorkingDirectory: info.WorkingDirectory,
			Directory:        directory,
			ModificationTime: modificationTime,
			UpdatedAt:        modificationTime,
		}
	}
	return TitlesResult{OK: true, Titles: out, Count: len(out)}, nil
}

type SignalsResult struct {
	OK                  bool           `json:"ok"`
	Error               string         `json:"error,omitempty"`
	SessionID           string         `json:"sessionId"`
	Directory           string         `json:"dir,omitempty"`
	ContextTokensUsed   any            `json:"contextTokensUsed"`
	ContextWindowTokens any            `json:"contextWindowTokens"`
	ContextWindowUsage  any            `json:"contextWindowUsage"`
	TurnCount           any            `json:"turnCount"`
	ToolCallCount       any            `json:"toolCallCount"`
	PrimaryModelID      any            `json:"primaryModelId"`
	Signals             map[string]any `json:"signals"`
}

func (service *Service) Signals(sessionID, workingDirectory string) (SignalsResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SignalsResult{}, SessionRequiredError
	}
	workingDirectory = service.workingDirectory(workingDirectory)
	directory, valid := service.store().FindSessionDirectory(sessionID, workingDirectory)
	if !valid {
		return SignalsResult{OK: false, Error: NotFoundError.Error(), SessionID: sessionID}, NotFoundError
	}

	signals, operationError := readSignals(filepath.Join(directory, "signals.json"))
	if operationError != nil {
		return SignalsResult{OK: false, Error: operationError.Error(), SessionID: sessionID}, operationError
	}
	used := signals["contextTokensUsed"]
	if used == nil {
		used = signals["context_tokens_used"]
	}
	window := signals["contextWindowTokens"]
	if window == nil {
		window = signals["context_window_tokens"]
	}
	usage := signals["contextWindowUsage"]
	if usage == nil && used != nil && truthy(window) {
		usedNumber, usedOK := numeric(used)
		windowNumber, windowOK := numeric(window)
		if usedOK && windowOK && windowNumber != 0 {
			usage = math.RoundToEven((100*usedNumber/windowNumber)*10) / 10
		}
	}
	primary := signals["primaryModelId"]
	if !truthy(primary) {
		primary = signals["primary_model_id"]
	}
	return SignalsResult{
		OK:                  true,
		SessionID:           sessionID,
		Directory:           directory,
		ContextTokensUsed:   used,
		ContextWindowTokens: window,
		ContextWindowUsage:  usage,
		TurnCount:           signals["turnCount"],
		ToolCallCount:       signals["toolCallCount"],
		PrimaryModelID:      primary,
		Signals:             signals,
	}, nil
}

type ArchivedResult struct {
	OK    bool     `json:"ok"`
	IDs   []string `json:"ids"`
	Count int      `json:"count"`
	Path  string   `json:"path,omitempty"`
}

type SetArchivedRequest struct {
	IDs       []string `json:"ids"`
	ID        string   `json:"id"`
	SessionID string   `json:"sessionId"`
	Archived  *bool    `json:"archived"`
}

func (service *Service) Archived() (ArchivedResult, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	ids, path, operationError := service.loadArchivedLocked()
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	return ArchivedResult{OK: true, IDs: ids, Count: len(ids), Path: path}, nil
}

func (service *Service) SetArchived(request SetArchivedRequest) (ArchivedResult, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	ids, _, operationError := service.loadArchivedLocked()
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	if request.IDs != nil {
		ids = cleanIDs(request.IDs)
	} else {
		rawID := request.ID
		if rawID == "" {
			rawID = request.SessionID
		}
		sessionID := strings.TrimSpace(rawID)
		if sessionID == "" {
			return ArchivedResult{}, fmt.Errorf("%w: ids[] or id required", BadRequestError)
		}
		set := make(map[string]struct{}, len(ids)+1)
		for _, identifier := range ids {
			set[identifier] = struct{}{}
		}
		want := false
		if request.Archived == nil {
			_, exists := set[sessionID]
			want = !exists
		} else {
			want = *request.Archived
		}
		if want {
			set[sessionID] = struct{}{}
		} else {
			delete(set, sessionID)
		}
		ids = ids[:0]
		for identifier := range set {
			ids = append(ids, identifier)
		}
		sort.Strings(ids)
	}
	ids, operationError = service.saveArchivedLocked(ids)
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	return ArchivedResult{OK: true, IDs: ids, Count: len(ids)}, nil
}

type RenameRequest struct {
	SessionID        string `json:"sessionId"`
	ID               string `json:"id"`
	Title            string `json:"title"`
	Name             string `json:"name"`
	WorkingDirectory string `json:"cwd"`
}

type RenameResult struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	SessionID string `json:"sessionId"`
	Title     string `json:"title,omitempty"`
	Previous  string `json:"previous"`
	Directory string `json:"dir,omitempty"`
}

func (service *Service) Rename(request RenameRequest) (RenameResult, error) {
	rawSessionID := request.SessionID
	if rawSessionID == "" {
		rawSessionID = request.ID
	}
	sessionID := strings.TrimSpace(rawSessionID)
	if sessionID == "" {
		return RenameResult{}, SessionRequiredError
	}
	rawTitle := request.Title
	if rawTitle == "" {
		rawTitle = request.Name
	}
	title := strings.TrimSpace(rawTitle)
	if title == "" {
		return RenameResult{}, TitleRequiredError
	}
	title = truncateTitle(title, 160)
	workingDirectory := service.workingDirectory(request.WorkingDirectory)
	directory, valid := service.store().FindSessionDirectory(sessionID, workingDirectory)
	if !valid {
		return RenameResult{OK: false, Error: NotFoundError.Error(), SessionID: sessionID}, NotFoundError
	}

	service.mutex.Lock()
	defer service.mutex.Unlock()
	path := filepath.Join(directory, "summary.json")
	summary := readSummary(path)
	previous := pythonString(firstTruthy(
		summary["remote_title"],
		summary["generated_title"],
		summary["session_summary"],
	))
	summary["remote_title"] = title
	summary["generated_title"] = title
	summary["session_summary"] = title
	summary["updated_at"] = formatPythonUTC(service.clock())
	data, operationError := marshalIndent(summary)
	if operationError != nil {
		return RenameResult{OK: false, Error: operationError.Error(), SessionID: sessionID}, operationError
	}
	if operationError := os.WriteFile(path, data, 0o644); operationError != nil {
		wrapped := fmt.Errorf("write summary: %w", operationError)
		return RenameResult{OK: false, Error: wrapped.Error(), SessionID: sessionID}, wrapped
	}
	return RenameResult{OK: true, SessionID: sessionID, Title: title, Previous: previous, Directory: directory}, nil
}

func (service *Service) store() *history.Store {
	if service != nil && service.Store != nil {
		return service.Store
	}
	return history.NewStore("")
}

func (service *Service) workingDirectory(value string) string {
	if value == "" && service != nil && service.DefaultWorkingDirectory != nil {
		value = service.DefaultWorkingDirectory()
	}
	return strings.TrimSpace(value)
}

func (service *Service) clock() time.Time {
	if service != nil && service.now != nil {
		return service.now()
	}
	return time.Now()
}

func defaultDataDirectory() string {
	if value := os.Getenv("GROK_PLUGIN_DATA"); value != "" {
		return value
	}
	home, operationError := os.UserHomeDir()
	if operationError != nil || home == "" {
		return filepath.Join(".grok", "plugin-data", "grok-remote")
	}
	return filepath.Join(home, ".grok", "plugin-data", "grok-remote")
}

func sessionMTimeMillis(directory string) int64 {
	if info, operationError := os.Stat(filepath.Join(directory, "updates.jsonl")); operationError == nil && !info.IsDir() {
		return info.ModTime().UnixMilli()
	}
	if info, operationError := os.Stat(filepath.Join(directory, "summary.json")); operationError == nil && !info.IsDir() {
		return info.ModTime().UnixMilli()
	}
	return 0
}

func readSignals(path string) (map[string]any, error) {
	info, operationError := os.Stat(path)
	if errors.Is(operationError, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if operationError != nil {
		return nil, operationError
	}
	if !info.Mode().IsRegular() {
		return map[string]any{}, nil
	}
	data, operationError := os.ReadFile(path)
	if operationError != nil {
		return nil, operationError
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if operationError := decoder.Decode(&value); operationError != nil {
		return nil, operationError
	}
	if value == nil || !truthy(value) {
		return map[string]any{}, nil
	}
	signals, valid := value.(map[string]any)
	if !valid {
		return nil, errors.New("signals must be a JSON object")
	}
	return signals, nil
}

func numeric(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		parsedNumber, operationError := value.Float64()
		return parsedNumber, operationError == nil
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case string:
		parsedNumber, operationError := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsedNumber, operationError == nil
	case bool:
		if value {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	switch value := value.(type) {
	case bool:
		return value
	case string:
		return value != ""
	case []any:
		return len(value) != 0
	case map[string]any:
		return len(value) != 0
	}
	if number, valid := numeric(value); valid {
		return number != 0
	}
	return true
}

func (service *Service) archivePath() (string, error) {
	dataDirectory := ""
	if service != nil {
		dataDirectory = service.DataDirectory
	}
	if strings.TrimSpace(dataDirectory) == "" {
		dataDirectory = defaultDataDirectory()
	}
	if operationError := os.MkdirAll(dataDirectory, 0o755); operationError != nil {
		return "", operationError
	}
	return filepath.Join(dataDirectory, "archived_sessions.json"), nil
}

func (service *Service) loadArchivedLocked() ([]string, string, error) {
	path, operationError := service.archivePath()
	if operationError != nil {
		return nil, "", operationError
	}
	data, operationError := os.ReadFile(path)
	if errors.Is(operationError, os.ErrNotExist) {
		return []string{}, path, nil
	}
	if operationError != nil {
		return []string{}, path, nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return []string{}, path, nil
	}
	var raw any
	switch value := value.(type) {
	case map[string]any:
		raw = value["ids"]
		if !truthy(raw) {
			raw = value["archived"]
		}
	case []any:
		raw = value
	}
	items, _ := raw.([]any)
	ids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		identifier := strings.TrimSpace(pythonString(item))
		if identifier == "" {
			continue
		}
		if _, exists := seen[identifier]; exists {
			continue
		}
		seen[identifier] = struct{}{}
		ids = append(ids, identifier)
	}
	return ids, path, nil
}

func (service *Service) saveArchivedLocked(ids []string) ([]string, error) {
	path, operationError := service.archivePath()
	if operationError != nil {
		return nil, operationError
	}
	ids = cleanIDs(ids)
	payload := struct {
		IDs       []string `json:"ids"`
		UpdatedAt float64  `json:"updatedAt"`
	}{IDs: ids, UpdatedAt: float64(service.clock().UnixNano()) / float64(time.Second)}
	data, operationError := json.MarshalIndent(payload, "", "  ")
	if operationError != nil {
		return nil, operationError
	}
	if operationError := os.WriteFile(path, data, 0o644); operationError != nil {
		return nil, operationError
	}
	return ids, nil
}

func cleanIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		identifier := strings.TrimSpace(raw)
		if identifier == "" {
			continue
		}
		if _, exists := seen[identifier]; exists {
			continue
		}
		seen[identifier] = struct{}{}
		out = append(out, identifier)
	}
	return out
}

func readSummary(path string) map[string]any {
	data, operationError := os.ReadFile(path)
	if operationError != nil {
		return map[string]any{}
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return map[string]any{}
	}
	if summary, valid := value.(map[string]any); valid {
		return summary
	}
	return map[string]any{}
}

func firstTruthy(values ...any) any {
	for _, value := range values {
		if truthy(value) {
			return value
		}
	}
	return nil
}

func pythonString(value any) string {
	if !truthy(value) {
		return ""
	}
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case bool:
		if value {
			return "True"
		}
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func truncateTitle(title string, limit int) string {
	runes := []rune(title)
	if len(runes) <= limit {
		return title
	}
	return strings.TrimRightFunc(string(runes[:limit]), unicode.IsSpace)
}

func formatPythonUTC(value time.Time) string {
	value = value.UTC()
	base := value.Format("2006-01-02T15:04:05")
	microseconds := value.Nanosecond() / 1_000
	if microseconds == 0 {
		return base + "Z"
	}
	return fmt.Sprintf("%s.%06dZ", base, microseconds)
}

func marshalIndent(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if operationError := encoder.Encode(value); operationError != nil {
		return nil, operationError
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
