package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rezoch340/any-aicli-remote/backend/internal/loops"
	processapi "github.com/rezoch340/any-aicli-remote/backend/internal/process"
)

type delayedAgentOS struct {
	mutex             sync.Mutex
	port              int
	starts            int
	listenerProcessID int
	commands          map[int]string
	alive             map[int]bool
	stamps            map[int]string
	listener          net.Listener
	server            *http.Server
	connections       map[*websocket.Conn]struct{}
	started           chan struct{}
	received          chan []byte
	newSessionID      string
	startedOnce       sync.Once
	serveError        error
}

func newDelayedAgentOS(port int) *delayedAgentOS {
	return &delayedAgentOS{
		port: port, commands: map[int]string{}, alive: map[int]bool{}, stamps: map[int]string{},
		connections: map[*websocket.Conn]struct{}{}, started: make(chan struct{}), received: make(chan []byte, 8),
	}
}

func (delayedAgentEnvironment *delayedAgentOS) operations() processapi.Operations {
	return processapi.Operations{
		ListenProcessIDs: func(int, bool) ([]int, error) {
			delayedAgentEnvironment.mutex.Lock()
			defer delayedAgentEnvironment.mutex.Unlock()
			if delayedAgentEnvironment.listener == nil || delayedAgentEnvironment.listenerProcessID == 0 {
				return nil, nil
			}
			return []int{delayedAgentEnvironment.listenerProcessID}, nil
		},
		CommandLine: func(processID int) (string, error) {
			delayedAgentEnvironment.mutex.Lock()
			defer delayedAgentEnvironment.mutex.Unlock()
			return delayedAgentEnvironment.commands[processID], nil
		},
		ProcessAlive: func(processID int) bool {
			delayedAgentEnvironment.mutex.Lock()
			defer delayedAgentEnvironment.mutex.Unlock()
			return delayedAgentEnvironment.alive[processID]
		},
		ProcessStart: func(processID int) (string, error) {
			delayedAgentEnvironment.mutex.Lock()
			defer delayedAgentEnvironment.mutex.Unlock()
			return delayedAgentEnvironment.stamps[processID], nil
		},
		StartProcess: func(startSpecification processapi.StartSpecification) (int, error) {
			delayedAgentEnvironment.mutex.Lock()
			delayedAgentEnvironment.starts++
			processID := 8000 + delayedAgentEnvironment.starts
			delayedAgentEnvironment.commands[processID] = startSpecification.Path + " " + strings.Join(startSpecification.Arguments, " ")
			delayedAgentEnvironment.alive[processID] = true
			delayedAgentEnvironment.stamps[processID] = fmt.Sprintf("start-%d", processID)
			delayedAgentEnvironment.mutex.Unlock()
			delayedAgentEnvironment.startedOnce.Do(func() { close(delayedAgentEnvironment.started) })
			go delayedAgentEnvironment.serveAfterDelay(processID)
			return processID, nil
		},
		KillProcess: func(identity processapi.ProcessIdentity, _ time.Duration) error {
			delayedAgentEnvironment.mutex.Lock()
			delayedAgentEnvironment.alive[identity.ProcessID] = false
			delayedAgentEnvironment.mutex.Unlock()
			return nil
		},
		Now: time.Now,
	}
}

