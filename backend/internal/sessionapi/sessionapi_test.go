package sessionapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/grok-remote/grok-remote-app/backend/internal/history"
)

func TestHistoryUsesRouteDefaultsAndAddsResolutionMetadata(testContext *testing.T) {
	root := testContext.TempDir()
	workingDirectory := testContext.TempDir()
	directory := makeSession(testContext, root, workingDirectory, "session-1")
	writeJSON(testContext, filepath.Join(directory, "summary.json"), map[string]any{
		"remote_title": "Remote title",
		"info":         map[string]any{"cwd": workingDirectory},
	})
	for itemIndex := range 25 {
		appendUpdate(testContext, filepath.Join(directory, "updates.jsonl"), "session-1", "agent_message_chunk", string(rune('a'+itemIndex)))
	}

	service := New(history.NewStore(root), testContext.TempDir(), func() string { return workingDirectory })
	result, operationError := service.History(context.Background(), HistoryQuery{SessionID: " session-1 ", Limit: 1})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !result.OK || result.SessionID != "session-1" || result.WorkingDirectory != workingDirectory || result.Title != "Remote title" || result.Directory != directory {
		testContext.Fatalf("history result = %#v", result)
	}
	// history preserves 3/4 of a trimmed page for chat events, so a route-level
	// minimum limit of 20 yields 15 chat events here (rather than one).
	if result.Count != 15 || len(result.Events) != 15 {
		testContext.Fatalf("limit was not clamped to 20: count=%d", result.Count)
	}
	if result.Meta["resolvedSid"] != "session-1" || result.Meta["resolvedDir"] != directory || result.Meta["resolvedCwd"] != workingDirectory {
		testContext.Fatalf("history meta = %#v", result.Meta)
	}

	missing, operationError := service.History(context.Background(), HistoryQuery{SessionID: "missing"})
	if !errors.Is(operationError, NotFoundError) || missing.OK || len(missing.Events) != 0 || missing.Meta["has_more"] != false {
		testContext.Fatalf("missing result=%#v err=%v", missing, operationError)
	}
	if _, operationError := service.History(context.Background(), HistoryQuery{}); !errors.Is(operationError, SessionRequiredError) {
		testContext.Fatalf("required error = %v", operationError)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, operationError := service.History(cancelled, HistoryQuery{SessionID: "session-1"}); !errors.Is(operationError, context.Canceled) {
		testContext.Fatalf("cancel error = %v", operationError)
	}
}

func TestTitlesDeduplicatesCapsAndUsesUpdatesMTime(testContext *testing.T) {
	root := testContext.TempDir()
	workingDirectory := testContext.TempDir()
	directory := makeSession(testContext, root, workingDirectory, "known")
	writeJSON(testContext, filepath.Join(directory, "summary.json"), map[string]any{
		"generated_title": "Known title",
		"cwd":             workingDirectory,
	})
	updates := filepath.Join(directory, "updates.jsonl")
	if operationError := os.WriteFile(updates, nil, 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	modificationTime := time.Unix(1_700_000_000, 123_000_000)
	if operationError := os.Chtimes(updates, modificationTime, modificationTime); operationError != nil {
		testContext.Fatal(operationError)
	}

	service := New(history.NewStore(root), testContext.TempDir(), nil)
	result, operationError := service.Titles([]string{" known ", "known", "missing"}, workingDirectory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	known, valid := result.Titles["known"]
	if !result.OK || result.Count != 1 || !valid {
		testContext.Fatalf("titles = %#v", result)
	}
	if known.Title != "Known title" || known.WorkingDirectory != workingDirectory || known.Directory != directory || known.ModificationTime != modificationTime.UnixMilli() || known.UpdatedAt != known.ModificationTime {
		testContext.Fatalf("known title = %#v", known)
	}

	ids := make([]string, 251)
	for itemIndex := range 250 {
		ids[itemIndex] = "missing-" + strconv.Itoa(itemIndex)
	}
	ids[250] = "known"
	capped, operationError := service.Titles(ids, workingDirectory)
	if operationError != nil || capped.Count != 0 {
		testContext.Fatalf("titles cap = %#v, %v", capped, operationError)
	}
}

func TestSignalsSupportsAliasesAndCalculatesUsage(testContext *testing.T) {
	root := testContext.TempDir()
	workingDirectory := testContext.TempDir()
	directory := makeSession(testContext, root, workingDirectory, "signals")
	writeJSON(testContext, filepath.Join(directory, "summary.json"), map[string]any{"cwd": workingDirectory})
	writeJSON(testContext, filepath.Join(directory, "signals.json"), map[string]any{
		"context_tokens_used":   25,
		"context_window_tokens": 200,
		"turnCount":             3,
		"toolCallCount":         4,
		"primaryModelId":        "",
		"primary_model_id":      "grok-test",
	})

	service := New(history.NewStore(root), testContext.TempDir(), nil)
	result, operationError := service.Signals(" signals ", workingDirectory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !result.OK || result.Directory != directory || result.ContextWindowUsage != 12.5 || result.PrimaryModelID != "grok-test" {
		testContext.Fatalf("signals result = %#v", result)
	}
	if result.TurnCount.(json.Number).String() != "3" || result.ToolCallCount.(json.Number).String() != "4" {
		testContext.Fatalf("signal counts = %#v / %#v", result.TurnCount, result.ToolCallCount)
	}

	if operationError := os.WriteFile(filepath.Join(directory, "signals.json"), []byte("{"), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	failed, operationError := service.Signals("signals", workingDirectory)
	if operationError == nil || failed.OK || failed.SessionID != "signals" {
		testContext.Fatalf("invalid signals result=%#v err=%v", failed, operationError)
	}
	if _, operationError := service.Signals("", workingDirectory); !errors.Is(operationError, SessionRequiredError) {
		testContext.Fatalf("required error = %v", operationError)
	}
	if _, operationError := service.Signals("missing", workingDirectory); !errors.Is(operationError, NotFoundError) {
		testContext.Fatalf("not found error = %v", operationError)
	}
}

func TestArchivedLoadsLegacyShapeAndSupportsReplaceAndToggle(testContext *testing.T) {
	dataDirectory := testContext.TempDir()
	path := filepath.Join(dataDirectory, "archived_sessions.json")
	writeJSON(testContext, path, map[string]any{"archived": []any{" z ", "a", "z", "", nil}})
	service := New(history.NewStore(testContext.TempDir()), dataDirectory, nil)
	service.now = func() time.Time { return time.Unix(100, 500_000_000) }

	current, operationError := service.Archived()
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !current.OK || current.Path != path || strings.Join(current.IDs, ",") != "z,a" || current.Count != 2 {
		testContext.Fatalf("archived = %#v", current)
	}

	added, operationError := service.SetArchived(SetArchivedRequest{ID: "b"})
	if operationError != nil || strings.Join(added.IDs, ",") != "a,b,z" {
		testContext.Fatalf("added=%#v err=%v", added, operationError)
	}
	wantFalse := false
	removed, operationError := service.SetArchived(SetArchivedRequest{SessionID: "z", Archived: &wantFalse})
	if operationError != nil || strings.Join(removed.IDs, ",") != "a,b" {
		testContext.Fatalf("removed=%#v err=%v", removed, operationError)
	}
	replaced, operationError := service.SetArchived(SetArchivedRequest{IDs: []string{}})
	if operationError != nil || replaced.Count != 0 || replaced.IDs == nil {
		testContext.Fatalf("replaced=%#v err=%v", replaced, operationError)
	}
	var saved struct {
		IDs       []string `json:"ids"`
		UpdatedAt float64  `json:"updatedAt"`
	}
	readJSON(testContext, path, &saved)
	if saved.IDs == nil || len(saved.IDs) != 0 || saved.UpdatedAt != 100.5 {
		testContext.Fatalf("saved archive = %#v", saved)
	}
	if _, operationError := service.SetArchived(SetArchivedRequest{}); !errors.Is(operationError, BadRequestError) {
		testContext.Fatalf("bad request error = %v", operationError)
	}
}

func TestRenamePreservesSummaryAndMatchesTitleRules(testContext *testing.T) {
	root := testContext.TempDir()
	workingDirectory := testContext.TempDir()
	directory := makeSession(testContext, root, workingDirectory, "rename")
	path := filepath.Join(directory, "summary.json")
	writeJSON(testContext, path, map[string]any{
		"generated_title": "old title",
		"cwd":             workingDirectory,
		"extra":           map[string]any{"keep": true},
	})
	service := New(history.NewStore(root), testContext.TempDir(), func() string { return workingDirectory })
	service.now = func() time.Time {
		return time.Date(2026, 8, 22, 12, 34, 56, 123_400_000, time.FixedZone("offset", 3600))
	}
	requested := strings.Repeat("界", 159) + "  尾"

	result, operationError := service.Rename(RenameRequest{ID: " rename ", Name: requested})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !result.OK || result.Previous != "old title" || result.Directory != directory || utf8.RuneCountInString(result.Title) != 159 {
		testContext.Fatalf("rename result = %#v", result)
	}
	var summary map[string]any
	readJSON(testContext, path, &summary)
	if summary["remote_title"] != result.Title || summary["generated_title"] != result.Title || summary["session_summary"] != result.Title {
		testContext.Fatalf("renamed summary = %#v", summary)
	}
	if summary["updated_at"] != "2026-08-22T11:34:56.123400Z" {
		testContext.Fatalf("updated_at = %#v", summary["updated_at"])
	}
	extra, valid := summary["extra"].(map[string]any)
	if !valid || extra["keep"] != true {
		testContext.Fatalf("summary field was not preserved: %#v", summary)
	}

	if _, operationError := service.Rename(RenameRequest{}); !errors.Is(operationError, SessionRequiredError) {
		testContext.Fatalf("session required error = %v", operationError)
	}
	if _, operationError := service.Rename(RenameRequest{SessionID: "rename"}); !errors.Is(operationError, TitleRequiredError) {
		testContext.Fatalf("title required error = %v", operationError)
	}
	if _, operationError := service.Rename(RenameRequest{SessionID: "missing", Title: "title"}); !errors.Is(operationError, NotFoundError) {
		testContext.Fatalf("not found error = %v", operationError)
	}
}

func makeSession(testContext *testing.T, root, workingDirectory, sessionID string) string {
	testContext.Helper()
	absolutePath, operationError := filepath.Abs(workingDirectory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if resolved, operationError := filepath.EvalSymlinks(absolutePath); operationError == nil {
		absolutePath = resolved
	}
	directory := filepath.Join(root, history.EncodeSessionWorkingDirectory(absolutePath), sessionID)
	if operationError := os.MkdirAll(directory, 0o755); operationError != nil {
		testContext.Fatal(operationError)
	}
	return directory
}

func appendUpdate(testContext *testing.T, path, sessionID, kind, text string) {
	testContext.Helper()
	value := map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": kind,
				"content":       map[string]any{"type": "text", "text": text},
			},
		},
	}
	data, operationError := json.Marshal(value)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	file, operationError := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := file.Write(append(data, '\n')); operationError != nil {
		_ = file.Close()
		testContext.Fatal(operationError)
	}
	if operationError := file.Close(); operationError != nil {
		testContext.Fatal(operationError)
	}
}

func writeJSON(testContext *testing.T, path string, value any) {
	testContext.Helper()
	data, operationError := json.Marshal(value)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(path, data, 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
}

func readJSON(testContext *testing.T, path string, target any) {
	testContext.Helper()
	data, operationError := os.ReadFile(path)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := json.Unmarshal(data, target); operationError != nil {
		testContext.Fatal(operationError)
	}
}
