package historydata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func TestReadSessionUpdatesFromRootChatOnlyCoalescesAndPaginatesBefore(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testContext, path,
		updateLine("s", "user_message_chunk", "u1", nil),
		updateLine("s", "agent_message_chunk", "a1", nil),
		updateLine("s", "agent_thought_chunk", "thinking", nil),
		updateLine("s", "tool_call", "", map[string]any{"name": "ignored by chat_only"}),
		updateLine("s", "user_message_chunk", "he", nil),
		updateLine("s", "user_message_chunk", "llo", nil),
		updateLine("s", "agent_message_chunk", "a2", nil),
		updateLine("s", "agent_message_chunk", "!", nil),
	)

	events, meta := readSessionUpdates(testContext, directory, ReadOptions{Limit: 2, MaxBytes: 128, ChatOnly: true})
	if len(events) != 2 {
		testContext.Fatalf("got %d events: %#v", len(events), events)
	}
	if kindAt(testContext, events[0]) != "user_message_chunk" || textAt(testContext, events[0]) != "hello" {
		testContext.Fatalf("first event = kind %q text %q", kindAt(testContext, events[0]), textAt(testContext, events[0]))
	}
	if kindAt(testContext, events[1]) != "agent_message_chunk" || textAt(testContext, events[1]) != "a2!" {
		testContext.Fatalf("second event = kind %q text %q", kindAt(testContext, events[1]), textAt(testContext, events[1]))
	}
	if meta["chat_only"] != true || meta["live"] != false || meta["has_more"] != true {
		testContext.Fatalf("unexpected meta: %#v", meta)
	}
	olderBefore, valid := meta["older_before"].(int64)
	if !valid || olderBefore <= 0 {
		testContext.Fatalf("older_before = %#v", meta["older_before"])
	}
	for _, event := range events {
		if _, private := event["_kind"]; private {
			testContext.Fatalf("private field leaked: %#v", event)
		}
	}

	older, olderMeta := readSessionUpdates(testContext, directory, ReadOptions{Limit: 2, MaxBytes: 128, BeforeBytes: &olderBefore, ChatOnly: true})
	if len(older) != 2 || textAt(testContext, older[0]) != "u1" || textAt(testContext, older[1]) != "a1" {
		testContext.Fatalf("older page events=%#v meta=%#v", older, olderMeta)
	}
	if olderMeta["window_end"].(int64) != olderBefore {
		testContext.Fatalf("older window_end=%#v want %d", olderMeta["window_end"], olderBefore)
	}
}

func TestReadSessionUpdatesFromRootFullFiltersAndMergesMeta(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testContext, path,
		updateLine("s", "agent_thought_chunk", "   ", nil),
		updateLineWithMeta("s", "user_message_chunk", "hi", nil, map[string]any{"eventId": "params", "keep": "params"}, map[string]any{"eventId": "update", "keep": "update", "other": "update"}),
		updateLine("s", "tool_call_update", "", map[string]any{"status": "running"}),
		updateLine("s", "tool_call_update", "", map[string]any{"status": "completed"}),
		updateLine("s", "tool_call_update", "", map[string]any{"status": "running", "rawOutput": "still keep"}),
		updateLine("s", "available_commands_update", "", map[string]any{"commands": []any{"/x"}}),
	)

	events, meta := readSessionUpdates(testContext, directory, ReadOptions{Limit: 20, MaxBytes: 4096})
	if len(events) != 4 {
		testContext.Fatalf("got %d events: %#v meta=%#v", len(events), events, meta)
	}
	if kindAt(testContext, events[0]) != "user_message_chunk" || textAt(testContext, events[0]) != "hi" {
		testContext.Fatalf("first kept event wrong: %#v", events[0])
	}
	merged := metaAt(testContext, events[0])
	if merged["eventId"] != "params" || merged["keep"] != "params" || merged["other"] != "update" {
		testContext.Fatalf("merged meta = %#v", merged)
	}
	if kindAt(testContext, events[1]) != "tool_call_update" || kindAt(testContext, events[2]) != "tool_call_update" || kindAt(testContext, events[3]) != "available_commands_update" {
		testContext.Fatalf("filter/order wrong: %#v", events)
	}
	if meta["returned"] != 4 || meta["scanned"] != 4 || meta["chat_only"] != false {
		testContext.Fatalf("unexpected meta: %#v", meta)
	}
}