func (delayedAgentEnvironment *delayedAgentOS) serveAfterDelay(processID int) {
	time.Sleep(150 * time.Millisecond)
	listener, errorValue := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(delayedAgentEnvironment.port)))
	if errorValue != nil {
		delayedAgentEnvironment.mutex.Lock()
		delayedAgentEnvironment.serveError = errorValue
		delayedAgentEnvironment.mutex.Unlock()
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(responseWriter http.ResponseWriter, request *http.Request) {
		connection, upgradeError := upgrader.Upgrade(responseWriter, request, nil)
		if upgradeError != nil {
			return
		}
		delayedAgentEnvironment.mutex.Lock()
		delayedAgentEnvironment.connections[connection] = struct{}{}
		delayedAgentEnvironment.mutex.Unlock()
		defer func() {
			delayedAgentEnvironment.mutex.Lock()
			delete(delayedAgentEnvironment.connections, connection)
			delayedAgentEnvironment.mutex.Unlock()
			_ = connection.Close()
		}()
		for {
			_, payload, readError := connection.ReadMessage()
			if readError != nil {
				return
			}
			select {
			case delayedAgentEnvironment.received <- payload:
			default:
			}
			var request map[string]any
			if delayedAgentEnvironment.newSessionID != "" && json.Unmarshal(payload, &request) == nil && request["method"] == "session/new" && request["id"] != nil {
				_ = connection.WriteJSON(map[string]any{
					"jsonrpc": "2.0", "id": request["id"],
					"result": map[string]any{"sessionId": delayedAgentEnvironment.newSessionID},
				})
			}
		}
	})
	server := &http.Server{Handler: mux}
	delayedAgentEnvironment.mutex.Lock()
	delayedAgentEnvironment.listener = listener
	delayedAgentEnvironment.listenerProcessID = processID
	delayedAgentEnvironment.server = server
	delayedAgentEnvironment.mutex.Unlock()
	_ = server.Serve(listener)
}

func (delayedAgentEnvironment *delayedAgentOS) close() {
	delayedAgentEnvironment.mutex.Lock()
	server := delayedAgentEnvironment.server
	listener := delayedAgentEnvironment.listener
	connections := make([]*websocket.Conn, 0, len(delayedAgentEnvironment.connections))
	for connection := range delayedAgentEnvironment.connections {
		connections = append(connections, connection)
	}
	delayedAgentEnvironment.mutex.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
	if server != nil {
		_ = server.Close()
	} else if listener != nil {
		_ = listener.Close()
	}
}

func (delayedAgentEnvironment *delayedAgentOS) startCount() (int, error) {
	delayedAgentEnvironment.mutex.Lock()
	defer delayedAgentEnvironment.mutex.Unlock()
	return delayedAgentEnvironment.starts, delayedAgentEnvironment.serveError
}

