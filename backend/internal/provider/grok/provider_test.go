package grok

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func writeSummary(testContext *testing.T, root, project, sessionID, workingDirectory, title, lastActive string) string {
	testContext.Helper()
	directory := filepath.Join(root, project, sessionID)
	if operationError := os.MkdirAll(directory, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	value := map[string]any{
		"info":            map[string]any{"id": sessionID, "cwd": workingDirectory},
		"generated_title": title, "session_summary": title + " summary",
		"created_at": "2026-01-01T00:00:00Z", "last_active_at": lastActive,
	}
	data, _ := json.Marshal(value)
	path := filepath.Join(directory, "summary.json")
	if operationError := os.WriteFile(path, data, 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	return path
}

func TestProviderScansActiveAndArchivedWithDeterministicPrecedence(testContext *testing.T) {
	baseDirectory := testContext.TempDir()
	activeRoot := filepath.Join(baseDirectory, "sessions")
	archivedRoot := filepath.Join(baseDirectory, "archived_sessions")
	activeWorkspace := testContext.TempDir()
	archivedWorkspace := testContext.TempDir()
	writeSummary(testContext, archivedRoot, "project", "duplicate", archivedWorkspace, "archived newer", "2026-08-01T00:00:00Z")
	activePath := writeSummary(testContext, activeRoot, "project", "duplicate", activeWorkspace, "active wins", "2026-01-02T00:00:00Z")
	writeSummary(testContext, archivedRoot, "project", "archived-only", archivedWorkspace, "archived", "2026-07-01T00:00:00Z")

	providerInstance := New(Config{SessionsDirectory: activeRoot})
	sessions, operationError := providerInstance.ScanSessions(context.Background())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(sessions) != 2 {
		testContext.Fatalf("sessions = %#v", sessions)
	}
	metadata, operationError := providerInstance.ResolveSession(context.Background(), "duplicate")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	canonicalActivePath, _ := filepath.EvalSymlinks(activePath)
	if metadata.Title != "active wins" || metadata.ProjectDirectory != activeWorkspace || metadata.SourcePath != canonicalActivePath || metadata.ProviderID != ProviderID {
		testContext.Fatalf("active metadata = %#v", metadata)
	}
}

func TestLoadMessagesExtractsContentAndExcludesReasoning(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	workingDirectory := testContext.TempDir()
	summaryPath := writeSummary(testContext, activeRoot, "project", "message-session", workingDirectory, "messages", "2026-01-02T00:00:00Z")
	lines := []string{
		`{"type":"user","content":[{"type":"input_text","input_text":"hello"}],"timestamp":1700000000}`,
		`{"type":"reasoning","content":"private"}`,
		`{"type":"assistant","content":[{"type":"tool_use","name":"search"},{"type":"tool_result","content":[{"type":"output_text","output_text":"result"}]}],"ts":"1970-01-01T00:00:02Z"}`,
		`{"type":"tool","content":{"text":"tool message"}}`,
	}
	if operationError := os.WriteFile(filepath.Join(filepath.Dir(summaryPath), "chat_history.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	providerInstance := New(Config{SessionsDirectory: activeRoot})
	messages, operationError := providerInstance.LoadMessages(context.Background(), "message-session")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(messages) != 3 || messages[0].Content != "hello" || messages[0].Timestamp != 1_700_000_000_000 || messages[1].Content != "[Tool: search]\nresult" || messages[1].Timestamp != 2000 {
		testContext.Fatalf("messages = %#v", messages)
	}
}

func TestProtocolDefersExistingSessionWorkspaceResolutionAndRequiresNewSessionWorkspace(testContext *testing.T) {
	providerInstance := New(Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})
	firstParameters := map[string]any{"sessionId": "first-session", "mcpServers": []any{}}
	firstRequest, operationError := providerInstance.PrepareClientRequest(context.Background(), acp.AgentMethodSessionLoad, firstParameters)
	if operationError != nil || firstRequest.SessionID != "first-session" || !firstRequest.RequiresSession || !firstRequest.RestoresSession || firstRequest.WorkingDirectory != "" || firstParameters["cwd"] != nil {
		testContext.Fatalf("first load = %#v params=%#v error=%v", firstRequest, firstParameters, operationError)
	}
	promptParameters := map[string]any{"sessionId": "first-session", "prompt": []any{}}
	promptRequest, operationError := providerInstance.PrepareClientRequest(context.Background(), acp.AgentMethodSessionPrompt, promptParameters)
	if operationError != nil || promptRequest.SessionID != "first-session" || !promptRequest.RequiresSession || promptRequest.RestoresSession || promptRequest.WorkingDirectory != "" {
		testContext.Fatalf("prompt = %#v params=%#v error=%v", promptRequest, promptParameters, operationError)
	}
	_, operationError = providerInstance.PrepareClientRequest(context.Background(), acp.AgentMethodSessionNew, map[string]any{"mcpServers": []any{}})
	if !errors.Is(operationError, providerapi.WorkspaceRequiredError) {
		testContext.Fatalf("new session missing cwd error = %v", operationError)
	}
}

func TestEffortRequestUsesSelectedModelWithoutHardcodedFallback(testContext *testing.T) {
	providerInstance := New(Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})
	_, selectedParameters := providerInstance.EffortRequest("session", "grok-current", "high")
	if selectedParameters["modelId"] != "grok-current" {
		testContext.Fatalf("selected model = %#v", selectedParameters["modelId"])
	}
	_, emptyParameters := providerInstance.EffortRequest("session", "", "high")
	if emptyParameters["modelId"] != "" {
		testContext.Fatalf("adapter invented model = %#v", emptyParameters["modelId"])
	}
}

func TestAgentCommandKeepsTransportSecretOutOfProcessArguments(testContext *testing.T) {
	executablePath := filepath.Join(testContext.TempDir(), "grok")
	if operationError := os.WriteFile(executablePath, []byte("#!/bin/sh\n"), 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	providerInstance := New(Config{ExecutablePath: executablePath, AlwaysApprove: true})
	transportSecret := "provider-transport-secret"
	command, operationError := providerInstance.AgentCommand(providerapi.AgentLaunchConfiguration{
		Host: "127.0.0.1", Port: 2419, Secret: transportSecret,
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if strings.Contains(strings.Join(command.Arguments, " "), transportSecret) {
		testContext.Fatalf("transport secret leaked into process arguments: %#v", command.Arguments)
	}
	if len(command.Environment) != 1 || command.Environment[0] != "GROK_AGENT_SECRET="+transportSecret {
		testContext.Fatalf("transport secret environment = %#v", command.Environment)
	}
}

func TestResolveSessionRejectsMismatchedSummaryIdentity(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	workingDirectory := testContext.TempDir()
	path := writeSummary(testContext, activeRoot, "project", "directory-id", workingDirectory, "bad", "2026-01-02T00:00:00Z")
	data, _ := os.ReadFile(path)
	updated := strings.Replace(string(data), `"id":"directory-id"`, `"id":"different-id"`, 1)
	if operationError := os.WriteFile(path, []byte(updated), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	providerInstance := New(Config{SessionsDirectory: activeRoot})
	sessions, operationError := providerInstance.ScanSessions(context.Background())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(sessions) != 0 {
		testContext.Fatalf("mismatched directory entered session listing: %#v", sessions)
	}
	if _, operationError := providerInstance.ResolveSession(context.Background(), "directory-id"); operationError == nil {
		testContext.Fatal("mismatched summary identity was accepted")
	}
}

func TestSessionFilesRejectSiblingSymlinkEscape(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	workingDirectory := testContext.TempDir()
	firstSummary := writeSummary(testContext, activeRoot, "first-project", "first-session", workingDirectory, "first", "2026-01-02T00:00:00Z")
	secondSummary := writeSummary(testContext, activeRoot, "second-project", "second-session", workingDirectory, "second", "2026-01-03T00:00:00Z")
	firstDirectory := filepath.Dir(firstSummary)
	secondDirectory := filepath.Dir(secondSummary)
	secondChat := filepath.Join(secondDirectory, "chat_history.jsonl")
	secondUpdates := filepath.Join(secondDirectory, "updates.jsonl")
	secondSignals := filepath.Join(secondDirectory, "signals.json")
	if operationError := os.WriteFile(secondChat, []byte(`{"type":"assistant","content":"secret"}`+"\n"), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(secondUpdates, []byte(`{"method":"session/update","params":{"sessionId":"second-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"secret"}}}}`+"\n"), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(secondSignals, []byte(`{"secret":true}`), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	for name, target := range map[string]string{
		"chat_history.jsonl": secondChat,
		"updates.jsonl":      secondUpdates,
		"signals.json":       secondSignals,
	} {
		relativeTarget, operationError := filepath.Rel(firstDirectory, target)
		if operationError != nil {
			testContext.Fatal(operationError)
		}
		if operationError := os.Symlink(relativeTarget, filepath.Join(firstDirectory, name)); operationError != nil {
			testContext.Fatal(operationError)
		}
	}

	providerInstance := New(Config{SessionsDirectory: activeRoot})
	if _, operationError := providerInstance.LoadMessages(context.Background(), "first-session"); operationError == nil {
		testContext.Fatal("chat history escaped through sibling symlink")
	}
	if _, operationError := providerInstance.ReadHistory(context.Background(), providerapi.HistoryQuery{SessionID: "first-session"}); operationError == nil {
		testContext.Fatal("updates history escaped through sibling symlink")
	}
	if _, operationError := providerInstance.ReadSignals(context.Background(), "first-session"); operationError == nil {
		testContext.Fatal("signals escaped through sibling symlink")
	}
}

func TestRenameRejectsSymlinkedSummaryWithoutChangingSibling(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	workingDirectory := testContext.TempDir()
	firstSummary := writeSummary(testContext, activeRoot, "first-project", "first-session", workingDirectory, "first", "2026-01-02T00:00:00Z")
	secondSummary := writeSummary(testContext, activeRoot, "second-project", "second-session", workingDirectory, "second", "2026-01-03T00:00:00Z")
	secondBefore, operationError := os.ReadFile(secondSummary)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Remove(firstSummary); operationError != nil {
		testContext.Fatal(operationError)
	}
	relativeTarget, operationError := filepath.Rel(filepath.Dir(firstSummary), secondSummary)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Symlink(relativeTarget, firstSummary); operationError != nil {
		testContext.Fatal(operationError)
	}

	providerInstance := New(Config{SessionsDirectory: activeRoot})
	if _, operationError := providerInstance.RenameSession(context.Background(), "first-session", "escaped"); operationError == nil {
		testContext.Fatal("rename followed a sibling summary symlink")
	}
	secondAfter, operationError := os.ReadFile(secondSummary)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if string(secondAfter) != string(secondBefore) {
		testContext.Fatal("sibling summary changed after rejected rename")
	}
}

func TestRenameWritesSummaryThroughSessionRoot(testContext *testing.T) {
	activeRoot := filepath.Join(testContext.TempDir(), "sessions")
	workingDirectory := testContext.TempDir()
	summaryPath := writeSummary(testContext, activeRoot, "project", "rename-session", workingDirectory, "before", "2026-01-02T00:00:00Z")
	victimSummary := writeSummary(testContext, activeRoot, "other-project", "victim-session", workingDirectory, "victim", "2026-01-03T00:00:00Z")
	victimBefore, operationError := os.ReadFile(victimSummary)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	preplacedTemporary := filepath.Join(filepath.Dir(summaryPath), ".summary.json.any-aicli-remote.tmp")
	relativeVictim, operationError := filepath.Rel(filepath.Dir(summaryPath), victimSummary)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.Symlink(relativeVictim, preplacedTemporary); operationError != nil {
		testContext.Fatal(operationError)
	}
	providerInstance := New(Config{SessionsDirectory: activeRoot})
	result, operationError := providerInstance.RenameSession(context.Background(), "rename-session", "after")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if result.PreviousTitle != "before" || result.Title != "after" {
		testContext.Fatalf("rename result = %#v", result)
	}
	data, operationError := os.ReadFile(summaryPath)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	var summary map[string]any
	if operationError := json.Unmarshal(data, &summary); operationError != nil {
		testContext.Fatal(operationError)
	}
	if summary["remote_title"] != "after" || summary["generated_title"] != "after" || summary["session_summary"] != "after" {
		testContext.Fatalf("renamed summary = %#v", summary)
	}
	victimAfter, operationError := os.ReadFile(victimSummary)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if string(victimAfter) != string(victimBefore) {
		testContext.Fatal("preplaced temporary symlink target was overwritten")
	}
	preplacedInfo, operationError := os.Lstat(preplacedTemporary)
	if operationError != nil || preplacedInfo.Mode()&os.ModeSymlink == 0 {
		testContext.Fatalf("preplaced temporary symlink changed: %v", operationError)
	}
	generatedTemporary, operationError := filepath.Glob(filepath.Join(filepath.Dir(summaryPath), ".summary.json.any-aicli-remote-*.tmp"))
	if operationError != nil || len(generatedTemporary) != 0 {
		testContext.Fatalf("generated temporary rename files remain: %#v, %v", generatedTemporary, operationError)
	}
}
