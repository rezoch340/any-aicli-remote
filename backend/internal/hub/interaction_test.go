package hub

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

// subscribeInteractionClient connects a client and subscribes it to a session so
// interaction requests route to it.
func subscribeInteractionClient(testInstance *testing.T, hubInstance *Hub, sessionID string) (*websocket.Conn, func()) {
	client, closeClient := connectHubClient(testInstance, hubInstance)
	hubInstance.stateMutex.Lock()
	var serverClient *clientConnection
	for connectedClient := range hubInstance.clients {
		serverClient = connectedClient
	}
	if serverClient == nil {
		hubInstance.stateMutex.Unlock()
		testInstance.Fatal("server connection missing")
	}
	hubInstance.subscribeSessionClientLocked(serverClient, "test", sessionID)
	hubInstance.stateMutex.Unlock()
	return client, closeClient
}

func TestInteractionExitPlanRoundTrip(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	client, closeClient := subscribeInteractionClient(testInstance, hubInstance, "plan-session")
	defer closeClient()

	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": "exit-1", "method": "_x.ai/exit_plan_mode", "params": map[string]any{
		"sessionId": "plan-session", "toolCallId": "call-12", "planContent": "# Plan",
	}})

	// The client receives the neutral request, never the provider wire.
	request := readMethod(testInstance, client, providerapi.SessionInteractionRequestMethod)
	params := rpcParams(testInstance, request)
	if params["kind"] != string(providerapi.InteractionKindExitPlan) || params["sessionId"] != "plan-session" {
		testInstance.Fatalf("neutral interaction params = %#v", params)
	}
	clientIdentifier, valid := numericID(request["id"])
	if !valid {
		testInstance.Fatalf("interaction request id = %#v", request["id"])
	}

	if operationError := client.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": clientIdentifier, "result": map[string]any{"outcome": "approved"},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}

	response := readAgentResponse(testInstance, agent, "exit-1")
	if rpcResult(testInstance, response)["outcome"] != "approved" {
		testInstance.Fatalf("agent exit result = %#v", response)
	}
}

func TestInteractionAskRoundTrip(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	client, closeClient := subscribeInteractionClient(testInstance, hubInstance, "ask-session")
	defer closeClient()

	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": "ask-1", "method": "_x.ai/ask_user_question", "params": map[string]any{
		"sessionId": "ask-session", "toolCallId": "call-4",
		"questions": []any{map[string]any{"question": "缓存用 Redis 还是进程内 LRU？"}},
	}})

	request := readMethod(testInstance, client, providerapi.SessionInteractionRequestMethod)
	params := rpcParams(testInstance, request)
	if params["providerId"] != "test" || params["kind"] != string(providerapi.InteractionKindAskQuestion) {
		testInstance.Fatalf("ask neutral params = %#v", params)
	}
	questions, valid := params["questions"].([]any)
	if !valid || len(questions) != 1 || questions[0].(map[string]any)["question"] != "缓存用 Redis 还是进程内 LRU？" {
		testInstance.Fatalf("ask questions = %#v", params["questions"])
	}
	clientIdentifier, _ := numericID(request["id"])

	if operationError := client.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": clientIdentifier,
		"result": map[string]any{"outcome": "accepted", "answers": map[string]any{"0": []any{"a"}}},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}

	response := readAgentResponse(testInstance, agent, "ask-1")
	result := rpcResult(testInstance, response)
	if result["outcome"] != "accepted" {
		testInstance.Fatalf("agent ask result = %#v", response)
	}
	if _, isMap := result["answers"].(map[string]any); !isMap {
		testInstance.Fatalf("answers not relayed as a map: %#v", result["answers"])
	}
	answers := result["answers"].(map[string]any)
	if _, present := answers["缓存用 Redis 还是进程内 LRU？"]; !present {
		testInstance.Fatalf("answers not keyed by original question: %#v", answers)
	}
}

func TestInteractionMalformedAnswerFailsClosed(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	client, closeClient := subscribeInteractionClient(testInstance, hubInstance, "ask-session")
	defer closeClient()

	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": "ask-2", "method": "_x.ai/ask_user_question", "params": map[string]any{
		"sessionId": "ask-session", "toolCallId": "call-9",
		"questions": []any{map[string]any{"question": "选择方案？"}},
	}})
	request := readMethod(testInstance, client, providerapi.SessionInteractionRequestMethod)
	clientIdentifier, _ := numericID(request["id"])

	// An exit outcome answered against an ask must not reach the agent as a result.
	if operationError := client.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": clientIdentifier, "result": map[string]any{"outcome": "approved"},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	response := readAgentResponse(testInstance, agent, "ask-2")
	if _, hasError := response["error"]; !hasError {
		testInstance.Fatalf("malformed answer did not fail closed to the agent: %#v", response)
	}
	if _, hasResult := response["result"]; hasResult {
		testInstance.Fatalf("malformed answer produced a result: %#v", response)
	}
}

func TestInteractionMalformedRequestFailsClosed(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	client, closeClient := subscribeInteractionClient(testInstance, hubInstance, "plan-session")
	defer closeClient()

	// Missing toolCallId: NormalizeInteractionRequest returns false.
	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": "exit-bad", "method": "_x.ai/exit_plan_mode", "params": map[string]any{
		"sessionId": "plan-session",
	}})
	response := readAgentResponse(testInstance, agent, "exit-bad")
	if _, hasError := response["error"]; !hasError {
		testInstance.Fatalf("malformed interaction request did not fail closed: %#v", response)
	}
	// The client must never receive a forwarded request for the malformed frame.
	_ = client.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var forwarded map[string]any
	if readError := client.ReadJSON(&forwarded); readError == nil {
		if method, _ := forwarded["method"].(string); method == providerapi.SessionInteractionRequestMethod {
			testInstance.Fatalf("malformed request was forwarded to client: %#v", forwarded)
		}
	}
}
