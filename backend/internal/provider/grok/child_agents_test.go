package grok

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func TestListChildAgentsReadsStructuredMetadataAndSorts(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	workingDirectory := testContext.TempDir()
	summaryPath := writeSummary(testContext, activeRoot, "project", "parent-session", workingDirectory, "parent", "2026-01-02T00:00:00Z")
	sessionDirectory := filepath.Dir(summaryPath)
	writeChildAgentMetadata(testContext, sessionDirectory, "child-z", map[string]any{
		"subagent_id": "child-z", "parent_session_id": "parent-session", "parent_prompt_id": "prompt-z", "child_session_id": "session-z",
		"subagent_type": "explore", "description": strings.Repeat("甲", 200), "status": "cancelled",
		"started_at": "2026-01-02T03:04:05.678Z", "completed_at": "2026-01-02T03:05:05.678Z",
		"duration_ms": 60_000, "tool_calls": 4, "turns": 2, "effective_model_id": "grok-4.6",
		"effective_context_source": "workspace", "context_normalized": true, "capability_mode": "safe", "persona": "builder",
		"role": "worker", "resumed_from": "child-y", "tokens_used": 999, "context_window_tokens": 4096,
		"context_usage_pct": 150, "tools_used": []string{"grep", "go test"}, "error_count": 3,
		"child_cwd": workingDirectory, "prompt": "must not be surfaced", "output": "must not be read",
	})
	writeChildAgentMetadata(testContext, sessionDirectory, "child-a", map[string]any{
		"subagent_id": "child-a", "parent_session_id": "parent-session", "child_session_id": "session-a",
		"subagent_type": "code", "description": "write tests", "status": "running",
		"started_at": "2026-01-02T01:02:03Z", "duration_ms": 5, "tool_calls": 1, "turns": 1,
		"context_normalized": false,
	})
	writeChildAgentMetadata(testContext, sessionDirectory, "child-b", map[string]any{
		"subagent_id": "child-b", "parent_session_id": "parent-session", "child_session_id": "session-b",
		"subagent_type": "code", "description": "unknown state", "status": "mystery",
		"started_at": "2026-01-02T01:02:03Z",
	})
	writeChildAgentMetadata(testContext, sessionDirectory, "child-c", map[string]any{
		"subagent_id": "child-c", "parent_session_id": "parent-session", "child_session_id": "session-c",
		"subagent_type": "code", "description": "failed state", "status": "failed",
		"started_at": "2026-01-02T01:02:03Z",
	})

	providerInstance := mustNew(testContext, Config{SessionsDirectory: activeRoot})
	childAgents, operationError := providerInstance.ListChildAgents(context.Background(), "parent-session")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(childAgents) != 4 {
		testContext.Fatalf("child agents = %#v", childAgents)
	}
	if childAgents[0].ProviderChildID != "child-a" || childAgents[1].ProviderChildID != "child-b" || childAgents[2].ProviderChildID != "child-c" || childAgents[3].ProviderChildID != "child-z" {
		testContext.Fatalf("child agents are not sorted by StartedAt then id: %#v", childAgents)
	}
	if childAgents[1].Status != providerapi.ChildAgentStatusUnknown || childAgents[2].Status != providerapi.ChildAgentStatusFailed {
		testContext.Fatalf("typed statuses missing: %#v", childAgents)
	}
	childAgent := childAgents[3]
	if childAgent.ParentSessionID != "parent-session" || childAgent.ParentPromptID != "prompt-z" || childAgent.ChildSessionID != "session-z" || childAgent.AgentType != "explore" || !strings.HasSuffix(childAgent.Description, "...") || childAgent.Status != providerapi.ChildAgentStatusCancelled || childAgent.StartedAt != time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC).UnixMilli() || childAgent.CompletedAt != time.Date(2026, 1, 2, 3, 5, 5, 678_000_000, time.UTC).UnixMilli() || childAgent.DurationMS != 60_000 || childAgent.ToolCallCount != 4 || childAgent.TurnCount != 2 || childAgent.ModelID != "grok-4.6" || childAgent.ContextSource != "workspace" || !childAgent.ContextNormalized || childAgent.CapabilityMode != "safe" || childAgent.Persona != "builder" || childAgent.Role != "worker" || childAgent.ResumedFrom != "child-y" || childAgent.TokensUsed != 999 || childAgent.ContextWindowTokens != 4096 || childAgent.ContextUsagePercent != 100 || childAgent.ErrorCount != 3 || childAgent.WorkingDirectory != workingDirectory {
		testContext.Fatalf("child agent = %#v", childAgent)
	}
	if len(childAgent.ToolsUsed) != 2 || childAgent.ToolsUsed[0] != "grep" || childAgent.ToolsUsed[1] != "go test" {
		testContext.Fatalf("tools used = %#v", childAgent.ToolsUsed)
	}
}

