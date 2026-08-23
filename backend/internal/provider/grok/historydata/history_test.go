package historydata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	appendLines(testContext, path, updateLine("s", "user_message_chunk", strings.Repeat("x", chatTextCap+10), nil))
	events, _ := readSessionUpdates(testContext, directory, ReadOptions{Limit: 1, MaxBytes: int64(chatTextCap + 4096), ChatOnly: true})
	if len(events) != 1 {
		testContext.Fatalf("events=%#v", events)
	}
	text := textAt(testContext, events[0])
	if len(text) <= chatTextCap || !strings.Contains(text, "truncated for load speed") {
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
	return ReadSessionUpdatesFromRoot(sessionRoot, filepath.Join(directory, "updates.jsonl"), options)
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
