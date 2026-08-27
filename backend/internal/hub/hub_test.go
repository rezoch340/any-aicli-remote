package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeAgent struct {
	server     *httptest.Server
	messages   chan map[string]any
	connection chan *websocket.Conn
	writeMutex sync.Mutex
}

func newFakeAgent(testInstance *testing.T) *fakeAgent {
	testInstance.Helper()
	fixture := &fakeAgent{
		messages:   make(chan map[string]any, 16),
		connection: make(chan *websocket.Conn, 1),
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		connection, operationError := upgrader.Upgrade(responseWriter, request, nil)
		if operationError != nil {
			testInstance.Errorf("upgrade fake agent: %v", operationError)
			return
		}
		select {
		case fixture.connection <- connection:
		default:
		}
		defer connection.Close()
		for {
			_, raw, operationError := connection.ReadMessage()
			if operationError != nil {
				return
			}
			var object map[string]any
			if operationError := json.Unmarshal(raw, &object); operationError == nil {
				fixture.messages <- object
			}
		}
	}))
	testInstance.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *fakeAgent) websocketURL() string {
	return "ws" + strings.TrimPrefix(fixture.server.URL, "http")
}

func (fixture *fakeAgent) send(testInstance *testing.T, object map[string]any) {
	testInstance.Helper()
	var connection *websocket.Conn
	select {
	case connection = <-fixture.connection:
		fixture.connection <- connection
	case <-time.After(2 * time.Second):
		testInstance.Fatal("fake agent did not connect")
	}
	fixture.writeMutex.Lock()
	defer fixture.writeMutex.Unlock()
	if operationError := connection.WriteJSON(object); operationError != nil {
		testInstance.Fatalf("fake agent write: %v", operationError)
	}
}

func (fixture *fakeAgent) disconnect(testInstance *testing.T) {
	testInstance.Helper()
	select {
	case connection := <-fixture.connection:
		if operationError := connection.Close(); operationError != nil {
			testInstance.Fatalf("fake agent disconnect: %v", operationError)
		}
	case <-time.After(2 * time.Second):
		testInstance.Fatal("fake agent did not connect")
	}
}

func connectHubClient(testInstance *testing.T, hubInstance *Hub) (*websocket.Conn, func()) {
	testInstance.Helper()
	server := httptest.NewServer(http.HandlerFunc(hubInstance.HandleWebSocket))
	connection, _, operationError := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if operationError != nil {
		server.Close()
		testInstance.Fatal(operationError)
	}
	return connection, func() {
		_ = connection.Close()
		server.Close()
	}
}

func TestHandleWebSocketRejectsCrossSiteBrowserOrigin(testInstance *testing.T) {
	hubInstance := newTestHub("ws://127.0.0.1:1", testInstance.TempDir(), nil)
	defer hubInstance.Close()
	server := httptest.NewServer(http.HandlerFunc(hubInstance.HandleWebSocket))
	defer server.Close()
	headers := http.Header{"Origin": {"https://attacker.example"}}
	connection, response, operationError := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"), headers,
	)
	if connection != nil {
		_ = connection.Close()
		testInstance.Fatal("cross-site WebSocket origin was accepted")
	}
	if operationError == nil || response == nil || response.StatusCode != http.StatusForbidden {
		testInstance.Fatalf("cross-site upgrade error=%v response=%v", operationError, response)
	}
	_ = response.Body.Close()
}

func readObject(testInstance *testing.T, connection *websocket.Conn) map[string]any {
	testInstance.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	var object map[string]any
	if operationError := connection.ReadJSON(&object); operationError != nil {
		testInstance.Fatal(operationError)
	}
	return object
}

func readMethod(testInstance *testing.T, connection *websocket.Conn, method string) map[string]any {
	testInstance.Helper()
	for attemptIndex := 0; attemptIndex < 8; attemptIndex++ {
		object := readObject(testInstance, connection)
		if object["method"] == method {
			return object
		}
	}
	testInstance.Fatalf("did not receive method %q", method)
	return nil
}

func readResponse(testInstance *testing.T, connection *websocket.Conn, requestIdentifier any) map[string]any {
	testInstance.Helper()
	want, _ := json.Marshal(requestIdentifier)
	for attemptIndex := 0; attemptIndex < 8; attemptIndex++ {
		object := readObject(testInstance, connection)
		got, _ := json.Marshal(object["id"])
		if string(got) == string(want) && object["method"] == nil {
			return object
		}
	}
	testInstance.Fatalf("did not receive response id %v", requestIdentifier)
	return nil
}