func freeAgentPort(testingContext *testing.T) int {
	testingContext.Helper()
	listener, errorValue := net.Listen("tcp", "127.0.0.1:0")
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func TestConcurrentStackStartAndEnsureSpawnAgentOnce(testingContext *testing.T) {
	port := freeAgentPort(testingContext)
	fixture := newRouteTestServerWithAgentPort(testingContext, port)
	fake := newDelayedAgentOS(port)
	fixture.server.process.Operations = fake.operations()
	testingContext.Cleanup(fake.close)

	responses := make(chan int, 2)
	go func() {
		response := fixture.request(testingContext, http.MethodPost, "/api/stack/start", map[string]any{}, remotePeer, true)
		responses <- response.Code
	}()
	select {
	case <-fake.started:
	case <-time.After(2 * time.Second):
		testingContext.Fatal("first stack start did not spawn the delayed agent")
	}

	go func() {
		response := fixture.request(testingContext, http.MethodPost, "/api/stack/start", map[string]any{}, remotePeer, true)
		responses <- response.Code
	}()
	ensureDone := make(chan error, 1)
	go func() {
		executionContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		ensureDone <- fixture.server.ensureAgentProcess(executionContext)
	}()

	for range 2 {
		select {
		case status := <-responses:
			if status != http.StatusOK {
				testingContext.Fatalf("stack start status = %d, want 200", status)
			}
		case <-time.After(4 * time.Second):
			testingContext.Fatal("concurrent stack start timed out")
		}
	}
	if errorValue := <-ensureDone; errorValue != nil {
		testingContext.Fatalf("concurrent ensure: %v", errorValue)
	}
	if starts, serveError := fake.startCount(); serveError != nil || starts != 1 {
		testingContext.Fatalf("agent starts = %d, serve error = %v; want exactly one spawn", starts, serveError)
	}
}

func TestStartingAgentDoesNotCreateLoadOrResumeSession(testingContext *testing.T) {
	port := freeAgentPort(testingContext)
	fixture := newRouteTestServerWithAgentPort(testingContext, port)
	fake := newDelayedAgentOS(port)
	fixture.server.process.Operations = fake.operations()
	testingContext.Cleanup(fake.close)

	operationContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	operationError := fixture.server.ensureAgentProcess(operationContext)
	cancel()
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	select {
	case payload := <-fake.received:
		testingContext.Fatalf("idle startup sent provider RPC: %s", payload)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestServerRunWithPersistedLoopDoesNotSendProviderRPCAtStartup(testingContext *testing.T) {
	agentPort := freeAgentPort(testingContext)
	serverPort := freeAgentPort(testingContext)
	fixture := newRouteTestServerWithAgentPort(testingContext, agentPort)
	fake := newDelayedAgentOS(agentPort)
	fixture.server.process.Operations = fake.operations()
	fixture.server.configuration.Port = serverPort
	fixture.server.configuration.EnsureAgent = true
	testingContext.Cleanup(fake.close)

	if _, operationError := fixture.server.loops.Create("persisted-session", "scheduled prompt", loops.MinInterval, "", ""); operationError != nil {
		testingContext.Fatal(operationError)
	}
	executionContext, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() {
		runResult <- fixture.server.Run(executionContext)
	}()
	select {
	case <-fake.started:
	case <-time.After(2 * time.Second):
		cancel()
		testingContext.Fatal("daemon did not start its idle provider service")
	}

	select {
	case payload := <-fake.received:
		cancel()
		testingContext.Fatalf("daemon startup sent provider RPC for persisted loop: %s", payload)
	case <-time.After(400 * time.Millisecond):
	}
	cancel()
	select {
	case operationError := <-runResult:
		if operationError != nil {
			testingContext.Fatalf("daemon run returned error: %v", operationError)
		}
	case <-time.After(3 * time.Second):
		testingContext.Fatal("daemon did not stop after context cancellation")
	}
}

func TestNewSessionAppearsInSessionsEndpointBeforeProviderPersistence(testingContext *testing.T) {
	port := freeAgentPort(testingContext)
	fixture := newRouteTestServerWithAgentPort(testingContext, port)
	fake := newDelayedAgentOS(port)
	fake.newSessionID = "active-session-without-summary"
	fixture.server.process.Operations = fake.operations()
	testingContext.Cleanup(fake.close)

	startResponse := fixture.request(testingContext, http.MethodPost, "/api/stack/start", map[string]any{}, remotePeer, true)
	assertStatus(testingContext, startResponse, http.StatusOK)
	httpServer := httptest.NewServer(fixture.handler)
	defer httpServer.Close()
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?key=" + routeTestSecret
	connection, _, operationError := websocket.DefaultDialer.Dial(websocketURL, nil)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	defer connection.Close()
	workspace := testingContext.TempDir()
	if operationError := connection.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "mobile-new", "method": "session/new", "params": map[string]any{"cwd": workspace},
	}); operationError != nil {
		testingContext.Fatal(operationError)
	}
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		var response map[string]any
		if operationError := connection.ReadJSON(&response); operationError != nil {
			testingContext.Fatal(operationError)
		}
		if response["id"] == "mobile-new" && response["method"] == nil {
			if response["error"] != nil {
				testingContext.Fatalf("new session response = %#v", response)
			}
			break
		}
	}

	sessionsResponse := fixture.request(testingContext, http.MethodGet, "/api/sessions", nil, remotePeer, true)
	assertStatus(testingContext, sessionsResponse, http.StatusOK)
	sessionsBody := decodeObject(testingContext, sessionsResponse)
	if sessionsBody["providerId"] != "grok" {
		testingContext.Fatalf("providerId = %#v", sessionsBody["providerId"])
	}
	canonicalWorkspace, _ := filepath.EvalSymlinks(workspace)
	found := false
	for _, rawSession := range sessionsBody["sessions"].([]any) {
		metadata, _ := rawSession.(map[string]any)
		if metadata["sessionId"] == fake.newSessionID {
			found = metadata["providerId"] == "grok" && metadata["projectDir"] == canonicalWorkspace
			break
		}
	}
	if !found {
		testingContext.Fatalf("active session missing before summary persistence: %#v", sessionsBody)
	}
	messagesResponse := fixture.request(testingContext, http.MethodGet, "/api/sessions/"+fake.newSessionID+"/messages?providerId=grok", nil, remotePeer, true)
	assertStatus(testingContext, messagesResponse, http.StatusOK)
	messagesBody := decodeObject(testingContext, messagesResponse)
	if messagesBody["providerId"] != "grok" || messagesBody["sessionId"] != fake.newSessionID || messagesBody["count"] != float64(0) {
		testingContext.Fatalf("active session messages response = %#v", messagesBody)
	}
	messageSession, _ := messagesBody["session"].(map[string]any)
	if messageSession["projectDir"] != canonicalWorkspace {
		testingContext.Fatalf("active session message metadata = %#v", messageSession)
	}
}

