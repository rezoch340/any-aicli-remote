package provider

import (
	"context"
	"net/url"
)

type RequestKind int

const (
	StandardRequest RequestKind = iota
	InitializationRequest
	PingRequest
)

type PreparedRequest struct {
	Kind               RequestKind
	Method             string
	SessionID          string
	WorkingDirectory   string
	CreatesSession     bool
	RestoresSession    bool
	RequiresSession    bool
	Patient            bool
	PingValue          any
	PingResponseMethod string
	PingResponseParams map[string]any
}

type SessionBinding struct {
	ProviderID       string
	SessionID        string
	WorkingDirectory string
}

type ReverseOperation int

const (
	UnknownReverseOperation ReverseOperation = iota
	ReadFileOperation
	WriteFileOperation
	CreateTerminalOperation
	ReadTerminalOperation
	WaitTerminalOperation
	KillTerminalOperation
	ReleaseTerminalOperation
	PermissionOperation
	InteractionOperation
)

type ReverseRequest struct {
	Operation     ReverseOperation
	SessionID     string
	RequestedPath string
	// DisplayTitle is a provider-neutral, human-readable description of what a
	// permission request authorizes (e.g. the command). The hub writes it into
	// the forwarded ACP toolCall.title so clients need no provider-wire knowledge.
	DisplayTitle string
}

type NotificationKind int

const (
	HubStateNotification NotificationKind = iota
	DetachedRequestNotification
	ClientOperationNotification
	ProtocolErrorNotification
	LoopFiredNotification
)

type AgentLaunchConfiguration struct {
	Host             string
	Port             int
	Secret           string
	RuntimeDirectory string
}

type AgentCommand struct {
	ExecutablePath string
	Arguments      []string
	Environment    []string
	IdentityTokens []string
}

type ProtocolAdapter interface {
	ID() string
	AgentWebSocketURL(host string, port int, secret string) *url.URL
	AgentCommand(AgentLaunchConfiguration) (AgentCommand, error)
	PrepareClientRequest(context.Context, string, map[string]any) (PreparedRequest, error)
	CaptureSessionBinding(PreparedRequest, map[string]any) (SessionBinding, bool)
	ConfigureInitialization(params map[string]any)
	TextPromptRequest(sessionID string, text string) (string, map[string]any)
	EffortRequest(sessionID string, modelID string, effort string) (string, map[string]any)
	NormalizeAgentNotification(method string, params map[string]any) (string, map[string]any)
	// AutoDeclineNotification lets an adapter intercept a provider notification the
	// daemon should answer on the operator's behalf instead of surfacing it (e.g. a
	// product-feedback prompt). When handled is true the notification is not
	// forwarded to clients; agentReply, when non-nil, is a JSON-RPC message the hub
	// sends to the agent (the hub assigns its id).
	AutoDeclineNotification(method string, params map[string]any) (agentReply map[string]any, handled bool)
	ClassifyReverseRequest(method string, params map[string]any) (ReverseRequest, bool)
	DaemonNotification(NotificationKind, map[string]any) (string, map[string]any)
	// NormalizeInteractionRequest converts a provider reverse interaction request
	// into the neutral typed form. It is called only for InteractionOperation.
	NormalizeInteractionRequest(method string, params map[string]any) (InteractionRequest, bool)
	// DenormalizeInteractionResponse converts a neutral interaction answer back
	// into the provider result payload using the original interaction request.
	DenormalizeInteractionResponse(request InteractionRequest, response InteractionResponse) (map[string]any, error)
}
