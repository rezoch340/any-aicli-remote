package grok

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func TestNormalizeChildAgentNotifications(testContext *testing.T) {
	providerInstance := mustNew(testContext, Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})

	startedMethod, startedParams := providerInstance.NormalizeAgentNotification("x.ai/session_notification", map[string]any{
		"sessionId": "parent-session",
		"update": map[string]any{
			"sessionUpdate":            "subagent_spawned",
			"subagent_id":              "child-1",
			"child_session_id":         "child-session-1",
			"parent_session_id":        "parent-session",
			"parent_prompt_id":         "prompt-123",
			"subagent_type":            "code",
			"model":                    "grok-4.6",
			"description":              strings.Repeat("d", 300),
			"effective_context_source": "workspace",
			"context_normalized":       false,
			"capability_mode":          "full",
			"persona":                  "builder",
			"role":                     "worker",
			"resumed_from":             "child-0",
			"prompt":                   "secret prompt",
		},
		"_meta": map[string]any{"eventId": "parent-session-10", "agentTimestampMs": 1700000010000.0},
	})
	assertChildEvent(testContext, startedMethod, startedParams, providerapi.ChildAgentEventStarted, providerapi.ChildAgentStatusRunning)
	startedEvent := childEventFromParams(testContext, startedParams)
	if startedEvent.Sequence == nil || *startedEvent.Sequence != 10 || startedEvent.Agent.StartedAt != 1700000010000 || startedEvent.Agent.ParentPromptID != "prompt-123" || startedEvent.Agent.AgentType != "code" || startedEvent.Agent.ModelID != "grok-4.6" || startedEvent.Agent.ContextSource != "workspace" || startedEvent.Agent.ContextNormalized || startedEvent.Agent.CapabilityMode != "full" || startedEvent.Agent.Persona != "builder" || startedEvent.Agent.Role != "worker" || startedEvent.Agent.ResumedFrom != "child-0" {
		testContext.Fatalf("started event = %#v", startedEvent)
	}
	if len(startedEvent.Agent.Description) <= 160 || !strings.HasSuffix(startedEvent.Agent.Description, "...") {
		testContext.Fatalf("description not truncated: %#v", startedEvent.Agent.Description)
	}
	assertNoSecrets(testContext, startedParams)

	progressMethod, progressParams := providerInstance.NormalizeAgentNotification("_x.ai/session_notification", map[string]any{
		"method": "x.ai/session_notification",
		"params": map[string]any{
			"sessionId": "parent-session",
			"update": map[string]any{
				"sessionUpdate":         "subagent_progress",
				"subagent_id":           "child-1",
				"child_session_id":      "child-session-1",
				"parent_session_id":     "parent-session",
				"duration_ms":           -1,
				"turn_count":            2,
				"tool_call_count":       3,
				"tokens_used":           44,
				"context_window_tokens": 88,
				"context_usage_pct":     140,
				"tools_used":            []any{"grep", "go test"},
				"error_count":           -9,
				"error":                 "secret failure detail",
			},
			"_meta": map[string]any{"agentTimestampMs": 1700000015000.0, "isReplay": true},
		},
	})
	assertChildEvent(testContext, progressMethod, progressParams, providerapi.ChildAgentEventProgress, providerapi.ChildAgentStatusRunning)
	progressEvent := childEventFromParams(testContext, progressParams)
	if progressEvent.Sequence != nil || !progressEvent.Replay || progressEvent.OccurredAt != 1700000015000 || progressEvent.Agent.DurationMS != 0 || progressEvent.Agent.TurnCount != 2 || progressEvent.Agent.ToolCallCount != 3 || progressEvent.Agent.TokensUsed != 44 || progressEvent.Agent.ContextWindowTokens != 88 || progressEvent.Agent.ContextUsagePercent != 100 || len(progressEvent.Agent.ToolsUsed) != 2 || progressEvent.Agent.ErrorCount != 0 {
		testContext.Fatalf("progress event = %#v", progressEvent)
	}
	assertNoSecrets(testContext, progressParams)

	finishCases := []struct {
		name    string
		status  string
		kind    providerapi.ChildAgentEventKind
		typed   providerapi.ChildAgentStatus
		eventID string
		method  string
		replay  bool
	}{
		{name: "completed persisted", status: "completed", kind: providerapi.ChildAgentEventCompleted, typed: providerapi.ChildAgentStatusCompleted, eventID: "parent-session-11", method: "_x.ai/session/update", replay: true},
		{name: "failed persisted", status: "failed", kind: providerapi.ChildAgentEventFailed, typed: providerapi.ChildAgentStatusFailed, eventID: "parent-session-12", method: "x.ai/session/update"},
		{name: "cancelled persisted", status: "canceled", kind: providerapi.ChildAgentEventCancelled, typed: providerapi.ChildAgentStatusCancelled, eventID: "parent-session-13", method: "x.ai/session/update"},
	}
	for _, testCase := range finishCases {
		finishMethod, finishParams := providerInstance.NormalizeAgentNotification(testCase.method, map[string]any{
			"sessionId": "parent-session",
			"update": map[string]any{
				"sessionUpdate":    "subagent_finished",
				"subagent_id":      "child-1",
				"child_session_id": "child-session-1",
				"status":           testCase.status,
				"duration_ms":      66,
				"tool_calls":       7,
				"turns":            8,
				"tokens_used":      99,
				"output":           "secret final output",
				"error":            "secret failure detail",
			},
			"_meta": map[string]any{"eventId": testCase.eventID, "agentTimestampMs": 1700000020000.0, "isReplay": testCase.replay},
		})
		assertChildEvent(testContext, finishMethod, finishParams, testCase.kind, testCase.typed)
		finishEvent := childEventFromParams(testContext, finishParams)
		if finishEvent.Sequence == nil || *finishEvent.Sequence == 0 || finishEvent.Agent.CompletedAt != 1700000020000 || finishEvent.Agent.DurationMS != 66 || finishEvent.Agent.ToolCallCount != 7 || finishEvent.Agent.TurnCount != 8 || finishEvent.Agent.TokensUsed != 99 {
			testContext.Fatalf("finish event %s = %#v", testCase.name, finishEvent)
		}
		assertNoSecrets(testContext, finishParams)
	}
}

