package sessionapi

import (
	"context"
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type sessionTestProvider struct {
	metadata providerapi.SessionMetadata
}

func (providerInstance *sessionTestProvider) ID() string { return "test" }
func (providerInstance *sessionTestProvider) ScanSessions(context.Context) ([]providerapi.SessionMetadata, error) {
	return []providerapi.SessionMetadata{providerInstance.metadata}, nil
}
func (providerInstance *sessionTestProvider) ResolveSession(_ context.Context, sessionID string) (providerapi.SessionMetadata, error) {
	if sessionID != providerInstance.metadata.SessionID {
		return providerapi.SessionMetadata{}, providerapi.SessionNotFoundError
	}
	return providerInstance.metadata, nil
}
func (providerInstance *sessionTestProvider) LoadMessages(context.Context, string) ([]providerapi.Message, error) {
	return nil, nil
}
func (providerInstance *sessionTestProvider) ReadHistory(_ context.Context, query providerapi.HistoryQuery) (providerapi.HistoryPage, error) {
	if query.SessionID != providerInstance.metadata.SessionID {
		return providerapi.HistoryPage{}, providerapi.SessionNotFoundError
	}
	return providerapi.HistoryPage{Session: providerInstance.metadata, Events: []providerapi.Event{{"text": "hello"}}, Metadata: providerapi.HistoryMetadata{"has_more": false}}, nil
}
func (providerInstance *sessionTestProvider) ReadSignals(context.Context, string) (map[string]any, error) {
	return map[string]any{"context_tokens_used": 25, "context_window_tokens": 100, "primary_model_id": "test-model"}, nil
}
func (providerInstance *sessionTestProvider) RenameSession(_ context.Context, sessionID, title string) (providerapi.RenameResult, error) {
	if sessionID != providerInstance.metadata.SessionID {
		return providerapi.RenameResult{}, providerapi.SessionNotFoundError
	}
	return providerapi.RenameResult{SessionID: sessionID, Title: title, PreviousTitle: providerInstance.metadata.Title, SourcePath: providerInstance.metadata.SourcePath}, nil
}

func newSessionService(testContext *testing.T) *Service {
	testContext.Helper()
	providerInstance := &sessionTestProvider{metadata: providerapi.SessionMetadata{
		ProviderID: "test", SessionID: "session-one", Title: "Original",
		ProjectDirectory: testContext.TempDir(), SourcePath: testContext.TempDir() + "/summary.json",
		LastActiveAt: 1234,
	}}
	return New(providerapi.NewRegistry(providerInstance), "test", testContext.TempDir())
}

func TestSessionServiceUsesProviderCatalog(testContext *testing.T) {
	service := newSessionService(testContext)
	history, operationError := service.History(context.Background(), HistoryQuery{SessionID: "session-one"})
	if operationError != nil || history.WorkingDirectory == "" || history.Count != 1 || history.Title != "Original" {
		testContext.Fatalf("history = %#v, error = %v", history, operationError)
	}
	titles, operationError := service.Titles(context.Background(), "", []string{"session-one"})
	if operationError != nil || titles.Count != 1 || titles.Titles["session-one"].WorkingDirectory == "" {
		testContext.Fatalf("titles = %#v, error = %v", titles, operationError)
	}
	signals, operationError := service.Signals(context.Background(), "", "session-one")
	if operationError != nil || signals.ContextWindowUsage != float64(25) || signals.PrimaryModelID != "test-model" {
		testContext.Fatalf("signals = %#v, error = %v", signals, operationError)
	}
	renamed, operationError := service.Rename(context.Background(), RenameRequest{SessionID: "session-one", Title: "Renamed", WorkingDirectory: "/ignored"})
	if operationError != nil || renamed.Title != "Renamed" || renamed.Previous != "Original" {
		testContext.Fatalf("rename = %#v, error = %v", renamed, operationError)
	}
}

func TestArchivedSessionsPersist(testContext *testing.T) {
	service := newSessionService(testContext)
	wantArchived := true
	result, operationError := service.SetArchived(SetArchivedRequest{SessionID: "session-one", Archived: &wantArchived})
	if operationError != nil || result.Count != 1 {
		testContext.Fatalf("set archived = %#v, error = %v", result, operationError)
	}
	loaded, operationError := service.Archived()
	if operationError != nil || loaded.Count != 1 || loaded.IDs[0] != "session-one" {
		testContext.Fatalf("archived = %#v, error = %v", loaded, operationError)
	}
}

var _ providerapi.Provider = (*sessionTestProvider)(nil)
