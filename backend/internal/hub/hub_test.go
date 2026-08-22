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
	hubInstance := New(agent.websocketURL(), func() string { return testInstance.TempDir() }, nil, nil)
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
		"jsonrpc": "2.0", "id": "turn-1", "method": "session/prompt", "params": map[string]any{},
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

func TestReadTextFileKeepsLineEndingsAndHonorsExplicitZeroLimit(testInstance *testing.T) {
	path := filepath.Join(testInstance.TempDir(), "sample.txt")
	if operationError := os.WriteFile(path, []byte("one\r\ntwo\nthree"), 0o644); operationError != nil {
		testInstance.Fatal(operationError)
	}
	result, operationError := readTextFile(map[string]any{"path": path, "line": 2, "limit": 1})
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	if result["content"] != "two\n" {
		testInstance.Fatalf("content = %q", result["content"])
	}
	result, operationError = readTextFile(map[string]any{"path": path, "limit": 0})
	if operationError != nil || result["content"] != "" {
		testInstance.Fatalf("zero limit result=%#v err=%v", result, operationError)
	}
	result, operationError = readTextFile(map[string]any{"path": path, "line": 3})
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
	hubInstance := New("ws://127.0.0.1:1/ws", func() string { return "" }, func(context.Context) error {
		starts.Add(1)
		return nil
	}, nil)
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
