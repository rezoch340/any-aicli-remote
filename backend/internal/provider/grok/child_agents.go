// Child-agent wire handling. Grok's live and persisted subagent updates are
// mapped to the provider-neutral event without copying prompt, output, or
// error text, and unknown or malformed shapes fail closed.

package grok

import (
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const (
	childAgentsDirectoryName = "subagents"
	childAgentMetadataName   = "meta.json"
)

// grokChildAgentMetadata is deliberately limited to the fields that can be
// safely shown as structured state. Prompt and generated content are omitted.
type grokChildAgentMetadata struct {
	SubagentID             string   `json:"subagent_id"`
	ParentSessionID        string   `json:"parent_session_id"`
	ParentPromptID         string   `json:"parent_prompt_id"`
	ChildSessionID         string   `json:"child_session_id"`
	SubagentType           string   `json:"subagent_type"`
	Description            string   `json:"description"`
	Status                 string   `json:"status"`
	StartedAt              string   `json:"started_at"`
	CompletedAt            string   `json:"completed_at"`
	DurationMS             int64    `json:"duration_ms"`
	ToolCalls              int      `json:"tool_calls"`
	Turns                  int      `json:"turns"`
	ModelID                string   `json:"effective_model_id"`
	EffectiveContextSource string   `json:"effective_context_source"`
	ContextNormalized      bool     `json:"context_normalized"`
	CapabilityMode         string   `json:"capability_mode"`
	Persona                string   `json:"persona"`
	Role                   string   `json:"role"`
	ResumedFrom            string   `json:"resumed_from"`
	TokensUsed             int64    `json:"tokens_used"`
	ContextWindowTokens    int64    `json:"context_window_tokens"`
	ContextUsagePercent    float64  `json:"context_usage_pct"`
	ToolsUsed              []string `json:"tools_used"`
	ErrorCount             int      `json:"error_count"`
	ChildDirectory         string   `json:"child_cwd"`
}

type childAgentWireEnvelope struct {
	parentSessionID string
	update          map[string]any
	meta            map[string]any
}

var _ providerapi.ChildAgentSource = (*GrokProvider)(nil)

// normalizeChildAgentNotification converts Grok's live and persisted child-agent
// updates to the provider-neutral event without copying prompt, output, or error text.
func (providerInstance *GrokProvider) normalizeChildAgentNotification(method string, params map[string]any) (string, map[string]any, bool) {
	envelope, handled := parseChildAgentEnvelope(method, params)
	if !handled {
		return "", nil, false
	}
	kind := strings.TrimSpace(stringValue(envelope.update["sessionUpdate"]))
	if kind == "" || !strings.HasPrefix(kind, "subagent_") {
		return "", nil, false
	}
	if kind != "subagent_spawned" && kind != "subagent_progress" && kind != "subagent_finished" {
		return "", nil, true
	}

	parentSessionID := strings.TrimSpace(envelope.parentSessionID)
	declaredParentSessionID := strings.TrimSpace(stringValue(envelope.update["parent_session_id"]))
	if declaredParentSessionID != "" && declaredParentSessionID != parentSessionID {
		return "", nil, true
	}
	if parentSessionID == "" {
		parentSessionID = declaredParentSessionID
	}
	childID := strings.TrimSpace(stringValue(envelope.update["subagent_id"]))
	childSessionID := strings.TrimSpace(stringValue(envelope.update["child_session_id"]))
	if parentSessionID == "" || childID == "" || childSessionID == "" {
		return "", nil, true
	}

	record := providerapi.ChildAgentRecord{
		ProviderChildID:   childID,
		ParentSessionID:   parentSessionID,
		ParentPromptID:    strings.TrimSpace(stringValue(envelope.update["parent_prompt_id"])),
		ChildSessionID:    childSessionID,
		AgentType:         strings.TrimSpace(stringValue(envelope.update["subagent_type"])),
		Description:       truncate(stringValue(envelope.update["description"]), providerInstance.historyPolicy.MetadataSummaryMaxRunes),
		Status:            providerapi.ChildAgentStatusRunning,
		ModelID:           strings.TrimSpace(stringValue(envelope.update["model"])),
		ContextSource:     strings.TrimSpace(stringValue(envelope.update["effective_context_source"])),
		ContextNormalized: boolValue(envelope.update["context_normalized"]),
		CapabilityMode:    strings.TrimSpace(stringValue(envelope.update["capability_mode"])),
		Persona:           strings.TrimSpace(stringValue(envelope.update["persona"])),
		Role:              strings.TrimSpace(stringValue(envelope.update["role"])),
		ResumedFrom:       strings.TrimSpace(stringValue(envelope.update["resumed_from"])),
		ToolsUsed:         []string{},
		WorkingDirectory:  strings.TrimSpace(stringValue(envelope.update["child_cwd"])),
	}
	event := providerapi.ChildAgentEvent{
		EventID:    strings.TrimSpace(stringValue(envelope.meta["eventId"])),
		OccurredAt: nonnegativeInt64(anyInt64(envelope.meta["agentTimestampMs"])),
		Replay:     boolValue(envelope.meta["isReplay"]),
		Agent:      record,
	}
	if sequence, valid := eventSequence(event.EventID); valid {
		event.Sequence = &sequence
	}

	switch kind {
	case "subagent_spawned":
		event.Kind = providerapi.ChildAgentEventStarted
		event.Agent.StartedAt = event.OccurredAt
	case "subagent_progress":
		event.Kind = providerapi.ChildAgentEventProgress
		event.Agent.DurationMS = nonnegativeInt64(anyInt64(envelope.update["duration_ms"]))
		event.Agent.TurnCount = nonnegativeInt(anyInt(envelope.update["turn_count"]))
		event.Agent.ToolCallCount = nonnegativeInt(anyInt(envelope.update["tool_call_count"]))
		event.Agent.TokensUsed = nonnegativeInt64(anyInt64(envelope.update["tokens_used"]))
		event.Agent.ContextWindowTokens = nonnegativeInt64(anyInt64(envelope.update["context_window_tokens"]))
		event.Agent.ContextUsagePercent = clampPercent(anyFloat64(envelope.update["context_usage_pct"]))
		event.Agent.ToolsUsed = stringSliceValue(envelope.update["tools_used"])
		event.Agent.ErrorCount = nonnegativeInt(anyInt(envelope.update["error_count"]))
	case "subagent_finished":
		event.Agent.Status = normalizeChildAgentStatus(stringValue(envelope.update["status"]))
		event.Agent.CompletedAt = event.OccurredAt
		event.Agent.DurationMS = nonnegativeInt64(anyInt64(envelope.update["duration_ms"]))
		event.Agent.ToolCallCount = nonnegativeInt(anyInt(envelope.update["tool_calls"]))
		event.Agent.TurnCount = nonnegativeInt(anyInt(envelope.update["turns"]))
		event.Agent.TokensUsed = nonnegativeInt64(anyInt64(envelope.update["tokens_used"]))
		event.Agent.ToolsUsed = stringSliceValue(envelope.update["tools_used"])
		switch event.Agent.Status {
		case providerapi.ChildAgentStatusCompleted:
			event.Kind = providerapi.ChildAgentEventCompleted
		case providerapi.ChildAgentStatusFailed:
			event.Kind = providerapi.ChildAgentEventFailed
		case providerapi.ChildAgentStatusCancelled:
			event.Kind = providerapi.ChildAgentEventCancelled
		default:
			event.Kind = providerapi.ChildAgentEventUpdated
		}
	}

	return providerapi.ChildAgentUpdateMethod, map[string]any{"sessionId": parentSessionID, "event": event}, true
}

func parseChildAgentEnvelope(method string, params map[string]any) (childAgentWireEnvelope, bool) {
	switch method {
	case "_x.ai/session_notification":
		outerMethod := strings.TrimSpace(stringValue(params["method"]))
		if outerMethod != "" && outerMethod != "x.ai/session_notification" {
			return childAgentWireEnvelope{}, false
		}
		inner := params
		if nested, valid := params["params"].(map[string]any); valid {
			inner = nested
		}
		update, valid := inner["update"].(map[string]any)
		if !valid {
			return childAgentWireEnvelope{}, false
		}
		meta, _ := inner["_meta"].(map[string]any)
		if meta == nil {
			meta, _ = update["_meta"].(map[string]any)
		}
		return childAgentWireEnvelope{parentSessionID: stringValue(inner["sessionId"]), update: update, meta: meta}, true
	case "_x.ai/session/update", "x.ai/session/update", "x.ai/session_notification":
		update, valid := params["update"].(map[string]any)
		if !valid {
			return childAgentWireEnvelope{}, false
		}
		meta, _ := params["_meta"].(map[string]any)
		if meta == nil {
			meta, _ = update["_meta"].(map[string]any)
		}
		return childAgentWireEnvelope{parentSessionID: stringValue(params["sessionId"]), update: update, meta: meta}, true
	default:
		return childAgentWireEnvelope{}, false
	}
}
