// Package sessionapi exposes provider-neutral session management operations.
package sessionapi

import (
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

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

var (
	SessionRequiredError = errors.New("sessionId required")
	NotFoundError        = errors.New("session not found")
	TitleRequiredError   = errors.New("title required")
	BadRequestError      = errors.New("bad request")
)

type Service struct {
	Providers         *providerapi.Registry
	DefaultProviderID string
	DataDirectory     string
	historyPolicy     providerapi.HistoryPolicy

	mutex sync.Mutex
}

func New(providers *providerapi.Registry, defaultProviderID, dataDirectory string, historyPolicy providerapi.HistoryPolicy) (*Service, error) {
	if providers == nil {
		providers = providerapi.NewRegistry()
	}
	if operationError := historyPolicy.Validate(); operationError != nil {
		return nil, operationError
	}
	return &Service{Providers: providers, DefaultProviderID: defaultProviderID, DataDirectory: dataDirectory, historyPolicy: historyPolicy}, nil
}

func (service *Service) HistoryPolicy() providerapi.HistoryPolicy { return service.historyPolicy }

func (service *Service) provider(providerID string) (providerapi.Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = service.DefaultProviderID
	}
	return service.Providers.Provider(providerID)
}

type HistoryQuery struct {
	ProviderID  string
	SessionID   string
	Live        bool
	Limit       int
	SinceBytes  int64
	BeforeBytes *int64
	MaxBytes    int64
	ChatOnly    bool
}

type HistoryResult struct {
	OK               bool                        `json:"ok"`
	Error            string                      `json:"error,omitempty"`
	ProviderID       string                      `json:"providerId"`
	SessionID        string                      `json:"sessionId"`
	WorkingDirectory string                      `json:"cwd"`
	Title            string                      `json:"title"`
	Directory        string                      `json:"dir"`
	Events           []providerapi.Event         `json:"events"`
	Meta             providerapi.HistoryMetadata `json:"meta"`
	Count            int                         `json:"count"`
}

func (service *Service) History(operationContext context.Context, query HistoryQuery) (HistoryResult, error) {
	if operationContext == nil {
		operationContext = context.Background()
	}
	sessionID := strings.TrimSpace(query.SessionID)
	if sessionID == "" {
		return HistoryResult{}, SessionRequiredError
	}
	providerInstance, operationError := service.provider(query.ProviderID)
	if operationError != nil {
		return HistoryResult{}, operationError
	}
	limit, maxBytes := service.historyPolicy.NormalizeRequest(query.Live, query.BeforeBytes, query.Limit, query.MaxBytes)
	page, operationError := providerInstance.ReadHistory(operationContext, providerapi.HistoryQuery{
		SessionID: sessionID, Live: query.Live, Limit: limit, SinceBytes: query.SinceBytes,
		BeforeBytes: query.BeforeBytes, MaxBytes: maxBytes, ChatOnly: query.ChatOnly,
	})
	if operationError != nil {
		result := HistoryResult{OK: false, Error: NotFoundError.Error(), ProviderID: providerInstance.ID(), SessionID: sessionID, Events: []providerapi.Event{}, Meta: providerapi.HistoryMetadata{"has_more": false}}
		if errors.Is(operationError, providerapi.SessionNotFoundError) {
			return result, NotFoundError
		}
		return result, operationError
	}
	directory := ""
	if page.Session.SourcePath != "" {
		directory = filepath.Dir(page.Session.SourcePath)
	}
	if page.Events == nil {
		page.Events = []providerapi.Event{}
	}
	if page.Metadata == nil {
		page.Metadata = providerapi.HistoryMetadata{}
	}
	return HistoryResult{
		OK: true, ProviderID: page.Session.ProviderID, SessionID: page.Session.SessionID,
		WorkingDirectory: page.Session.ProjectDirectory, Title: page.Session.Title, Directory: directory,
		Events: page.Events, Meta: page.Metadata, Count: len(page.Events),
	}, nil
}

