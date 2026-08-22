package hub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func waitForTestCondition(testInstance *testing.T, description string, condition func() bool) {
	testInstance.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !condition() {
		testInstance.Fatalf("timed out waiting for %s", description)
	}
}

func TestCloseLinearizesConcurrentAgentPublication(testInstance *testing.T) {
	hubInstance := New("ws://unused.invalid", func() string { return "" }, nil, nil)
	hubInstance.agentMutex.Lock()

	publicationStarted := make(chan struct{})
	publicationResult := make(chan bool, 1)
	go func() {
		close(publicationStarted)
		publicationResult <- hubInstance.publishAgent(nil)
	}()
	<-publicationStarted

	closeReturned := make(chan struct{})
	go func() {
		hubInstance.Close()
		close(closeReturned)
	}()
	waitForTestCondition(testInstance, "hub closing flag", hubInstance.closed.Load)
	hubInstance.agentMutex.Unlock()

	if published := <-publicationResult; published {
		testInstance.Fatal("agent connection published after close began")
	}
	select {
	case <-closeReturned:
	case <-time.After(2 * time.Second):
		testInstance.Fatal("close did not finish after concurrent publication stopped")
	}
	if hubInstance.AgentConnected() {
		testInstance.Fatal("agent remained connected after close")
	}
}

func TestCloseReturnsWithNoConnectionsClientsTerminalsOrReverseTasks(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := New(agent.websocketURL(), func() string { return testInstance.TempDir() }, nil, nil)
	clientConnection, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()
	waitForTestCondition(testInstance, "upstream agent connection", hubInstance.AgentConnected)

	created := reverseCall(testInstance, agent, "close-terminal-create", "terminal/create", map[string]any{
		"command": "sleep 30",
	})
	terminalIdentifier, valid := rpcResult(testInstance, created)["terminalId"].(string)
	if !valid || terminalIdentifier == "" {
		testInstance.Fatalf("terminal/create result = %#v", created)
	}
	if terminalCount := hubInstance.terminals.count(); terminalCount != 1 {
		testInstance.Fatalf("terminal count before close = %d, want 1", terminalCount)
	}

	agent.send(testInstance, map[string]any{
		"jsonrpc": "2.0",
		"id":      "close-terminal-wait",
		"method":  "terminal/wait_for_exit",
		"params":  map[string]any{"terminalId": terminalIdentifier},
	})
	waitForTestCondition(testInstance, "blocked reverse task", func() bool {
		return hubInstance.reverseActive.Load() == 1
	})

	hubInstance.Close()
	if hubInstance.AgentConnected() {
		testInstance.Fatal("upstream agent remained connected")
	}
	if clientCount := hubInstance.ClientCount(); clientCount != 0 {
		testInstance.Fatalf("client count after close = %d", clientCount)
	}
	if terminalCount := hubInstance.terminals.count(); terminalCount != 0 {
		testInstance.Fatalf("terminal count after close = %d", terminalCount)
	}
	if reverseTaskCount := hubInstance.reverseActive.Load(); reverseTaskCount != 0 {
		testInstance.Fatalf("reverse task count after close = %d", reverseTaskCount)
	}

	_ = clientConnection.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	for {
		_, _, readFailure := clientConnection.ReadMessage()
		if readFailure != nil {
			break
		}
	}
	if operationError := hubInstance.Ensure(context.Background()); operationError == nil {
		testInstance.Fatal("closed hub accepted ensure")
	}
	if _, operationError := hubInstance.terminals.create(map[string]any{"command": "sleep 30"}); operationError == nil {
		testInstance.Fatal("closed terminal manager started a terminal")
	}
}

func TestClosedHubRejectsNewWebSocketBeforeUpgrade(testInstance *testing.T) {
	hubInstance := New("ws://unused.invalid", func() string { return "" }, nil, nil)
	hubInstance.Close()
	server := httptest.NewServer(http.HandlerFunc(hubInstance.HandleWebSocket))
	defer server.Close()

	connection, response, dialFailure := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], nil)
	if connection != nil {
		_ = connection.Close()
		testInstance.Fatal("closed hub upgraded a new websocket")
	}
	if dialFailure == nil {
		testInstance.Fatal("closed hub websocket dial unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		testInstance.Fatalf("closed hub response = %#v, want HTTP 503", response)
	}
	_ = response.Body.Close()
	if clientCount := hubInstance.ClientCount(); clientCount != 0 {
		testInstance.Fatalf("closed hub registered %d clients", clientCount)
	}
}

func TestTerminalCreateCannotCrossCloseBoundary(testInstance *testing.T) {
	managerInstance := newTerminalManager()
	managerInstance.accessMutex.Lock()
	closeReturned := make(chan struct{})
	go func() {
		managerInstance.close()
		close(closeReturned)
	}()
	waitForTestCondition(testInstance, "terminal manager closing flag", managerInstance.closed.Load)

	createReturned := make(chan error, 1)
	go func() {
		_, operationError := managerInstance.create(map[string]any{"command": "sleep 30"})
		createReturned <- operationError
	}()
	if operationError := <-createReturned; operationError == nil {
		managerInstance.accessMutex.Unlock()
		testInstance.Fatal("terminal create succeeded after close began")
	}
	managerInstance.accessMutex.Unlock()

	select {
	case <-closeReturned:
	case <-time.After(2 * time.Second):
		testInstance.Fatal("terminal manager close did not finish")
	}
	if terminalCount := managerInstance.count(); terminalCount != 0 {
		testInstance.Fatalf("terminal count after concurrent close = %d", terminalCount)
	}
}

func TestIdleClientStaysConnectedThroughProtocolHeartbeat(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := New(agent.websocketURL(), func() string { return testInstance.TempDir() }, nil, nil)
	hubInstance.heartbeatInterval = 10 * time.Millisecond
	hubInstance.clientReadTimeout = 80 * time.Millisecond
	defer hubInstance.Close()

	connection, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()
	heartbeatReceived := make(chan struct{}, 1)
	connection.SetPingHandler(func(payload string) error {
		select {
		case heartbeatReceived <- struct{}{}:
		default:
		}
		return connection.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(time.Second))
	})
	readComplete := make(chan struct{})
	go func() {
		defer close(readComplete)
		for {
			if _, _, operationError := connection.ReadMessage(); operationError != nil {
				return
			}
		}
	}()

	select {
	case <-heartbeatReceived:
	case <-time.After(time.Second):
		testInstance.Fatal("idle client did not receive a protocol heartbeat")
	}
	time.Sleep(3 * hubInstance.clientReadTimeout)
	if clientCount := hubInstance.ClientCount(); clientCount != 1 {
		testInstance.Fatalf("idle client disconnected despite heartbeat; clients = %d", clientCount)
	}

	closeClient()
	select {
	case <-readComplete:
	case <-time.After(time.Second):
		testInstance.Fatal("client reader did not stop after close")
	}
}
