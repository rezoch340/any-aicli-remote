// Normalization of Grok-private session status updates (retry / auto model
// switch) into the provider-neutral session/status_update. Standard ACP updates
// pass through NormalizeAgentNotification untouched; only these private
// extensions are rewritten here.

package grok

import (
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

// normalizeStatusNotification rewrites a Grok-private retry_state or
// model_auto_switched session update into a neutral status update. The bool
// reports whether the notification was a status update we own: when true the
// caller must stop (returning the neutral form, or dropping an unmapped one);
// when false it is not a status update and normal handling continues.
func (providerInstance *GrokProvider) normalizeStatusNotification(method string, params map[string]any) (string, map[string]any, bool) {
	envelope, handled := parseChildAgentEnvelope(method, params)
	if !handled {
		return "", nil, false
	}
	kind := strings.TrimSpace(stringValue(envelope.update["sessionUpdate"]))
	sessionID := strings.TrimSpace(envelope.parentSessionID)
	switch kind {
	case "retry_state":
		return providerapi.SessionStatusUpdateMethod, statusParams(sessionID, map[string]any{
			"retry": neutralRetry(envelope.update),
		}), true
	case "model_auto_switched":
		return providerapi.SessionStatusUpdateMethod, statusParams(sessionID, map[string]any{
			"modelSwitch": map[string]any{
				"previous": strings.TrimSpace(stringValue(envelope.update["previous_model_id"])),
				"current":  strings.TrimSpace(stringValue(envelope.update["new_model_id"])),
				"reason":   strings.TrimSpace(stringValue(envelope.update["reason"])),
			},
		}), true
	default:
		return "", nil, false
	}
}

func statusParams(sessionID string, extra map[string]any) map[string]any {
	params := map[string]any{"sessionId": sessionID}
	for key, value := range extra {
		params[key] = value
	}
	return params
}

// neutralRetry maps Grok's RetryState variants onto the neutral phase. The inner
// RetryState enum is internally tagged by "type" with camelCase fields
// (retrying{attempt,maxRetries,reason} | exhausted{attempts,reason,isRateLimit} |
// failed{errorType,message}); snake_case keys are read as a defensive fallback.
func neutralRetry(update map[string]any) map[string]any {
	phase := providerapi.RetryPhaseRetrying
	switch strings.ToLower(strings.TrimSpace(stringValue(update["type"]))) {
	case "exhausted":
		phase = providerapi.RetryPhaseExhausted
	case "failed":
		phase = providerapi.RetryPhaseFailed
	}
	retry := map[string]any{"phase": string(phase)}
	if attempt := anyInt(update["attempt"]); attempt > 0 {
		retry["attempt"] = attempt
	} else if attempts := anyInt(update["attempts"]); attempts > 0 {
		retry["attempt"] = attempts
	}
	if maxRetries := firstInt(update, "maxRetries", "max_retries"); maxRetries > 0 {
		retry["maxRetries"] = maxRetries
	}
	if reason := firstString(update, "reason", "message"); reason != "" {
		retry["reason"] = reason
	}
	if boolValue(update["isRateLimit"]) || boolValue(update["is_rate_limit"]) {
		retry["rateLimit"] = true
	}
	return retry
}

func firstInt(mapping map[string]any, keys ...string) int {
	for _, key := range keys {
		if value := anyInt(mapping[key]); value > 0 {
			return value
		}
	}
	return 0
}
