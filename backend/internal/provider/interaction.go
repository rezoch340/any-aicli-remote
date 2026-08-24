package provider

// Structured interaction contract. A provider agent may block a turn waiting for
// the operator to answer a question or approve a plan. These arrive as reverse
// requests and are normalized here into a provider-neutral typed interaction so
// clients never parse a provider's private wire. The response travels the same
// path in reverse: a neutral answer is denormalized back to the provider shape.

const (
	// SessionInteractionRequestMethod is the neutral method a normalized
	// interaction request is forwarded to clients under.
	SessionInteractionRequestMethod = "session/interaction_request"
)

type InteractionKind string

const (
	InteractionKindAskQuestion InteractionKind = "ask_question"
	InteractionKindExitPlan    InteractionKind = "exit_plan"
)

// InteractionOption is one selectable choice for a question.
type InteractionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// InteractionQuestion is one question with its choices.
type InteractionQuestion struct {
	Question    string              `json:"question"`
	Options     []InteractionOption `json:"options"`
	MultiSelect bool                `json:"multiSelect"`
}

// InteractionRequest is the provider-neutral form of a blocking interaction.
// For an ask, Questions is populated. For an exit-plan, PlanContent is the plan
// text (may be empty). Mode carries the provider's interaction mode when known
// (for Grok: "default" or "plan").
type InteractionRequest struct {
	Kind        InteractionKind       `json:"kind"`
	SessionID   string                `json:"sessionId"`
	ToolCallID  string                `json:"toolCallId"`
	Questions   []InteractionQuestion `json:"questions,omitempty"`
	PlanContent string                `json:"planContent,omitempty"`
	Mode        string                `json:"mode,omitempty"`
}

type InteractionOutcome string

const (
	// Ask outcomes.
	InteractionOutcomeAccepted  InteractionOutcome = "accepted"
	InteractionOutcomeChatAbout InteractionOutcome = "chat_about_this"
	InteractionOutcomeSkip      InteractionOutcome = "skip_interview"
	// Exit-plan outcomes.
	InteractionOutcomeApproved  InteractionOutcome = "approved"
	InteractionOutcomeCancelled InteractionOutcome = "cancelled"
	InteractionOutcomeAbandoned InteractionOutcome = "abandoned"
)

// InteractionAnnotation carries optional extra text for an accepted answer.
type InteractionAnnotation struct {
	Preview string `json:"preview,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// InteractionResponse is the provider-neutral answer to an interaction. Answers
// maps a question index (as a decimal string, e.g. "0") to the selected option
// labels. PartialAnswers carries free-text answers for chat/skip outcomes.
// Feedback is optional plan-revision text for a cancelled exit-plan.
type InteractionResponse struct {
	Outcome        InteractionOutcome               `json:"outcome"`
	Answers        map[string][]string              `json:"answers,omitempty"`
	Annotations    map[string]InteractionAnnotation `json:"annotations,omitempty"`
	PartialAnswers map[string]string                `json:"partialAnswers,omitempty"`
	Feedback       string                           `json:"feedback,omitempty"`
}