func TestNormalizeChildAgentNotificationPreservesSourceSequenceAcrossArrivalOrder(testContext *testing.T) {
	providerInstance := mustNew(testContext, Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})

	finishedMethod, finishedParams := providerInstance.NormalizeAgentNotification("x.ai/session_notification", map[string]any{
		"sessionId": "parent-session",
		"update": map[string]any{
			"sessionUpdate":    "subagent_finished",
			"subagent_id":      "child-1",
			"child_session_id": "child-session-1",
			"status":           "completed",
		},
		"_meta": map[string]any{"eventId": "parent-session-11", "agentTimestampMs": 1700000020000.0},
	})
	spawnedMethod, spawnedParams := providerInstance.NormalizeAgentNotification("x.ai/session_notification", map[string]any{
		"sessionId": "parent-session",
		"update": map[string]any{
			"sessionUpdate":    "subagent_spawned",
			"subagent_id":      "child-1",
			"child_session_id": "child-session-1",
		},
		"_meta": map[string]any{"eventId": "parent-session-10", "agentTimestampMs": 1700000010000.0},
	})
	zeroMethod, zeroParams := providerInstance.NormalizeAgentNotification("x.ai/session_notification", map[string]any{
		"sessionId": "parent-session",
		"update": map[string]any{
			"sessionUpdate":    "subagent_spawned",
			"subagent_id":      "child-0",
			"child_session_id": "child-session-0",
		},
		"_meta": map[string]any{"eventId": "parent-session-0", "agentTimestampMs": 1700000000000.0},
	})
	assertChildEvent(testContext, finishedMethod, finishedParams, providerapi.ChildAgentEventCompleted, providerapi.ChildAgentStatusCompleted)
	assertChildEvent(testContext, spawnedMethod, spawnedParams, providerapi.ChildAgentEventStarted, providerapi.ChildAgentStatusRunning)
	assertChildEvent(testContext, zeroMethod, zeroParams, providerapi.ChildAgentEventStarted, providerapi.ChildAgentStatusRunning)
	finishedEvent := childEventFromParams(testContext, finishedParams)
	spawnedEvent := childEventFromParams(testContext, spawnedParams)
	zeroEvent := childEventFromParams(testContext, zeroParams)
	if finishedEvent.Sequence == nil || *finishedEvent.Sequence != 11 {
		testContext.Fatalf("finished sequence = %#v", finishedEvent.Sequence)
	}
	if spawnedEvent.Sequence == nil || *spawnedEvent.Sequence != 10 {
		testContext.Fatalf("spawned sequence = %#v", spawnedEvent.Sequence)
	}
	if zeroEvent.Sequence == nil || *zeroEvent.Sequence != 0 {
		testContext.Fatalf("zero sequence = %#v", zeroEvent.Sequence)
	}
}