type TitleInfo struct {
	ProviderID       string `json:"providerId"`
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

func (service *Service) Titles(operationContext context.Context, providerID string, sessionIDs []string) (TitlesResult, error) {
	providerInstance, operationError := service.provider(providerID)
	if operationError != nil {
		return TitlesResult{}, operationError
	}
	titles := make(map[string]TitleInfo)
	for itemIndex, rawSessionID := range sessionIDs {
		if itemIndex >= service.historyPolicy.TitleBatchLimit {
			break
		}
		sessionID := strings.TrimSpace(rawSessionID)
		if sessionID == "" {
			continue
		}
		if _, exists := titles[sessionID]; exists {
			continue
		}
		metadata, resolutionError := providerInstance.ResolveSession(operationContext, sessionID)
		if resolutionError != nil {
			continue
		}
		directory := ""
		if metadata.SourcePath != "" {
			directory = filepath.Dir(metadata.SourcePath)
		}
		modified := metadata.LastActiveAt
		if modified == 0 {
			modified = metadata.CreatedAt
		}
		titles[sessionID] = TitleInfo{
			ProviderID: metadata.ProviderID, Title: metadata.Title,
			WorkingDirectory: metadata.ProjectDirectory, Directory: directory,
			ModificationTime: modified, UpdatedAt: modified,
		}
	}
	return TitlesResult{OK: true, Titles: titles, Count: len(titles)}, nil
}

type SignalsResult struct {
	OK                  bool           `json:"ok"`
	Error               string         `json:"error,omitempty"`
	ProviderID          string         `json:"providerId"`
	SessionID           string         `json:"sessionId"`
	ContextTokensUsed   any            `json:"contextTokensUsed"`
	ContextWindowTokens any            `json:"contextWindowTokens"`
	ContextWindowUsage  any            `json:"contextWindowUsage"`
	TurnCount           any            `json:"turnCount"`
	ToolCallCount       any            `json:"toolCallCount"`
	PrimaryModelID      any            `json:"primaryModelId"`
	Signals             map[string]any `json:"signals"`
}

func (service *Service) Signals(operationContext context.Context, providerID, sessionID string) (SignalsResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SignalsResult{}, SessionRequiredError
	}
	providerInstance, operationError := service.provider(providerID)
	if operationError != nil {
		return SignalsResult{}, operationError
	}
	signals, operationError := providerInstance.ReadSignals(operationContext, sessionID)
	if operationError != nil {
		result := SignalsResult{OK: false, Error: NotFoundError.Error(), ProviderID: providerInstance.ID(), SessionID: sessionID}
		if errors.Is(operationError, providerapi.SessionNotFoundError) {
			return result, NotFoundError
		}
		return result, operationError
	}
	used := firstTruthy(signals["contextTokensUsed"], signals["context_tokens_used"])
	window := firstTruthy(signals["contextWindowTokens"], signals["context_window_tokens"])
	usage := signals["contextWindowUsage"]
	if usage == nil {
		usedNumber, usedValid := numeric(used)
		windowNumber, windowValid := numeric(window)
		if usedValid && windowValid && windowNumber != 0 {
			usage = math.RoundToEven((100*usedNumber/windowNumber)*10) / 10
		}
	}
	primaryModelID := firstTruthy(signals["primaryModelId"], signals["primary_model_id"])
	return SignalsResult{
		OK: true, ProviderID: providerInstance.ID(), SessionID: sessionID,
		ContextTokensUsed: used, ContextWindowTokens: window, ContextWindowUsage: usage,
		TurnCount: signals["turnCount"], ToolCallCount: signals["toolCallCount"],
		PrimaryModelID: primaryModelID, Signals: signals,
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
	sessionIDs, path, operationError := service.loadArchivedLocked()
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	return ArchivedResult{OK: true, IDs: sessionIDs, Count: len(sessionIDs), Path: path}, nil
}

func (service *Service) SetArchived(request SetArchivedRequest) (ArchivedResult, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	sessionIDs, _, operationError := service.loadArchivedLocked()
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	if request.IDs != nil {
		sessionIDs = cleanIDs(request.IDs)
	} else {
		sessionID := strings.TrimSpace(firstNonEmpty(request.ID, request.SessionID))
		if sessionID == "" {
			return ArchivedResult{}, fmt.Errorf("%w: ids[] or id required", BadRequestError)
		}
		archivedSet := make(map[string]struct{}, len(sessionIDs)+1)
		for _, identifier := range sessionIDs {
			archivedSet[identifier] = struct{}{}
		}
		wantArchived := request.Archived == nil
		if request.Archived == nil {
			_, alreadyArchived := archivedSet[sessionID]
			wantArchived = !alreadyArchived
		} else {
			wantArchived = *request.Archived
		}
		if wantArchived {
			archivedSet[sessionID] = struct{}{}
		} else {
			delete(archivedSet, sessionID)
		}
		sessionIDs = sessionIDs[:0]
		for identifier := range archivedSet {
			sessionIDs = append(sessionIDs, identifier)
		}
		sort.Strings(sessionIDs)
	}
	sessionIDs, operationError = service.saveArchivedLocked(sessionIDs)
	if operationError != nil {
		return ArchivedResult{}, operationError
	}
	return ArchivedResult{OK: true, IDs: sessionIDs, Count: len(sessionIDs)}, nil
}

type RenameRequest struct {
	ProviderID       string `json:"providerId"`
	SessionID        string `json:"sessionId"`
	ID               string `json:"id"`
	Title            string `json:"title"`
	Name             string `json:"name"`
	WorkingDirectory string `json:"cwd,omitempty"`
}

type RenameResult struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	ProviderID string `json:"providerId"`
	SessionID  string `json:"sessionId"`
	Title      string `json:"title,omitempty"`
	Previous   string `json:"previous"`
	Directory  string `json:"dir,omitempty"`
}

