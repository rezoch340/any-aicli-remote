// Auto-decline of Grok's product-feedback prompts. Grok emits feedback_request
// (a rating/NPS solicitation carrying an app-level request_id) as a session
// update. A remote-control client has no use for it, so the daemon declines it on
// the operator's behalf via x.ai/feedback/dismiss and never surfaces it.

package grok

import (
	"strings"
)

const grokMethodFeedbackDismiss = "x.ai/feedback/dismiss"

// AutoDeclineNotification declines a feedback prompt. When it is a feedback
// request, handled is true (so the notification is dropped) and, if the request
// carries the ids needed to dismiss it, agentReply is the dismiss message.
func (providerInstance *GrokProvider) AutoDeclineNotification(method string, params map[string]any) (map[string]any, bool) {
	envelope, handled := parseChildAgentEnvelope(method, params)
	if !handled {
		return nil, false
	}
	if strings.TrimSpace(stringValue(envelope.update["sessionUpdate"])) != "feedback_request" {
		return nil, false
	}
	requestID := strings.TrimSpace(stringValue(envelope.update["request_id"]))
	sessionID := strings.TrimSpace(envelope.parentSessionID)
	if requestID == "" || sessionID == "" {
		// Still drop the prompt; we just cannot address a dismiss without ids.
		return nil, true
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"method":  grokMethodFeedbackDismiss,
		"params":  map[string]any{"session_id": sessionID, "request_id": requestID},
	}, true
}
