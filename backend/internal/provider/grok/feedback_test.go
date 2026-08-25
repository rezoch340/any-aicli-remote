package grok

import "testing"

func feedbackUpdateParams(sessionID string, update map[string]any) map[string]any {
	return map[string]any{"sessionId": sessionID, "update": update}
}

func TestAutoDeclineFeedbackBuildsDismiss(testContext *testing.T) {
	provider := &GrokProvider{}
	reply, handled := provider.AutoDeclineNotification("_x.ai/session/update", feedbackUpdateParams("sess-A", map[string]any{
		"sessionUpdate": "feedback_request",
		"request_id":    "fb-1",
		"prompt":        "How was it?",
	}))
	if !handled {
		testContext.Fatalf("feedback request must be handled/dropped")
	}
	if reply["method"] != grokMethodFeedbackDismiss {
		testContext.Fatalf("method = %v", reply["method"])
	}
	dismiss := reply["params"].(map[string]any)
	if dismiss["request_id"] != "fb-1" || dismiss["session_id"] != "sess-A" {
		testContext.Fatalf("dismiss params = %v", dismiss)
	}
}

func TestAutoDeclineFeedbackDropsEvenWithoutIds(testContext *testing.T) {
	provider := &GrokProvider{}
	reply, handled := provider.AutoDeclineNotification("_x.ai/session/update", feedbackUpdateParams("", map[string]any{
		"sessionUpdate": "feedback_request",
	}))
	if !handled {
		testContext.Fatalf("feedback request must still be dropped")
	}
	if reply != nil {
		testContext.Fatalf("no dismiss should be built without ids: %v", reply)
	}
}

func TestAutoDeclineIgnoresNonFeedback(testContext *testing.T) {
	provider := &GrokProvider{}
	_, handled := provider.AutoDeclineNotification("_x.ai/session/update", feedbackUpdateParams("sess-A", map[string]any{
		"sessionUpdate": "agent_message_chunk",
	}))
	if handled {
		testContext.Fatalf("non-feedback update must not be handled")
	}
}
