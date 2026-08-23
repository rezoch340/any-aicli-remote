package provider

import "context"

type ChildAgentStatus string

const ChildAgentUpdateMethod = "session/child_agent_update"

const (
	ChildAgentStatusRunning   ChildAgentStatus = "running"
	ChildAgentStatusCompleted ChildAgentStatus = "completed"
	ChildAgentStatusFailed    ChildAgentStatus = "failed"
	ChildAgentStatusCancelled ChildAgentStatus = "cancelled"
	ChildAgentStatusUnknown   ChildAgentStatus = "unknown"
)

type ChildAgentEventKind string

const (
	ChildAgentEventStarted   ChildAgentEventKind = "started"
	ChildAgentEventProgress  ChildAgentEventKind = "progress"
	ChildAgentEventCompleted ChildAgentEventKind = "completed"
	ChildAgentEventFailed    ChildAgentEventKind = "failed"
	ChildAgentEventCancelled ChildAgentEventKind = "cancelled"
	ChildAgentEventUpdated   ChildAgentEventKind = "updated"
)

// ChildAgentRecord is provider-normalized structured child-agent metadata.
// It intentionally excludes prompts, outputs, terminal content, and Markdown.
type ChildAgentRecord struct {
	ProviderChildID     string           `json:"providerChildId"`
	ParentSessionID     string           `json:"parentSessionId"`
	ParentPromptID      string           `json:"parentPromptId,omitempty"`
	ChildSessionID      string           `json:"childSessionId,omitempty"`
	AgentType           string           `json:"agentType,omitempty"`
	Description         string           `json:"description,omitempty"`
	Status              ChildAgentStatus `json:"status"`
	StartedAt           int64            `json:"startedAt,omitempty"`
	CompletedAt         int64            `json:"completedAt,omitempty"`
	DurationMS          int64            `json:"durationMs,omitempty"`
	ToolCallCount       int              `json:"toolCallCount,omitempty"`
	TurnCount           int              `json:"turnCount,omitempty"`
	ModelID             string           `json:"modelId,omitempty"`
	ContextSource       string           `json:"contextSource,omitempty"`
	ContextNormalized   bool             `json:"contextNormalized"`
	CapabilityMode      string           `json:"capabilityMode,omitempty"`
	Persona             string           `json:"persona,omitempty"`
	Role                string           `json:"role,omitempty"`
	ResumedFrom         string           `json:"resumedFrom,omitempty"`
	TokensUsed          int64            `json:"tokensUsed,omitempty"`
	ContextWindowTokens int64            `json:"contextWindowTokens,omitempty"`
	ContextUsagePercent float64          `json:"contextUsagePercent,omitempty"`
	ToolsUsed           []string         `json:"toolsUsed"`
	ErrorCount          int              `json:"errorCount,omitempty"`
	WorkingDirectory    string           `json:"workingDirectory,omitempty"`
}

type ChildAgentEvent struct {
	EventID    string              `json:"eventId,omitempty"`
	Sequence   *uint64             `json:"sequence,omitempty"`
	OccurredAt int64               `json:"occurredAt,omitempty"`
	Replay     bool                `json:"replay,omitempty"`
	Kind       ChildAgentEventKind `json:"kind"`
	Agent      ChildAgentRecord    `json:"agent"`
}

// ChildAgentSource is an optional capability for providers with structured
// child-agent metadata. Implementations return records in a stable order.
type ChildAgentSource interface {
	ListChildAgents(context.Context, string) ([]ChildAgentRecord, error)
}
