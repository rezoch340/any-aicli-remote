package grok

import (
	"reflect"
	"testing"
)

func toolUpdateParams(content []any) map[string]any {
	return map[string]any{
		"sessionId": "sess-A",
		"update": map[string]any{
			"sessionUpdate": "tool_call_update",
			"toolCallId":    "t1",
			"status":        "completed",
			"content":       content,
		},
	}
}

func toolContentAfterNormalize(content []any) []any {
	provider := &GrokProvider{}
	_, params := provider.NormalizeAgentNotification("_x.ai/session/update", toolUpdateParams(content))
	return params["update"].(map[string]any)["content"].([]any)
}

func TestWrapsBareContentBlockIntoStandardToolContent(testContext *testing.T) {
	got := toolContentAfterNormalize([]any{map[string]any{"type": "text", "text": "ls output"}})
	want := []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "ls output"}}}
	if !reflect.DeepEqual(got, want) {
		testContext.Fatalf("got %v, want %v", got, want)
	}
}

func TestLeavesStandardAndDiffToolContentUntouched(testContext *testing.T) {
	standard := map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "x"}}
	diff := map[string]any{"type": "diff", "path": "/f", "newText": "y"}
	got := toolContentAfterNormalize([]any{standard, diff})
	if !reflect.DeepEqual(got, []any{standard, diff}) {
		testContext.Fatalf("standard/diff content was rewritten: %v", got)
	}
}

func TestNonToolUpdateContentIsUntouched(testContext *testing.T) {
	provider := &GrokProvider{}
	params := map[string]any{
		"sessionId": "sess-A",
		"update": map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": "hi"},
		},
	}
	_, out := provider.NormalizeAgentNotification("_x.ai/session/update", params)
	content := out["update"].(map[string]any)["content"]
	if _, isObject := content.(map[string]any); !isObject {
		testContext.Fatalf("non-tool content shape changed: %v", content)
	}
}