func TestTimedOutEnsureDoesNotRespawnStartingAgent(testingContext *testing.T) {
	port := freeAgentPort(testingContext)
	fixture := newRouteTestServerWithAgentPort(testingContext, port)
	fake := newDelayedAgentOS(port)
	fixture.server.process.Operations = fake.operations()
	testingContext.Cleanup(fake.close)

	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	errorValue := fixture.server.ensureAgentProcess(short)
	cancel()
	if errorValue == nil {
		testingContext.Fatal("short ensure unexpectedly succeeded")
	}
	select {
	case <-fake.started:
	default:
		testingContext.Fatal("short ensure did not spawn the first agent")
	}

	patient, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	errorValue = fixture.server.ensureAgentProcess(patient)
	cancel()
	if errorValue != nil {
		testingContext.Fatalf("patient ensure: %v", errorValue)
	}
	if starts, serveError := fake.startCount(); serveError != nil || starts != 1 {
		testingContext.Fatalf("agent starts = %d, serve error = %v; want the in-flight child reused", starts, serveError)
	}
}

func TestEnsureCannotSpawnAfterServerClose(testingContext *testing.T) {
	port := freeAgentPort(testingContext)
	fixture := newRouteTestServerWithAgentPort(testingContext, port)
	fake := newDelayedAgentOS(port)
	fixture.server.process.Operations = fake.operations()
	testingContext.Cleanup(fake.close)
	fixture.server.Close()

	errorValue := fixture.server.ensureAgentProcess(context.Background())
	if errorValue == nil || !strings.Contains(errorValue.Error(), "stopping") {
		testingContext.Fatalf("ensure after close error = %v", errorValue)
	}
	if starts, _ := fake.startCount(); starts != 0 {
		testingContext.Fatalf("ensure after close spawned %d agents", starts)
	}
	restartAttempt, restartError := fixture.server.restartOwnedAgentForAuthentication(context.Background())
	if restartError == nil || restartAttempt.attempted {
		testingContext.Fatalf("authentication retry after close = %+v, error = %v", restartAttempt, restartError)
	}
	if starts, _ := fake.startCount(); starts != 0 {
		testingContext.Fatalf("authentication retry after close spawned %d agents", starts)
	}
}

func TestCloseWaitsForAgentLifecycleTransaction(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	fixture.server.agentLifecycleMutex.Lock()
	closeFinished := make(chan struct{})
	go func() {
		fixture.server.Close()
		close(closeFinished)
	}()

	deadline := time.Now().Add(time.Second)
	for !fixture.server.closing.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !fixture.server.closing.Load() {
		fixture.server.agentLifecycleMutex.Unlock()
		testingContext.Fatal("Close did not publish the stopping state")
	}
	select {
	case <-closeFinished:
		fixture.server.agentLifecycleMutex.Unlock()
		testingContext.Fatal("Close returned before the lifecycle transaction completed")
	case <-time.After(30 * time.Millisecond):
	}
	fixture.server.agentLifecycleMutex.Unlock()
	select {
	case <-closeFinished:
	case <-time.After(time.Second):
		testingContext.Fatal("Close did not finish after the lifecycle transaction released")
	}
}
