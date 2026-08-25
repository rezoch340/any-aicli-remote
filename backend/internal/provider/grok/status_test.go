package grok

import (
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func statusUpdateParams(sessionID string, update map[string]any) map[string]any {
	return map[string]any{"sessionId": sessionID, "update": update}
}

func TestNormalizeRetryStateToNeutralStatus(testContext *testing.T) {
	provider := &GrokProvider{}
	method, params := provider.NormalizeAgentNotification("_x.ai/session/update", statusUpdateParams("sess-A", map[string]any{
		"sessionUpdate": "retry_state",
		"type":          "retrying",
		"attempt":       float64(2),
		"maxRetries":    float64(5),
		"reason":        "transient",
	}))
	if method != providerapi.SessionStatusUpdateMethod {
		testContext.Fatalf("method = %q, want %q", method, providerapi.SessionStatusUpdateMethod)
	}
	if params["sessionId"] != "sess-A" {
		testContext.Fatalf("sessionId = %v", params["sessionId"])
	}
	retry, hasRetry := params["retry"].(map[string]any)
	if !hasRetry {
		testContext.Fatalf("retry missing: %v", params)
	}
	if retry["phase"] != string(providerapi.RetryPhaseRetrying) {
		testContext.Fatalf("phase = %v", retry["phase"])
	}
	if retry["attempt"] != 2 || retry["maxRetries"] != 5 {
		testContext.Fatalf("attempt/max = %v/%v", retry["attempt"], retry["maxRetries"])
	}
	if retry["reason"] != "transient" {
		testContext.Fatalf("reason = %v", retry["reason"])
	}
}

func TestNormalizeExhaustedRetryReadsRateLimitAndAttempts(testContext *testing.T) {
	provider := &GrokProvider{}
	_, params := provider.NormalizeAgentNotification("_x.ai/session/update", statusUpdateParams("sess-A", map[string]any{
		"sessionUpdate": "retry_state",
		"type":          "exhausted",
		"attempts":      float64(5),
		"isRateLimit":   true,
		"reason":        "429",
	}))
	retry := params["retry"].(map[string]any)
	if retry["phase"] != string(providerapi.RetryPhaseExhausted) {
		testContext.Fatalf("phase = %v", retry["phase"])
	}
	if retry["attempt"] != 5 {
		testContext.Fatalf("attempt = %v (want attempts folded in)", retry["attempt"])
	}
	if retry["rateLimit"] != true {
		testContext.Fatalf("rateLimit = %v", retry["rateLimit"])
	}
}

func TestNormalizeModelAutoSwitchedToNeutralStatus(testContext *testing.T) {
	provider := &GrokProvider{}
	method, params := provider.NormalizeAgentNotification("_x.ai/session/update", statusUpdateParams("sess-A", map[string]any{
		"sessionUpdate":     "model_auto_switched",
		"previous_model_id": "grok-4",
		"new_model_id":      "grok-3",
		"reason":            "unavailable",
	}))
	if method != providerapi.SessionStatusUpdateMethod {
		testContext.Fatalf("method = %q", method)
	}
	modelSwitch := params["modelSwitch"].(map[string]any)
	if modelSwitch["previous"] != "grok-4" || modelSwitch["current"] != "grok-3" || modelSwitch["reason"] != "unavailable" {
		testContext.Fatalf("modelSwitch = %v", modelSwitch)
	}
}

func TestUnrelatedSessionUpdatePassesThrough(testContext *testing.T) {
	provider := &GrokProvider{}
	// A standard ACP update (mode / message chunk) must not be rewritten.
	method, _ := provider.NormalizeAgentNotification("_x.ai/session/update", statusUpdateParams("sess-A", map[string]any{
		"sessionUpdate": "current_mode_update",
		"currentModeId": "plan",
	}))
	if method != "session/update" {
		testContext.Fatalf("method = %q, want session/update passthrough", method)
	}
}
