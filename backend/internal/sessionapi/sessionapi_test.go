package sessionapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type sessionTestProvider struct {
	metadata         providerapi.SessionMetadata
	lastHistoryQuery providerapi.HistoryQuery
	resolveCalls     int
	lastRenameTitle  string
}

func (providerInstance *sessionTestProvider) ID() string { return "test" }
func (providerInstance *sessionTestProvider) ScanSessions(context.Context) ([]providerapi.SessionMetadata, error) {
	return []providerapi.SessionMetadata{providerInstance.metadata}, nil
}
func (providerInstance *sessionTestProvider) ResolveSession(_ context.Context, sessionID string) (providerapi.SessionMetadata, error) {
	providerInstance.resolveCalls++
	if sessionID == "" {
		return providerapi.SessionMetadata{}, providerapi.SessionNotFoundError
	}
	metadata := providerInstance.metadata
	metadata.SessionID = sessionID
	return metadata, nil
}
func (providerInstance *sessionTestProvider) LoadMessages(context.Context, string) ([]providerapi.Message, error) {
	return nil, nil
}
func (providerInstance *sessionTestProvider) ReadHistory(_ context.Context, query providerapi.HistoryQuery) (providerapi.HistoryPage, error) {
	providerInstance.lastHistoryQuery = query
	if query.SessionID != providerInstance.metadata.SessionID {
		return providerapi.HistoryPage{}, providerapi.SessionNotFoundError
	}
	return providerapi.HistoryPage{Session: providerInstance.metadata, Events: []providerapi.Event{{"text": "hello"}}, Metadata: providerapi.HistoryMetadata{"has_more": false}}, nil
}
func (providerInstance *sessionTestProvider) ReadSignals(context.Context, string) (map[string]any, error) {
	return map[string]any{"context_tokens_used": 25, "context_window_tokens": 100, "primary_model_id": "test-model"}, nil
}
func (providerInstance *sessionTestProvider) RenameSession(_ context.Context, sessionID, title string) (providerapi.RenameResult, error) {
	providerInstance.lastRenameTitle = title
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
	service, operationError := New(providerapi.NewRegistry(providerInstance), "test", testContext.TempDir(), testHistoryPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	return service
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

func TestArchivedSessionsReadLegacyFormatAndSaveCurrentFormat(testContext *testing.T) {
	service := newSessionService(testContext)
	archivePath := filepath.Join(service.DataDirectory, "archived_sessions.json")
	legacyContents := []byte(`{"ids":[" session-a ","session-a"],"updatedAt":123}`)
	if operationError := os.WriteFile(archivePath, legacyContents, 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	loaded, operationError := service.Archived()
	if operationError != nil || len(loaded.IDs) != 1 || loaded.IDs[0] != "session-a" {
		testContext.Fatalf("archived = %#v, error = %v", loaded, operationError)
	}
	contents, operationError := os.ReadFile(archivePath)
	if operationError != nil || string(contents) != string(legacyContents) {
		testContext.Fatalf("GET modified archive to %q, error = %v", contents, operationError)
	}
	result, operationError := service.SetArchived(SetArchivedRequest{IDs: []string{"session-b"}})
	if operationError != nil || len(result.IDs) != 1 || result.IDs[0] != "session-b" {
		testContext.Fatalf("set archived = %#v, error = %v", result, operationError)
	}
	contents, operationError = os.ReadFile(archivePath)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	var sessionIDs []string
	if operationError := json.Unmarshal(contents, &sessionIDs); operationError != nil || len(sessionIDs) != 1 || sessionIDs[0] != "session-b" {
		testContext.Fatalf("archive contents = %q, error = %v", contents, operationError)
	}
}

func TestArchivedSessionsRejectMalformedData(testContext *testing.T) {
	service := newSessionService(testContext)
	if operationError := os.WriteFile(filepath.Join(service.DataDirectory, "archived_sessions.json"), []byte(`{"ids":"session-a"}`), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := service.Archived(); operationError == nil {
		testContext.Fatal("expected malformed archive error")
	}
}

var _ providerapi.Provider = (*sessionTestProvider)(nil)

func testHistoryPolicy() providerapi.HistoryPolicy {
	return providerapi.HistoryPolicy{DefaultLimit: 100, LiveLimit: 400, MinLimit: 20, MaxLimit: 4000, DefaultMaxBytes: 400000, LiveMaxBytes: 512000, BeforeMaxBytes: 1200000, MinMaxBytes: 64000, MaxMaxBytes: 12000000, AdapterEventLimit: 1600, AdapterReadBytes: 8000000, TitleBatchLimit: 250, ChatTextMaxRunes: 120000, MessageScanInitialBytes: 64 * 1024, MessageScanMaxBytes: 8 * 1024 * 1024, MetadataTitleMaxRunes: 80, MetadataSummaryMaxRunes: 160, RenameTitleMaxRunes: 160}
}

func TestHistoryPolicyControlsRequests(testContext *testing.T) {
	policy := providerapi.HistoryPolicy{DefaultLimit: 10, LiveLimit: 20, MinLimit: 5, MaxLimit: 30, DefaultMaxBytes: 100, LiveMaxBytes: 200, BeforeMaxBytes: 300, MinMaxBytes: 80, MaxMaxBytes: 400, AdapterEventLimit: 40, AdapterReadBytes: 500, TitleBatchLimit: 2, ChatTextMaxRunes: 100, MessageScanInitialBytes: 10, MessageScanMaxBytes: 1000, MetadataTitleMaxRunes: 80, MetadataSummaryMaxRunes: 160, RenameTitleMaxRunes: 160}
	providerInstance := &sessionTestProvider{metadata: providerapi.SessionMetadata{ProviderID: "test", SessionID: "one"}}
	service, operationError := New(providerapi.NewRegistry(providerInstance), "test", testContext.TempDir(), policy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	checks := []struct {
		query HistoryQuery
		limit int
		bytes int64
	}{{HistoryQuery{SessionID: "one"}, 10, 100}, {HistoryQuery{SessionID: "one", Live: true}, 20, 200}, {HistoryQuery{SessionID: "one", BeforeBytes: new(int64)}, 10, 300}, {HistoryQuery{SessionID: "one", Limit: 1, MaxBytes: 999}, 5, 400}}
	for _, check := range checks {
		if _, operationError = service.History(context.Background(), check.query); operationError != nil {
			testContext.Fatal(operationError)
		}
		if providerInstance.lastHistoryQuery.Limit != check.limit || providerInstance.lastHistoryQuery.MaxBytes != check.bytes {
			testContext.Fatalf("query=%#v", providerInstance.lastHistoryQuery)
		}
	}
	providerInstance.resolveCalls = 0
	if _, operationError = service.Titles(context.Background(), "", []string{"one", "two", "three"}); operationError != nil {
		testContext.Fatal(operationError)
	}
	if providerInstance.resolveCalls != 2 {
		testContext.Fatalf("resolve calls = %d", providerInstance.resolveCalls)
	}
	if _, operationError = New(nil, "", "", providerapi.HistoryPolicy{}); operationError == nil {
		testContext.Fatal("expected zero policy error")
	}
}

func TestRenameUsesPolicyRuneLimit(testContext *testing.T) {
	policy := testHistoryPolicy()
	policy.RenameTitleMaxRunes = 3
	providerInstance := &sessionTestProvider{metadata: providerapi.SessionMetadata{ProviderID: "test", SessionID: "one", Title: "old"}}
	service, errorValue := New(providerapi.NewRegistry(providerInstance), "test", testContext.TempDir(), policy)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	_, errorValue = service.Rename(context.Background(), RenameRequest{SessionID: "one", Title: "甲乙丙丁"})
	if errorValue != nil || providerInstance.lastRenameTitle != "甲乙丙" {
		testContext.Fatalf("title=%q err=%v", providerInstance.lastRenameTitle, errorValue)
	}
}

type sessionChildAgentProvider struct {
	*sessionTestProvider
	childAgents      []providerapi.ChildAgentRecord
	childAgentsError error
}

func (providerInstance *sessionChildAgentProvider) ListChildAgents(context.Context, string) ([]providerapi.ChildAgentRecord, error) {
	if providerInstance.childAgentsError != nil {
		return nil, providerInstance.childAgentsError
	}
	return providerInstance.childAgents, nil
}

func TestHistoryReturnsNonNilEmptyChildAgentsWithoutCapability(testContext *testing.T) {
	service := newSessionService(testContext)
	history, operationError := service.History(context.Background(), HistoryQuery{SessionID: "session-one"})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if history.ChildAgents == nil || len(history.ChildAgents) != 0 {
		testContext.Fatalf("childAgents = %#v", history.ChildAgents)
	}
}

func TestHistoryUsesChildAgentSourceWhenAvailable(testContext *testing.T) {
	baseProvider := &sessionTestProvider{metadata: providerapi.SessionMetadata{ProviderID: "test", SessionID: "session-one", Title: "Original", ProjectDirectory: testContext.TempDir(), SourcePath: testContext.TempDir() + "/summary.json"}}
	providerInstance := &sessionChildAgentProvider{sessionTestProvider: baseProvider, childAgents: []providerapi.ChildAgentRecord{{ProviderChildID: "child-1", ParentSessionID: "session-one", Status: providerapi.ChildAgentStatusRunning, ToolsUsed: []string{}}}}
	service, operationError := New(providerapi.NewRegistry(providerInstance), "test", testContext.TempDir(), testHistoryPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	history, operationError := service.History(context.Background(), HistoryQuery{SessionID: "session-one"})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(history.ChildAgents) != 1 || history.ChildAgents[0].ProviderChildID != "child-1" || history.ChildAgents[0].ToolsUsed == nil {
		testContext.Fatalf("childAgents = %#v", history.ChildAgents)
	}
}

func TestHistoryPropagatesChildAgentErrors(testContext *testing.T) {
	baseProvider := &sessionTestProvider{metadata: providerapi.SessionMetadata{ProviderID: "test", SessionID: "session-one", Title: "Original", ProjectDirectory: testContext.TempDir(), SourcePath: testContext.TempDir() + "/summary.json"}}
	providerInstance := &sessionChildAgentProvider{sessionTestProvider: baseProvider, childAgentsError: os.ErrPermission}
	service, operationError := New(providerapi.NewRegistry(providerInstance), "test", testContext.TempDir(), testHistoryPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	_, operationError = service.History(context.Background(), HistoryQuery{SessionID: "session-one"})
	if operationError == nil || !os.IsPermission(operationError) {
		testContext.Fatalf("error = %v", operationError)
	}
}