func TestReadSessionUpdatesFromRootLiveSinceAndIncompleteTail(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testContext, path, updateLine("s", "user_message_chunk", "old", nil))
	fileInfo, operationError := os.Stat(path)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	since := fileInfo.Size()
	good := updateLine("s", "agent_message_chunk", "new", nil)
	partial := strings.TrimSuffix(updateLine("s", "agent_message_chunk", "partial", nil), "\n")[:40]
	appendRaw(testContext, path, good+partial)

	events, meta := readSessionUpdates(testContext, directory, ReadOptions{Limit: 10, SinceBytes: since, Live: true, MaxBytes: 512_000})
	if len(events) != 1 || textAt(testContext, events[0]) != "new" {
		testContext.Fatalf("live events=%#v meta=%#v", events, meta)
	}
	if meta["since"] != since || meta["live"] != true || meta["has_more"] != false {
		testContext.Fatalf("unexpected live meta: %#v", meta)
	}

	end := meta["size"].(int64)
	empty, emptyMeta := readSessionUpdates(testContext, directory, ReadOptions{SinceBytes: end, Live: true})
	if len(empty) != 0 || emptyMeta["returned"] != 0 || emptyMeta["end"] != end {
		testContext.Fatalf("empty live events=%#v meta=%#v", empty, emptyMeta)
	}
}

func TestReadSessionUpdatesFromRootTrimsLongChatText(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testContext, path, updateLine("s", "user_message_chunk", strings.Repeat("x", 120000+10), nil))
	events, _ := readSessionUpdates(testContext, directory, ReadOptions{Limit: 1, MaxBytes: int64(120000 + 4096), ChatOnly: true})
	if len(events) != 1 {
		testContext.Fatalf("events=%#v", events)
	}
	text := textAt(testContext, events[0])
	if len(text) <= 120000 || !strings.Contains(text, "truncated for load speed") {
		testContext.Fatalf("text was not trimmed: len=%d suffix=%q", len(text), text[len(text)-40:])
	}
}

func updateLine(sessionID, kind, text string, extra map[string]any) string {
	return updateLineWithMeta(sessionID, kind, text, extra, nil, nil)
}

func updateLineWithMeta(sessionID, kind, text string, extra, paramsMeta, updateMeta map[string]any) string {
	update := map[string]any{"sessionUpdate": kind}
	if text != "" || kind == "user_message_chunk" || kind == "agent_message_chunk" || kind == "agent_thought_chunk" {
		update["content"] = map[string]any{"type": "text", "text": text}
	}
	for key, value := range extra {
		update[key] = value
	}
	if updateMeta != nil {
		update["_meta"] = updateMeta
	}
	params := map[string]any{"sessionId": sessionID, "update": update}
	if paramsMeta != nil {
		params["_meta"] = paramsMeta
	}
	obj := map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": params}
	data, operationError := json.Marshal(obj)
	if operationError != nil {
		panic(operationError)
	}
	return string(data) + "\n"
}