func TestHubRemapsIDsCachesInitializeAndReportsDetachedCompletion(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	workingDirectory := testInstance.TempDir()
	hubInstance := newTestHub(agent.websocketURL(), workingDirectory, nil)
	defer hubInstance.Close()

	first, closeFirst := connectHubClient(testInstance, hubInstance)
	defer closeFirst()
	if operationError := first.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "mobile-init", "method": "initialize", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	initRequest := <-agent.messages
	hubID, valid := numericID(initRequest["id"])
	if !valid || hubID < 1 || initRequest["id"] == "mobile-init" {
		testInstance.Fatalf("request id was not remapped: %#v", initRequest)
	}
	params, _ := initRequest["params"].(map[string]any)
	capabilities, _ := params["clientCapabilities"].(map[string]any)
	filesystem, _ := capabilities["fs"].(map[string]any)
	if filesystem["readTextFile"] != true || filesystem["writeTextFile"] != true || capabilities["terminal"] != true {
		testInstance.Fatalf("tool capabilities were not advertised: %#v", initRequest)
	}
	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": hubID, "result": map[string]any{"server": "grok"}})
	initResponse := readResponse(testInstance, first, "mobile-init")
	if initResponse["id"] != "mobile-init" {
		testInstance.Fatalf("response id was not restored: %#v", initResponse)
	}

	second, closeSecond := connectHubClient(testInstance, hubInstance)
	defer closeSecond()
	if operationError := second.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": float64(9), "method": "initialize", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	cached := readObject(testInstance, second)
	if cached["id"] != float64(9) || !hubInstance.InitCached() {
		testInstance.Fatalf("cached initialize response = %#v", cached)
	}
	select {
	case unexpected := <-agent.messages:
		testInstance.Fatalf("cached initialize went upstream: %#v", unexpected)
	case <-time.After(100 * time.Millisecond):
	}

	if operationError := first.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "turn-1", "method": "session/prompt", "params": map[string]any{"sessionId": "test-session"},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	prompt := <-agent.messages
	promptID, valid := numericID(prompt["id"])
	if !valid {
		testInstance.Fatalf("prompt id = %#v", prompt["id"])
	}
	_ = first.Close()
	deadline := time.Now().Add(2 * time.Second)
	for hubInstance.ClientCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]any{"ok": true}})
	done := readMethod(testInstance, second, "_x.ai/remote/rpc_done")
	doneParams, _ := done["params"].(map[string]any)
	if doneParams["id"] != "turn-1" || doneParams["detached"] != true {
		testInstance.Fatalf("detached completion = %#v", done)
	}
}

func TestHubAddsProviderIdentityToSessionNotification(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	agent.send(testInstance, map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params":  map[string]any{"sessionId": "session-notification", "update": map[string]any{"kind": "agent_message_chunk"}},
	})
	notification := readMethod(testInstance, client, "session/update")
	params := rpcParams(testInstance, notification)
	if params["providerId"] != "test" || params["sessionId"] != "session-notification" {
		testInstance.Fatalf("session notification scope = %#v", params)
	}
}

