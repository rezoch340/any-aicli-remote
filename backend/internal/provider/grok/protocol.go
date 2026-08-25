package grok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const (
	grokMethodSessionSetModel = "session/set_model"
	grokMethodRemotePing      = "_x.ai/remote/ping"
	grokMethodRemotePong      = "_x.ai/remote/pong"
)

func (providerInstance *GrokProvider) AgentWebSocketURL(host string, port int, secret string) *url.URL {
	websocketURL := &url.URL{Scheme: "ws", Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/ws"}
	query := websocketURL.Query()
	query.Set("server-key", secret)
	websocketURL.RawQuery = query.Encode()
	return websocketURL
}

func (providerInstance *GrokProvider) AgentCommand(configuration providerapi.AgentLaunchConfiguration) (providerapi.AgentCommand, error) {
	executablePath, operationError := providerInstance.resolveExecutable()
	if operationError != nil {
		return providerapi.AgentCommand{}, operationError
	}
	arguments := []string{"agent"}
	if providerInstance.alwaysApprove {
		arguments = append(arguments, "--always-approve")
	}
	if providerInstance.leader {
		arguments = append(arguments, "--leader")
	} else {
		arguments = append(arguments, "--no-leader")
	}
	bindAddress := net.JoinHostPort(configuration.Host, strconv.Itoa(configuration.Port))
	arguments = append(arguments, "serve", "--bind", bindAddress)
	return providerapi.AgentCommand{
		ExecutablePath: executablePath,
		Arguments:      arguments,
		Environment: []string{
			"GROK_AGENT_SECRET=" + configuration.Secret,
		},
		IdentityTokens: []string{"agent", "serve", "--bind", bindAddress},
	}, nil
}

func (providerInstance *GrokProvider) resolveExecutable() (string, error) {
	candidates := make([]string, 0, 3)
	if providerInstance.executablePath != "" {
		candidates = append(candidates, providerInstance.executablePath)
	}
	if homeDirectory, operationError := os.UserHomeDir(); operationError == nil {
		candidates = append(candidates, filepath.Join(homeDirectory, ".grok", "bin", "grok"), filepath.Join(homeDirectory, ".grok", "bin", "grok.exe"))
	}
	if executablePath, operationError := exec.LookPath("grok"); operationError == nil {
		candidates = append(candidates, executablePath)
	}
	for _, candidate := range candidates {
		fileInfo, operationError := os.Stat(candidate)
		if operationError == nil && !fileInfo.IsDir() && fileInfo.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New("grok CLI not found")
}

func (providerInstance *GrokProvider) PrepareClientRequest(_ context.Context, method string, params map[string]any) (providerapi.PreparedRequest, error) {
	prepared := providerapi.PreparedRequest{Method: method}
	switch method {
	case acp.AgentMethodInitialize:
		prepared.Kind = providerapi.InitializationRequest
		prepared.Patient = true
	case grokMethodRemotePing:
		prepared.Kind = providerapi.PingRequest
		prepared.PingValue = params["t"]
		prepared.PingResponseMethod = grokMethodRemotePong
		prepared.PingResponseParams = map[string]any{"t": params["t"]}
	case acp.AgentMethodSessionNew:
		var request acp.NewSessionRequest
		decodeACPParameters(params, &request)
		workingDirectory, operationError := providerapi.CanonicalDirectory(request.Cwd)
		if operationError != nil {
			return prepared, operationError
		}
		params["cwd"] = workingDirectory
		prepared.CreatesSession = true
		prepared.WorkingDirectory = workingDirectory
		prepared.Patient = true
	case acp.AgentMethodSessionLoad:
		var request acp.LoadSessionRequest
		decodeACPParameters(params, &request)
		sessionID := strings.TrimSpace(string(request.SessionId))
		prepared.SessionID = sessionID
		prepared.RestoresSession = true
		prepared.RequiresSession = true
		prepared.Patient = true
	case acp.AgentMethodSessionPrompt:
		var request acp.PromptRequest
		decodeACPParameters(params, &request)
		sessionID := strings.TrimSpace(string(request.SessionId))
		prepared.SessionID = sessionID
		prepared.RequiresSession = true
	case acp.AgentMethodSessionCancel:
		prepared.SessionID = strings.TrimSpace(stringValue(params["sessionId"]))
		prepared.RequiresSession = true
	case grokMethodSessionSetModel:
		prepared.SessionID = strings.TrimSpace(stringValue(params["sessionId"]))
		prepared.RequiresSession = true
	default:
		return prepared, fmt.Errorf("unsupported client RPC method: %s", method)
	}
	return prepared, nil
}

func (providerInstance *GrokProvider) CaptureSessionBinding(prepared providerapi.PreparedRequest, response map[string]any) (providerapi.SessionBinding, bool) {
	if response["error"] != nil || (prepared.SessionID == "" && !prepared.CreatesSession) {
		return providerapi.SessionBinding{}, false
	}
	sessionID := prepared.SessionID
	if prepared.CreatesSession {
		result, _ := response["result"].(map[string]any)
		sessionID = strings.TrimSpace(stringValue(result["sessionId"]))
		if sessionID == "" {
			session, _ := result["session"].(map[string]any)
			sessionID = strings.TrimSpace(stringValue(session["sessionId"]))
		}
	}
	if sessionID == "" || prepared.WorkingDirectory == "" {
		return providerapi.SessionBinding{}, false
	}
	return providerapi.SessionBinding{ProviderID: ProviderID, SessionID: sessionID, WorkingDirectory: prepared.WorkingDirectory}, true
}

func (providerInstance *GrokProvider) ConfigureInitialization(params map[string]any) {
	capabilities, _ := params["clientCapabilities"].(map[string]any)
	if capabilities == nil {
		capabilities = map[string]any{}
		params["clientCapabilities"] = capabilities
	}
	filesystem, _ := capabilities["fs"].(map[string]any)
	if filesystem == nil {
		filesystem = map[string]any{}
		capabilities["fs"] = filesystem
	}
	filesystem["readTextFile"] = true
	filesystem["writeTextFile"] = true
	capabilities["terminal"] = true
}

func (providerInstance *GrokProvider) TextPromptRequest(sessionID string, text string) (string, map[string]any) {
	return acp.AgentMethodSessionPrompt, map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	}
}

func (providerInstance *GrokProvider) EffortRequest(sessionID string, modelID string, effort string) (string, map[string]any) {
	return grokMethodSessionSetModel, map[string]any{
		"sessionId": sessionID, "modelId": strings.TrimSpace(modelID),
		"_meta": map[string]any{"reasoningEffort": effort},
	}
}

func (providerInstance *GrokProvider) NormalizeAgentNotification(method string, params map[string]any) (string, map[string]any) {
	if normalizedMethod, normalizedParams, handled := providerInstance.normalizeChildAgentNotification(method, params); handled {
		return normalizedMethod, normalizedParams
	}
	if normalizedMethod, normalizedParams, handled := providerInstance.normalizeStatusNotification(method, params); handled {
		return normalizedMethod, normalizedParams
	}
	providerInstance.normalizeToolContent(method, params)
	switch method {
	case "_x.ai/session/update", "x.ai/session/update":
		return "session/update", params
	case "_x.ai/sessions/changed":
		return "sessions/changed", params
	case "_x.ai/models/update":
		return "models/update", params
	case "_x.ai/mcp/servers_updated":
		return "mcp/servers_updated", params
	case grokMethodRemotePong:
		return "provider/pong", params
	default:
		return method, params
	}
}

func (providerInstance *GrokProvider) ClassifyReverseRequest(method string, params map[string]any) (providerapi.ReverseRequest, bool) {
	request := providerapi.ReverseRequest{SessionID: strings.TrimSpace(stringValue(params["sessionId"]))}
	switch method {
	case acp.ClientMethodFsReadTextFile, "fs/readTextFile":
		request.Operation = providerapi.ReadFileOperation
		request.RequestedPath = stringValue(params["path"])
	case acp.ClientMethodFsWriteTextFile, "fs/writeTextFile":
		request.Operation = providerapi.WriteFileOperation
		request.RequestedPath = stringValue(params["path"])
	case acp.ClientMethodTerminalCreate:
		request.Operation = providerapi.CreateTerminalOperation
		request.RequestedPath = stringValue(params["cwd"])
	case acp.ClientMethodTerminalOutput:
		request.Operation = providerapi.ReadTerminalOperation
	case acp.ClientMethodTerminalWaitForExit, "terminal/waitForExit":
		request.Operation = providerapi.WaitTerminalOperation
	case acp.ClientMethodTerminalKill:
		request.Operation = providerapi.KillTerminalOperation
	case acp.ClientMethodTerminalRelease:
		request.Operation = providerapi.ReleaseTerminalOperation
	case grokMethodAskUserQuestion, grokMethodExitPlanMode:
		request.Operation = providerapi.InteractionOperation
	default:
		if strings.HasPrefix(strings.ToLower(method), "terminal/") {
			return request, true
		}
		lowerMethod := strings.ToLower(method)
		if method == acp.ClientMethodSessionRequestPermission || method == "session/requestPermission" || strings.Contains(lowerMethod, "permission") {
			request.Operation = providerapi.PermissionOperation
		} else {
			return providerapi.ReverseRequest{}, false
		}
	}
	return request, true
}

func (providerInstance *GrokProvider) DaemonNotification(kind providerapi.NotificationKind, params map[string]any) (string, map[string]any) {
	switch kind {
	case providerapi.HubStateNotification:
		return "_x.ai/remote/hub", params
	case providerapi.DetachedRequestNotification:
		return "_x.ai/remote/rpc_done", params
	case providerapi.ClientOperationNotification:
		return "_x.ai/remote/client_rpc", params
	case providerapi.ProtocolErrorNotification:
		return "error", params
	case providerapi.LoopFiredNotification:
		return "_x.ai/remote/loop_fire", params
	default:
		return "", nil
	}
}

// decodeACPParameters reuses the ACP SDK's generated wire types while keeping
// unknown Grok extension fields intact in the original parameter map.
func decodeACPParameters(params map[string]any, destination any) {
	encoded, operationError := json.Marshal(params)
	if operationError == nil {
		_ = json.Unmarshal(encoded, destination)
	}
}

var _ providerapi.ProtocolAdapter = (*GrokProvider)(nil)
