// Session reads: paginated history, batched titles, and provider signals.

package sessionapi

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

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
	OK               bool                           `json:"ok"`
	Error            string                         `json:"error,omitempty"`
	ProviderID       string                         `json:"providerId"`
	SessionID        string                         `json:"sessionId"`
	WorkingDirectory string                         `json:"cwd"`
	Title            string                         `json:"title"`
	Directory        string                         `json:"dir"`
	Events           []providerapi.Event            `json:"events"`
	Meta             providerapi.HistoryMetadata    `json:"meta"`
	Count            int                            `json:"count"`
	ChildAgents      []providerapi.ChildAgentRecord `json:"childAgents"`
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
		result := HistoryResult{OK: false, Error: NotFoundError.Error(), ProviderID: providerInstance.ID(), SessionID: sessionID, Events: []providerapi.Event{}, ChildAgents: []providerapi.ChildAgentRecord{}, Meta: providerapi.HistoryMetadata{"has_more": false}}
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
	childAgents := []providerapi.ChildAgentRecord{}
	if source, supported := providerInstance.(providerapi.ChildAgentSource); supported {
		var childError error
		childAgents, childError = source.ListChildAgents(operationContext, page.Session.SessionID)
		if childError != nil {
			return HistoryResult{}, childError
		}
		if childAgents == nil {
			childAgents = []providerapi.ChildAgentRecord{}
		}
	}
	return HistoryResult{
		OK: true, ProviderID: page.Session.ProviderID, SessionID: page.Session.SessionID,
		WorkingDirectory: page.Session.ProjectDirectory, Title: page.Session.Title, Directory: directory,
		Events: page.Events, Meta: page.Metadata, Count: len(page.Events), ChildAgents: childAgents,
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