func (service *Service) Rename(operationContext context.Context, request RenameRequest) (RenameResult, error) {
	sessionID := strings.TrimSpace(firstNonEmpty(request.SessionID, request.ID))
	if sessionID == "" {
		return RenameResult{}, SessionRequiredError
	}
	title := strings.TrimSpace(firstNonEmpty(request.Title, request.Name))
	if title == "" {
		return RenameResult{}, TitleRequiredError
	}
	providerInstance, operationError := service.provider(request.ProviderID)
	if operationError != nil {
		return RenameResult{}, operationError
	}
	providerResult, operationError := providerInstance.RenameSession(operationContext, sessionID, truncateRunes(title, service.historyPolicy.RenameTitleMaxRunes))
	if operationError != nil {
		result := RenameResult{OK: false, Error: NotFoundError.Error(), ProviderID: providerInstance.ID(), SessionID: sessionID}
		if errors.Is(operationError, providerapi.SessionNotFoundError) {
			return result, NotFoundError
		}
		return result, operationError
	}
	directory := ""
	if providerResult.SourcePath != "" {
		directory = filepath.Dir(providerResult.SourcePath)
	}
	return RenameResult{
		OK: true, ProviderID: providerInstance.ID(), SessionID: providerResult.SessionID,
		Title: providerResult.Title, Previous: providerResult.PreviousTitle, Directory: directory,
	}, nil
}

func (service *Service) archivedPath() string {
	return filepath.Join(service.DataDirectory, "archived_sessions.json")
}

func (service *Service) loadArchivedLocked() ([]string, string, error) {
	path := service.archivedPath()
	data, operationError := os.ReadFile(path)
	if errors.Is(operationError, os.ErrNotExist) {
		return []string{}, path, nil
	}
	if operationError != nil {
		return nil, path, operationError
	}
	var sessionIDs []string
	if operationError := json.Unmarshal(data, &sessionIDs); operationError != nil {
		return nil, path, operationError
	}
	return cleanIDs(sessionIDs), path, nil
}

func (service *Service) saveArchivedLocked(sessionIDs []string) ([]string, error) {
	sessionIDs = cleanIDs(sessionIDs)
	data, operationError := json.MarshalIndent(sessionIDs, "", "  ")
	if operationError != nil {
		return nil, operationError
	}
	if operationError := atomicfile.WritePrivate(service.archivedPath(), append(data, '\n')); operationError != nil {
		return nil, operationError
	}
	return sessionIDs, nil
}

func cleanIDs(sessionIDs []string) []string {
	seen := make(map[string]struct{}, len(sessionIDs))
	cleaned := make([]string, 0, len(sessionIDs))
	for _, rawSessionID := range sessionIDs {
		sessionID := strings.TrimSpace(rawSessionID)
		if sessionID == "" {
			continue
		}
		if _, present := seen[sessionID]; present {
			continue
		}
		seen[sessionID] = struct{}{}
		cleaned = append(cleaned, sessionID)
	}
	sort.Strings(cleaned)
	return cleaned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstTruthy(values ...any) any {
	for _, value := range values {
		if truthy(value) {
			return value
		}
	}
	return nil
}

func numeric(value any) (float64, bool) {
	switch typedValue := value.(type) {
	case json.Number:
		parsedNumber, operationError := typedValue.Float64()
		return parsedNumber, operationError == nil
	case float64:
		return typedValue, true
	case int:
		return float64(typedValue), true
	case int64:
		return float64(typedValue), true
	case string:
		parsedNumber, operationError := strconv.ParseFloat(strings.TrimSpace(typedValue), 64)
		return parsedNumber, operationError == nil
	default:
		return 0, false
	}
}

func truthy(value any) bool {
	switch typedValue := value.(type) {
	case nil:
		return false
	case bool:
		return typedValue
	case string:
		return strings.TrimSpace(typedValue) != ""
	case float64:
		return typedValue != 0
	case int:
		return typedValue != 0
	case int64:
		return typedValue != 0
	case json.Number:
		parsedNumber, operationError := typedValue.Float64()
		return operationError == nil && parsedNumber != 0
	default:
		return true
	}
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
