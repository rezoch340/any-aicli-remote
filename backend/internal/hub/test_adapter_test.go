package hub

import (
	"context"
	"errors"
	"net/url"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type testProvider struct {
	workingDirectory   string
	sessionDirectories map[string]string
}

func (providerInstance *testProvider) ID() string { return "test" }

func (providerInstance *testProvider) ScanSessions(context.Context) ([]providerapi.SessionMetadata, error) {
	if providerInstance.sessionDirectories != nil {
		sessions := make([]providerapi.SessionMetadata, 0, len(providerInstance.sessionDirectories))
		for sessionID, workingDirectory := range providerInstance.sessionDirectories {
			sessions = append(sessions, providerapi.SessionMetadata{ProviderID: providerInstance.ID(), SessionID: sessionID, ProjectDirectory: workingDirectory})
		}
		return sessions, nil
	}
	return []providerapi.SessionMetadata{{ProviderID: providerInstance.ID(), SessionID: "test-session", ProjectDirectory: providerInstance.workingDirectory}}, nil
}

func (providerInstance *testProvider) ResolveSession(_ context.Context, sessionID string) (providerapi.SessionMetadata, error) {
	if strings.TrimSpace(sessionID) == "" {
		return providerapi.SessionMetadata{}, providerapi.SessionRequiredError
	}
	if providerInstance.sessionDirectories != nil {
		workingDirectory := providerInstance.sessionDirectories[sessionID]
		if workingDirectory == "" {
			return providerapi.SessionMetadata{}, providerapi.SessionNotFoundError
		}
		return providerapi.SessionMetadata{ProviderID: providerInstance.ID(), SessionID: sessionID, ProjectDirectory: workingDirectory}, nil
	}
	if providerInstance.workingDirectory == "" {
		return providerapi.SessionMetadata{}, providerapi.SessionNotFoundError
	}
	return providerapi.SessionMetadata{ProviderID: providerInstance.ID(), SessionID: sessionID, ProjectDirectory: providerInstance.workingDirectory}, nil
}

func (providerInstance *testProvider) LoadMessages(context.Context, string) ([]providerapi.Message, error) {
	return nil, nil
}

func (providerInstance *testProvider) ReadHistory(context.Context, providerapi.HistoryQuery) (providerapi.HistoryPage, error) {
	return providerapi.HistoryPage{}, nil
}

func (providerInstance *testProvider) ReadSignals(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (providerInstance *testProvider) RenameSession(context.Context, string, string) (providerapi.RenameResult, error) {
	return providerapi.RenameResult{}, nil
}

func (providerInstance *testProvider) AgentWebSocketURL(string, int, string) *url.URL {
	return &url.URL{}
}

func (providerInstance *testProvider) AgentCommand(providerapi.AgentLaunchConfiguration) (providerapi.AgentCommand, error) {
	return providerapi.AgentCommand{}, errors.New("unused")
}

func (providerInstance *testProvider) PrepareClientRequest(_ context.Context, method string, params map[string]any) (providerapi.PreparedRequest, error) {
	prepared := providerapi.PreparedRequest{Method: method}
	switch method {
	case "initialize":
		prepared.Kind = providerapi.InitializationRequest
		prepared.Patient = true
	case "_test/ping":
		prepared.Kind = providerapi.PingRequest
		prepared.PingResponseMethod = "_test/pong"
		prepared.PingResponseParams = map[string]any{"value": params["value"]}
	case "session/new":
		workingDirectory, operationError := providerapi.CanonicalDirectory(stringValue(params["cwd"]))
		if operationError != nil {
			return prepared, operationError
		}
		params["cwd"] = workingDirectory
		prepared.CreatesSession = true
		prepared.WorkingDirectory = workingDirectory
		prepared.Patient = true
	case "session/load", "session/prompt":
		sessionID := strings.TrimSpace(stringValue(params["sessionId"]))
		prepared.SessionID = sessionID
		prepared.RequiresSession = true
		prepared.Patient = method == "session/load"
		if method == "session/load" {
			prepared.RestoresSession = true
		}
	}
	return prepared, nil
}

func (providerInstance *testProvider) CaptureSessionBinding(prepared providerapi.PreparedRequest, response map[string]any) (providerapi.SessionBinding, bool) {
	if response["error"] != nil {
		return providerapi.SessionBinding{}, false
	}
	sessionID := prepared.SessionID
	if prepared.CreatesSession {
		result, _ := response["result"].(map[string]any)
		sessionID = stringValue(result["sessionId"])
	}
	if sessionID == "" || prepared.WorkingDirectory == "" {
		return providerapi.SessionBinding{}, false
	}
	return providerapi.SessionBinding{ProviderID: providerInstance.ID(), SessionID: sessionID, WorkingDirectory: prepared.WorkingDirectory}, true
}

func (providerInstance *testProvider) ConfigureInitialization(params map[string]any) {
	capabilities := map[string]any{"fs": map[string]any{"readTextFile": true, "writeTextFile": true}, "terminal": true}
	params["clientCapabilities"] = capabilities
}

func (providerInstance *testProvider) TextPromptRequest(sessionID string, text string) (string, map[string]any) {
	return "session/prompt", map[string]any{"sessionId": sessionID, "prompt": text}
}

func (providerInstance *testProvider) EffortRequest(sessionID string, modelID string, effort string) (string, map[string]any) {
	return "session/effort", map[string]any{"sessionId": sessionID, "modelId": modelID, "effort": effort}
}

func (providerInstance *testProvider) NormalizeAgentNotification(method string, params map[string]any) (string, map[string]any) {
	return method, params
}

func (providerInstance *testProvider) ClassifyReverseRequest(method string, params map[string]any) (providerapi.ReverseRequest, bool) {
	request := providerapi.ReverseRequest{SessionID: stringValue(params["sessionId"])}
	switch method {
	case "fs/read_text_file":
		request.Operation = providerapi.ReadFileOperation
		request.RequestedPath = stringValue(params["path"])
	case "fs/write_text_file":
		request.Operation = providerapi.WriteFileOperation
		request.RequestedPath = stringValue(params["path"])
	case "terminal/create":
		request.Operation = providerapi.CreateTerminalOperation
		request.RequestedPath = stringValue(params["cwd"])
	case "terminal/output":
		request.Operation = providerapi.ReadTerminalOperation
	case "terminal/wait_for_exit":
		request.Operation = providerapi.WaitTerminalOperation
	case "terminal/kill":
		request.Operation = providerapi.KillTerminalOperation
	case "terminal/release":
		request.Operation = providerapi.ReleaseTerminalOperation
	case "session/request_permission":
		request.Operation = providerapi.PermissionOperation
	default:
		if strings.HasPrefix(method, "terminal/") {
			return request, true
		}
		return providerapi.ReverseRequest{}, false
	}
	return request, true
}

func (providerInstance *testProvider) DaemonNotification(kind providerapi.NotificationKind, params map[string]any) (string, map[string]any) {
	methods := map[providerapi.NotificationKind]string{
		providerapi.HubStateNotification:        "_x.ai/remote/hub",
		providerapi.DetachedRequestNotification: "_x.ai/remote/rpc_done",
		providerapi.ClientOperationNotification: "_x.ai/remote/client_rpc",
		providerapi.ProtocolErrorNotification:   "error",
		providerapi.LoopFiredNotification:       "_x.ai/remote/loop_fire",
	}
	return methods[kind], params
}

func newTestHub(agentURL, workingDirectory string, ensureAgent EnsureAgentFunc) *Hub {
	providerInstance := &testProvider{workingDirectory: workingDirectory}
	return New(agentURL, providerInstance, providerInstance, ensureAgent, nil)
}

var (
	_ providerapi.Provider        = (*testProvider)(nil)
	_ providerapi.ProtocolAdapter = (*testProvider)(nil)
)
