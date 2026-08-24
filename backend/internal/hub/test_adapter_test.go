package hub

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
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
	case "_x.ai/ask_user_question", "_x.ai/exit_plan_mode":
		request.Operation = providerapi.InteractionOperation
	default:
		if strings.HasPrefix(method, "terminal/") {
			return request, true
		}
		return providerapi.ReverseRequest{}, false
	}
	return request, true
}

func (providerInstance *testProvider) NormalizeInteractionRequest(method string, params map[string]any) (providerapi.InteractionRequest, bool) {
	sessionID := stringValue(params["sessionId"])
	toolCallID := stringValue(params["toolCallId"])
	if sessionID == "" || toolCallID == "" {
		return providerapi.InteractionRequest{}, false
	}
	request := providerapi.InteractionRequest{SessionID: sessionID, ToolCallID: toolCallID}
	switch method {
	case "_x.ai/exit_plan_mode":
		request.Kind = providerapi.InteractionKindExitPlan
		request.PlanContent = stringValue(params["planContent"])
		return request, true
	case "_x.ai/ask_user_question":
		rawQuestions, valid := params["questions"].([]any)
		if !valid || len(rawQuestions) == 0 {
			return providerapi.InteractionRequest{}, false
		}
		questions := make([]providerapi.InteractionQuestion, 0, len(rawQuestions))
		for _, rawQuestion := range rawQuestions {
			questionParams, valid := rawQuestion.(map[string]any)
			if !valid {
				return providerapi.InteractionRequest{}, false
			}
			question := strings.TrimSpace(stringValue(questionParams["question"]))
			if question == "" {
				return providerapi.InteractionRequest{}, false
			}
			questions = append(questions, providerapi.InteractionQuestion{Question: question})
		}
		request.Kind = providerapi.InteractionKindAskQuestion
		request.Questions = questions
		return request, true
	default:
		return providerapi.InteractionRequest{}, false
	}
}

func (providerInstance *testProvider) DenormalizeInteractionResponse(request providerapi.InteractionRequest, response providerapi.InteractionResponse) (map[string]any, error) {
	if request.Kind == providerapi.InteractionKindExitPlan {
		if response.Outcome != providerapi.InteractionOutcomeApproved && response.Outcome != providerapi.InteractionOutcomeCancelled && response.Outcome != providerapi.InteractionOutcomeAbandoned {
			return nil, errors.New("invalid exit-plan outcome")
		}
		return map[string]any{"outcome": string(response.Outcome)}, nil
	}
	if response.Outcome != providerapi.InteractionOutcomeAccepted {
		return nil, errors.New("invalid ask outcome")
	}
	if len(response.Answers) == 0 {
		return nil, errors.New("empty answers")
	}
	answers := make(map[string]any, len(response.Answers))
	for key, selections := range response.Answers {
		index, parseError := strconv.ParseUint(key, 10, 64)
		if parseError != nil || index >= uint64(len(request.Questions)) {
			return nil, errors.New("invalid question index")
		}
		answers[request.Questions[int(index)].Question] = selections
	}
	return map[string]any{"outcome": "accepted", "answers": answers}, nil
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

func testHubPolicy() Policy {
	return Policy{
		ReadBufferBytes: 64 << 10, WriteBufferBytes: 64 << 10, MaxMessageBytes: 16 << 20,
		Heartbeat: 20 * time.Second, ClientReadTimeout: 60 * time.Second,
		WatcherEnsureInterval: 5 * time.Second, StateBroadcastInterval: 15 * time.Second,
		EnsureAttempt: 12 * time.Second, ClientConnectEnsure: 15 * time.Second,
		DialAttempts: 3, DialHandshake: 8 * time.Second, RetryDelay: 250 * time.Millisecond,
		WriteTimeout: 20 * time.Second, ControlWriteTimeout: 5 * time.Second,
		PendingLimit: 256, PendingClientLimit: 32, PendingTimeout: 30 * time.Minute,
		NormalEnsure: 5 * time.Second, PatientEnsure: 18 * time.Second,
		NotificationEnsure: 3 * time.Second, ReverseOperationTimeout: 2 * time.Minute,
		ReverseReadBytes: 2_000_000, TerminalOutputBytes: 1 << 20,
		FilesystemPolicy: fsapi.Policy{MaxReadBytes: 2 * 1024 * 1024, MaxWriteBytes: 4 * 1024 * 1024, MaxListItems: 10_000},
	}
}

func newTestHub(agentURL, workingDirectory string, ensureAgent EnsureAgentFunc) *Hub {
	providerInstance := &testProvider{workingDirectory: workingDirectory}
	hubInstance, operationError := New(agentURL, providerInstance, providerInstance, ensureAgent, testHubPolicy(), nil)
	if operationError != nil {
		panic(operationError)
	}
	return hubInstance
}

var (
	_ providerapi.Provider        = (*testProvider)(nil)
	_ providerapi.ProtocolAdapter = (*testProvider)(nil)
)
