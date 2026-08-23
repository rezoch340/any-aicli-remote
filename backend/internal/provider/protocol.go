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
)

type ReverseRequest struct {
	Operation     ReverseOperation
	SessionID     string
	RequestedPath string
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
	ClassifyReverseRequest(method string, params map[string]any) (ReverseRequest, bool)
	DaemonNotification(NotificationKind, map[string]any) (string, map[string]any)
}