func TestNormalizeChildAgentNotificationFailClosedAndOrdinaryFallback(testContext *testing.T) {
	providerInstance := mustNew(testContext, Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})

	method, params := providerInstance.NormalizeAgentNotification("x.ai/session_notification", map[string]any{
		"sessionId": "ordinary",
		"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "hello"}},
	})
	if method != "x.ai/session_notification" || params["sessionId"] != "ordinary" {
		testContext.Fatalf("ordinary notification changed: %q %#v", method, params)
	}

	for _, invalid := range []struct {
		name   string
		method string
		params map[string]any
	}{
		{name: "unknown child kind", method: "x.ai/session_notification", params: map[string]any{"sessionId": "parent", "update": map[string]any{"sessionUpdate": "subagent_weird", "subagent_id": "child", "child_session_id": "child-session"}}},
		{name: "missing child session", method: "x.ai/session_notification", params: map[string]any{"sessionId": "parent", "update": map[string]any{"sessionUpdate": "subagent_spawned", "subagent_id": "child"}}},
		{name: "parent mismatch", method: "x.ai/session_notification", params: map[string]any{"sessionId": "parent-a", "update": map[string]any{"sessionUpdate": "subagent_spawned", "subagent_id": "child", "child_session_id": "child-session", "parent_session_id": "parent-b"}}},
		{name: "wrapped unrelated outer method", method: "_x.ai/session_notification", params: map[string]any{"method": "x.ai/other", "params": map[string]any{"sessionId": "parent", "update": map[string]any{"sessionUpdate": "subagent_spawned", "subagent_id": "child", "child_session_id": "child-session"}}}},
	} {
		gotMethod, _ := providerInstance.NormalizeAgentNotification(invalid.method, invalid.params)
		if invalid.name == "wrapped unrelated outer method" {
			if gotMethod != invalid.method {
				testContext.Fatalf("outer non-child was swallowed: %q", gotMethod)
			}
			continue
		}
		if gotMethod != "" {
			testContext.Fatalf("%s did not fail closed: %q", invalid.name, gotMethod)
		}
	}
}

func assertChildEvent(testContext *testing.T, method string, params map[string]any, kind providerapi.ChildAgentEventKind, status providerapi.ChildAgentStatus) {
	testContext.Helper()
	if method != providerapi.ChildAgentUpdateMethod {
		testContext.Fatalf("method = %q", method)
	}
	event := childEventFromParams(testContext, params)
	if event.Kind != kind || event.Agent.Status != status {
		testContext.Fatalf("event=%#v", event)
	}
	if params["sessionId"] != event.Agent.ParentSessionID {
		testContext.Fatalf("parent session mismatch params=%#v event=%#v", params, event)
	}
}

func childEventFromParams(testContext *testing.T, params map[string]any) providerapi.ChildAgentEvent {
	testContext.Helper()
	encoded, marshalError := json.Marshal(params["event"])
	if marshalError != nil {
		testContext.Fatal(marshalError)
	}
	var event providerapi.ChildAgentEvent
	if operationError := json.Unmarshal(encoded, &event); operationError != nil {
		testContext.Fatal(operationError)
	}
	return event
}

func assertNoSecrets(testContext *testing.T, payload map[string]any) {
	testContext.Helper()
	encoded, marshalError := json.Marshal(payload)
	if marshalError != nil {
		testContext.Fatal(marshalError)
	}
	text := string(encoded)
	for _, forbidden := range []string{"secret prompt", "secret final output", "secret failure detail"} {
		if strings.Contains(text, forbidden) {
			testContext.Fatalf("secret leaked in %s", text)
		}
	}
}

// TestNormalizeChildAgentFinishWithUnrecognizedStatusStaysUnknown pins the only
// lifecycle branch a future Grok terminal status can take. A status this build
// does not know must surface as unknown/updated so clients never render it as a
// completed, failed, or cancelled outcome.
func TestNormalizeChildAgentFinishWithUnrecognizedStatusStaysUnknown(testContext *testing.T) {
	providerInstance := mustNew(testContext, Config{SessionsDirectory: filepath.Join(testContext.TempDir(), "sessions")})

	method, params := providerInstance.NormalizeAgentNotification("x.ai/session/update", map[string]any{
		"sessionId": "parent-session",
		"update": map[string]any{
			"sessionUpdate":    "subagent_finished",
			"subagent_id":      "child-1",
			"child_session_id": "child-session-1",
			"status":           "timed_out",
			"duration_ms":      12,
			"tool_calls":       3,
			"turns":            4,
			"tokens_used":      55,
			"output":           "secret final output",
			"error":            "secret failure detail",
		},
		"_meta": map[string]any{"eventId": "parent-session-21", "agentTimestampMs": 1700000030000.0},
	})

	assertChildEvent(testContext, method, params, providerapi.ChildAgentEventUpdated, providerapi.ChildAgentStatusUnknown)
	event := childEventFromParams(testContext, params)
	for _, rejected := range []providerapi.ChildAgentEventKind{
		providerapi.ChildAgentEventCompleted,
		providerapi.ChildAgentEventFailed,
		providerapi.ChildAgentEventCancelled,
	} {
		if event.Kind == rejected {
			testContext.Fatalf("unrecognized status reported as %s", rejected)
		}
	}
	if event.Sequence == nil || *event.Sequence != 21 {
		testContext.Fatalf("sequence = %#v", event.Sequence)
	}
	if event.Agent.ChildSessionID != "child-session-1" || event.Agent.DurationMS != 12 ||
		event.Agent.ToolCallCount != 3 || event.Agent.TurnCount != 4 || event.Agent.TokensUsed != 55 {
		testContext.Fatalf("identity or metrics lost: %#v", event.Agent)
	}
	assertNoSecrets(testContext, params)
}