func TestNewSessionBindingWorksBeforeProviderHistoryPersistsAndDoesNotCrossWorkspaces(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	firstWorkspace := testInstance.TempDir()
	secondWorkspace := testInstance.TempDir()
	providerInstance := &testProvider{sessionDirectories: map[string]string{}}
	hubInstance, operationError := New(agent.websocketURL(), providerInstance, providerInstance, nil, testHubPolicy(), nil)
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	createSession := func(requestIdentifier, sessionID, workingDirectory string) {
		testInstance.Helper()
		if operationError := client.WriteJSON(map[string]any{
			"jsonrpc": "2.0", "id": requestIdentifier, "method": "session/new",
			"params": map[string]any{"cwd": workingDirectory},
		}); operationError != nil {
			testInstance.Fatal(operationError)
		}
		request := <-agent.messages
		requestID, valid := numericID(request["id"])
		if !valid {
			testInstance.Fatalf("new session id was not remapped: %#v", request)
		}
		params, _ := request["params"].(map[string]any)
		canonicalWorkspace, _ := filepath.EvalSymlinks(workingDirectory)
		if params["cwd"] != canonicalWorkspace {
			testInstance.Fatalf("new session cwd = %#v, want %q", params["cwd"], canonicalWorkspace)
		}
		agent.send(testInstance, map[string]any{
			"jsonrpc": "2.0", "id": requestID, "result": map[string]any{"sessionId": sessionID},
		})
		response := readResponse(testInstance, client, requestIdentifier)
		if response["error"] != nil {
			testInstance.Fatalf("new session response = %#v", response)
		}
	}

	createSession("new-first", "new-session-first", firstWorkspace)
	activeSessions, operationError := hubInstance.ActiveSessions("test")
	canonicalFirstWorkspace, _ := filepath.EvalSymlinks(firstWorkspace)
	if operationError != nil || len(activeSessions) != 1 || activeSessions[0].SessionID != "new-session-first" || activeSessions[0].ProjectDirectory != canonicalFirstWorkspace {
		testInstance.Fatalf("active sessions before provider persistence = %#v, error = %v", activeSessions, operationError)
	}
	if operationError := client.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "prompt-first", "method": "session/prompt",
		"params": map[string]any{"sessionId": "new-session-first", "prompt": "hello"},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	promptRequest := <-agent.messages
	promptID, valid := numericID(promptRequest["id"])
	if !valid {
		testInstance.Fatalf("prompt did not reach agent: %#v", promptRequest)
	}
	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]any{"ok": true}})
	if response := readResponse(testInstance, client, "prompt-first"); response["error"] != nil {
		testInstance.Fatalf("prompt response = %#v", response)
	}
	agent.send(testInstance, map[string]any{
		"jsonrpc": "2.0", "id": "permission-first", "method": "session/request_permission",
		"params": map[string]any{
			"sessionId": "new-session-first",
			"options":   []any{map[string]any{"optionId": "deny_once"}},
		},
	})
	permissionRequest := readMethod(testInstance, client, "session/request_permission")
	permissionParams := rpcParams(testInstance, permissionRequest)
	if permissionParams["providerId"] != "test" || permissionParams["sessionId"] != "new-session-first" {
		testInstance.Fatalf("new session permission scope = %#v", permissionParams)
	}
	clientPermissionIdentifier, valid := numericID(permissionRequest["id"])
	if !valid {
		testInstance.Fatalf("new session permission request id = %#v", permissionRequest["id"])
	}
	if operationError := client.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": clientPermissionIdentifier,
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "deny_once"}},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	permissionResponse := readAgentResponse(testInstance, agent, "permission-first")
	permissionOutcome, _ := rpcResult(testInstance, permissionResponse)["outcome"].(map[string]any)
	if permissionOutcome["optionId"] != "deny_once" {
		testInstance.Fatalf("new session permission response = %#v", permissionResponse)
	}

	firstPath := filepath.Join(firstWorkspace, "first.txt")
	if response := reverseCall(testInstance, agent, "write-first", "fs/write_text_file", map[string]any{
		"sessionId": "new-session-first", "path": firstPath, "content": "first",
	}); len(rpcResult(testInstance, response)) != 0 {
		testInstance.Fatalf("first write response = %#v", response)
	}
	createSession("new-second", "new-session-second", secondWorkspace)

	crossedPath := filepath.Join(secondWorkspace, "crossed.txt")
	crossedResponse := reverseCall(testInstance, agent, "write-crossed", "fs/write_text_file", map[string]any{
		"sessionId": "new-session-first", "path": crossedPath, "content": "crossed",
	})
	if crossedResponse["error"] == nil {
		testInstance.Fatalf("first session wrote into second workspace: %#v", crossedResponse)
	}
	if _, operationError := os.Stat(crossedPath); !os.IsNotExist(operationError) {
		testInstance.Fatalf("cross-workspace path exists after rejected write: %v", operationError)
	}
	if response := reverseCall(testInstance, agent, "write-second", "fs/write_text_file", map[string]any{
		"sessionId": "new-session-second", "path": crossedPath, "content": "second",
	}); len(rpcResult(testInstance, response)) != 0 {
		testInstance.Fatalf("second write response = %#v", response)
	}
	secondContent, operationError := os.ReadFile(crossedPath)
	if operationError != nil || string(secondContent) != "second" {
		testInstance.Fatalf("second workspace content = %q, error = %v", secondContent, operationError)
	}
}