func readSessionUpdates(testContext *testing.T, directory string, options ReadOptions) ([]Event, Meta) {
	testContext.Helper()
	sessionRoot, operationError := os.OpenRoot(directory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	defer sessionRoot.Close()
	events, metadata, operationError := ReadSessionUpdatesFromRoot(sessionRoot, filepath.Join(directory, "updates.jsonl"), testHistoryPolicy(), options)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	return events, metadata
}

func appendLines(testContext *testing.T, path string, lines ...string) {
	testContext.Helper()
	appendRaw(testContext, path, strings.Join(lines, ""))
}

func appendRaw(testContext *testing.T, path, data string) {
	testContext.Helper()
	file, operationError := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	defer file.Close()
	if _, operationError := file.WriteString(data); operationError != nil {
		testContext.Fatal(operationError)
	}
}

func kindAt(testContext *testing.T, event Event) string {
	testContext.Helper()
	update := updateAt(testContext, event)
	value, _ := update["sessionUpdate"].(string)
	return value
}

func textAt(testContext *testing.T, event Event) string {
	testContext.Helper()
	update := updateAt(testContext, event)
	content, _ := update["content"].(map[string]any)
	value, _ := content["text"].(string)
	return value
}

func metaAt(testContext *testing.T, event Event) map[string]any {
	testContext.Helper()
	params, _ := event["params"].(map[string]any)
	meta, _ := params["_meta"].(map[string]any)
	return meta
}

func updateAt(testContext *testing.T, event Event) map[string]any {
	testContext.Helper()
	params, valid := event["params"].(map[string]any)
	if !valid {
		testContext.Fatalf("missing params: %#v", event)
	}
	update, valid := params["update"].(map[string]any)
	if !valid {
		testContext.Fatalf("missing update: %#v", event)
	}
	return update
}

func testHistoryPolicy() providerapi.HistoryPolicy {
	return providerapi.HistoryPolicy{DefaultLimit: 100, LiveLimit: 400, MinLimit: 20, MaxLimit: 4000, DefaultMaxBytes: 400000, LiveMaxBytes: 512000, BeforeMaxBytes: 1200000, MinMaxBytes: 64000, MaxMaxBytes: 12000000, AdapterEventLimit: 1600, AdapterReadBytes: 8000000, TitleBatchLimit: 250, ChatTextMaxRunes: 120000, MessageScanInitialBytes: 64 * 1024, MessageScanMaxBytes: 8 * 1024 * 1024, MetadataTitleMaxRunes: 80, MetadataSummaryMaxRunes: 160, RenameTitleMaxRunes: 160}
}

func TestReadSessionUpdatesRejectsZeroPolicy(testContext *testing.T) {
	directory := testContext.TempDir()
	appendLines(testContext, filepath.Join(directory, "updates.jsonl"), updateLine("one", "user_message_chunk", "hello", nil))
	sessionRoot, operationError := os.OpenRoot(directory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	defer sessionRoot.Close()
	if _, _, operationError = ReadSessionUpdatesFromRoot(sessionRoot, filepath.Join(directory, "updates.jsonl"), providerapi.HistoryPolicy{}, ReadOptions{}); operationError == nil {
		testContext.Fatal("expected policy error")
	}
}

func TestReadSessionUpdatesUsesPolicyAndLiveWindow(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testContext, path, updateLine("one", "user_message_chunk", strings.Repeat("old", 80), nil), updateLine("one", "agent_message_chunk", strings.Repeat("new", 80), nil))
	policy := testHistoryPolicy()
	policy.AdapterEventLimit = 1
	policy.AdapterReadBytes = 500
	sessionRoot, operationError := os.OpenRoot(directory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	defer sessionRoot.Close()
	events, metadata, operationError := ReadSessionUpdatesFromRoot(sessionRoot, path, policy, ReadOptions{})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(events) != 1 || eventText(events[0]) != strings.Repeat("new", 80) || metadata["window_start"].(int64) <= 0 {
		testContext.Fatalf("events=%#v meta=%#v", events, metadata)
	}
	events, metadata, operationError = ReadSessionUpdatesFromRoot(sessionRoot, path, policy, ReadOptions{Live: true, MaxBytes: 500})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(events) != 1 || metadata["window_start"].(int64) <= 0 {
		testContext.Fatalf("events=%#v meta=%#v", events, metadata)
	}
}

func eventText(event Event) string {
	params, _ := event["params"].(map[string]any)
	update, _ := params["update"].(map[string]any)
	content, _ := update["content"].(map[string]any)
	value, _ := content["text"].(string)
	return value
}

func TestChatTextPolicyTruncatesUnicodeAcrossPaths(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testContext, path, updateLine("s", "user_message_chunk", "甲乙丙丁戊", nil))
	policy := testHistoryPolicy()
	policy.ChatTextMaxRunes = 3
	root, errorValue := os.OpenRoot(directory)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	defer root.Close()
	for _, options := range []ReadOptions{{Limit: 1, MaxBytes: 4096}, {Limit: 1, MaxBytes: 4096, ChatOnly: true}, {Limit: 1, MaxBytes: 4096, Live: true}} {
		events, _, errorValue := ReadSessionUpdatesFromRoot(root, path, policy, options)
		if errorValue != nil || len(events) != 1 {
			testContext.Fatalf("events=%#v err=%v", events, errorValue)
		}
		text := textAt(testContext, events[0])
		if text != "甲乙丙\n…[truncated for load speed]" || !utf8.ValidString(text) {
			testContext.Fatalf("text=%q", text)
		}
	}
}

func TestReadLiveSinceCannotBypassMaxBytes(testingContext *testing.T) {
	directory := testingContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testingContext, path,
		updateLine("session", "user_message_chunk", strings.Repeat("old", 80), nil),
		updateLine("session", "agent_message_chunk", "new", nil),
	)
	events, metadata := readSessionUpdates(testingContext, directory, ReadOptions{Live: true, SinceBytes: 1, Limit: 10, MaxBytes: 128})
	if metadata["window_start"].(int64) < metadata["size"].(int64)-128 {
		testingContext.Fatalf("unclamped metadata=%#v", metadata)
	}
	for _, event := range events {
		if textAt(testingContext, event) == strings.Repeat("old", 80) {
			testingContext.Fatalf("old event bypassed cap: %#v", events)
		}
	}
}

