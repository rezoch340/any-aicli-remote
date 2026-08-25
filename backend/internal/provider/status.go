package provider

// Session status contract. Some provider agents emit private, non-ACP session
// updates that report transient run state — a retry in progress, or an automatic
// model switch. These are normalized here into a provider-neutral status update
// so clients never parse a provider's private wire. Standard ACP updates (mode,
// message chunks, tool calls) are already neutral and pass through untouched.

const (
	// SessionStatusUpdateMethod is the neutral method a normalized status
	// update is forwarded to clients under.
	SessionStatusUpdateMethod = "session/status_update"
)

// RetryPhase is the neutral phase of a request retry.
type RetryPhase string

const (
	RetryPhaseRetrying  RetryPhase = "retrying"
	RetryPhaseExhausted RetryPhase = "exhausted"
	RetryPhaseFailed    RetryPhase = "failed"
)

// RetryStatus is a provider-neutral view of a request retry.
type RetryStatus struct {
	Phase      RetryPhase `json:"phase"`
	Attempt    int        `json:"attempt,omitempty"`
	MaxRetries int        `json:"maxRetries,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	RateLimit  bool       `json:"rateLimit,omitempty"`
}

// ModelSwitch is a provider-neutral view of an automatic model switch.
type ModelSwitch struct {
	Previous string `json:"previous,omitempty"`
	Current  string `json:"current,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// SessionStatus is the provider-neutral status payload. Fields are optional; a
// given update carries exactly one of them.
type SessionStatus struct {
	SessionID   string       `json:"sessionId"`
	Retry       *RetryStatus `json:"retry,omitempty"`
	ModelSwitch *ModelSwitch `json:"modelSwitch,omitempty"`
}