func TestReadTextFileKeepsLineEndingsAndHonorsExplicitZeroLimit(testInstance *testing.T) {
	workingDirectory := testInstance.TempDir()
	path := filepath.Join(workingDirectory, "sample.txt")
	if operationError := os.WriteFile(path, []byte("one\r\ntwo\nthree"), 0o644); operationError != nil {
		testInstance.Fatal(operationError)
	}
	result, operationError := readTextFile(map[string]any{"path": path, "line": 2, "limit": 1}, workingDirectory, testHubPolicy().ReverseReadBytes)
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	if result["content"] != "two\n" {
		testInstance.Fatalf("content = %q", result["content"])
	}
	result, operationError = readTextFile(map[string]any{"path": path, "limit": 0}, workingDirectory, testHubPolicy().ReverseReadBytes)
	if operationError != nil || result["content"] != "" {
		testInstance.Fatalf("zero limit result=%#v err=%v", result, operationError)
	}
	result, operationError = readTextFile(map[string]any{"path": path, "line": 3}, workingDirectory, testHubPolicy().ReverseReadBytes)
	if operationError != nil || result["content"] != "three" {
		testInstance.Fatalf("final line result=%#v err=%v", result, operationError)
	}
}

func TestNumericIDRejectsFraction(testInstance *testing.T) {
	if _, valid := numericID(1.5); valid {
		testInstance.Fatal("fractional JSON-RPC id was accepted")
	}
}

func TestClosePreventsInFlightEnsureFromStartingAgent(testInstance *testing.T) {
	var starts atomic.Int32
	hubInstance := newTestHub("ws://127.0.0.1:1/ws", "", func(context.Context) error {
		starts.Add(1)
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- hubInstance.Ensure(context.Background()) }()
	// The first failed dial sleeps before attempt two, which is the attempt that
	// would normally invoke the agent starter.
	time.Sleep(50 * time.Millisecond)
	hubInstance.Close()
	select {
	case operationError := <-done:
		if operationError == nil {
			testInstance.Fatal("ensure unexpectedly succeeded after close")
		}
	case <-time.After(2 * time.Second):
		testInstance.Fatal("in-flight ensure did not stop")
	}
	if got := starts.Load(); got != 0 {
		testInstance.Fatalf("closed hub invoked the agent starter %d time(s)", got)
	}
}

func TestNewStoresCustomPolicy(testInstance *testing.T) {
	policy := testHubPolicy()
	policy.PendingLimit = 7
	policy.PendingClientLimit = 3
	policy.PendingTimeout = 41 * time.Millisecond
	policy.TerminalOutputBytes = 2048
	policy.MaxMessageBytes = 4096
	policy.AgentMaxMessageBytes = 8192
	policy.WriteTimeout = 17 * time.Millisecond
	policy.ControlWriteTimeout = 19 * time.Millisecond
	providerInstance := &testProvider{workingDirectory: testInstance.TempDir()}
	hubInstance, operationError := New("ws://127.0.0.1:1", providerInstance, providerInstance, nil, policy, nil)
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	defer hubInstance.Close()
	if hubInstance.pendingLimit != policy.PendingLimit || hubInstance.pendingClientLimit != policy.PendingClientLimit || hubInstance.pendingTimeout != policy.PendingTimeout {
		testInstance.Fatalf("pending policy not preserved: %#v", hubInstance)
	}
	if hubInstance.policy.MaxMessageBytes != 4096 || hubInstance.policy.AgentMaxMessageBytes != 8192 {
		testInstance.Fatalf("message limits not preserved: %#v", hubInstance.policy)
	}
	if hubInstance.terminals.defaultOutputBytes != policy.TerminalOutputBytes || hubInstance.upgrader.ReadBufferSize != policy.ReadBufferBytes || hubInstance.upgrader.WriteBufferSize != policy.WriteBufferBytes {
		testInstance.Fatalf("hub policy not preserved: %#v", hubInstance.policy)
	}
	client := &clientConnection{writeTimeout: policy.WriteTimeout, controlWriteTimeout: policy.ControlWriteTimeout}
	if client.writeTimeout != policy.WriteTimeout || client.controlWriteTimeout != policy.ControlWriteTimeout {
		testInstance.Fatal("websocket deadline policy not preserved")
	}
}

func TestNewRejectsInvalidPolicy(testInstance *testing.T) {
	providerInstance := &testProvider{workingDirectory: testInstance.TempDir()}
	policy := testHubPolicy()
	policy.ReadBufferBytes = 0
	if _, operationError := New("ws://127.0.0.1:1", providerInstance, providerInstance, nil, policy, nil); operationError == nil {
		testInstance.Fatal("zero buffer policy accepted")
	}
	policy = testHubPolicy()
	policy.PendingClientLimit = policy.PendingLimit + 1
	if _, operationError := New("ws://127.0.0.1:1", providerInstance, providerInstance, nil, policy, nil); operationError == nil {
		testInstance.Fatal("invalid pending policy accepted")
	}
}