func TestReadSessionUpdatesFromRootPreservesChildAgentOrderInLiveAndFull(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "updates.jsonl")
	appendLines(testContext, path,
		updateLineWithMeta("parent", "subagent_spawned", "", map[string]any{"subagent_id": "child-1", "child_session_id": "child-session-1"}, map[string]any{"eventId": "parent-10"}, nil),
		updateLineWithMeta("parent", "subagent_progress", "", map[string]any{"subagent_id": "child-1", "child_session_id": "child-session-1"}, map[string]any{"eventId": "parent-11"}, nil),
		updateLineWithMeta("parent", "subagent_finished", "", map[string]any{"subagent_id": "child-1", "child_session_id": "child-session-1", "status": "completed"}, map[string]any{"eventId": "parent-12"}, nil),
	)

	fullEvents, _ := readSessionUpdates(testContext, directory, ReadOptions{Limit: 10, MaxBytes: 4096})
	if len(fullEvents) != 3 || kindAt(testContext, fullEvents[0]) != "subagent_spawned" || kindAt(testContext, fullEvents[1]) != "subagent_progress" || kindAt(testContext, fullEvents[2]) != "subagent_finished" {
		testContext.Fatalf("full child agent order = %#v", fullEvents)
	}
	fullMeta := metaAt(testContext, fullEvents[1])
	if fullMeta["eventId"] != "parent-11" {
		testContext.Fatalf("progress meta missing: %#v", fullMeta)
	}

	liveEvents, _ := readSessionUpdates(testContext, directory, ReadOptions{Limit: 10, Live: true, MaxBytes: 4096})
	if len(liveEvents) != 3 || kindAt(testContext, liveEvents[0]) != "subagent_spawned" || kindAt(testContext, liveEvents[1]) != "subagent_progress" || kindAt(testContext, liveEvents[2]) != "subagent_finished" {
		testContext.Fatalf("live child agent order = %#v", liveEvents)
	}
}