func TestListChildAgentsReturnsEmptyForMissingDirectory(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	writeSummary(testContext, activeRoot, "project", "parent-session", testContext.TempDir(), "parent", "2026-01-02T00:00:00Z")

	providerInstance := mustNew(testContext, Config{SessionsDirectory: activeRoot})
	childAgents, operationError := providerInstance.ListChildAgents(context.Background(), "parent-session")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(childAgents) != 0 {
		testContext.Fatalf("child agents = %#v", childAgents)
	}
}

func TestListChildAgentsRejectsUnsafeAndInvalidEntries(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	summaryPath := writeSummary(testContext, activeRoot, "project", "parent-session", testContext.TempDir(), "parent", "2026-01-02T00:00:00Z")
	sessionDirectory := filepath.Dir(summaryPath)
	writeChildAgentMetadata(testContext, sessionDirectory, "valid", map[string]any{"subagent_id": "valid", "parent_session_id": "parent-session", "status": "completed", "child_session_id": "child-session"})
	writeChildAgentMetadata(testContext, sessionDirectory, "wrong-parent", map[string]any{"subagent_id": "wrong-parent", "parent_session_id": "other-session", "child_session_id": "wrong-parent"})
	writeChildAgentMetadata(testContext, sessionDirectory, "wrong-directory", map[string]any{"subagent_id": "different-id", "parent_session_id": "parent-session", "child_session_id": "wrong-directory"})
	writeChildAgentMetadata(testContext, sessionDirectory, "missing-child-session", map[string]any{"subagent_id": "missing-child-session", "parent_session_id": "parent-session", "child_session_id": "   "})
	writeRawChildAgentMetadata(testContext, sessionDirectory, "invalid-json", []byte("not JSON"))
	writeRawChildAgentMetadata(testContext, sessionDirectory, "oversized", make([]byte, 1024))

	subagentsDirectory := filepath.Join(sessionDirectory, childAgentsDirectoryName)
	externalDirectory := testContext.TempDir()
	writeRawChildAgentMetadata(testContext, externalDirectory, "external", []byte(`{"subagent_id":"symlink","parent_session_id":"parent-session","child_session_id":"symlink"}`))
	if operationError := os.Symlink(externalDirectory, filepath.Join(subagentsDirectory, "symlink")); operationError != nil {
		testContext.Fatal(operationError)
	}

	historyPolicy := testHistoryPolicy()
	historyPolicy.AdapterReadBytes = 512
	providerInstance := mustNew(testContext, Config{SessionsDirectory: activeRoot, HistoryPolicy: historyPolicy})
	childAgents, operationError := providerInstance.ListChildAgents(context.Background(), "parent-session")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(childAgents) != 1 || childAgents[0].ProviderChildID != "valid" {
		testContext.Fatalf("unsafe child entries were accepted: %#v", childAgents)
	}
}

func TestListChildAgentsHonorsCancelledContext(testContext *testing.T) {
	providerInstance := mustNew(testContext, Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})
	requestContext, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	_, operationError := providerInstance.ListChildAgents(requestContext, "parent-session")
	if !errors.Is(operationError, context.Canceled) {
		testContext.Fatalf("cancelled context error = %v", operationError)
	}
}

func writeChildAgentMetadata(testContext *testing.T, sessionDirectory, directoryName string, metadata map[string]any) {
	testContext.Helper()
	metadataBytes, marshalError := json.Marshal(metadata)
	if marshalError != nil {
		testContext.Fatal(marshalError)
	}
	writeRawChildAgentMetadata(testContext, sessionDirectory, directoryName, metadataBytes)
}

func writeRawChildAgentMetadata(testContext *testing.T, sessionDirectory, directoryName string, metadataBytes []byte) {
	testContext.Helper()
	childDirectory := filepath.Join(sessionDirectory, childAgentsDirectoryName, directoryName)
	if operationError := os.MkdirAll(childDirectory, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(filepath.Join(childDirectory, childAgentMetadataName), metadataBytes, 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
}
