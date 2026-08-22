package hub

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxMessageSize = 16 * 1024 * 1024
	maxReadSize    = 2_000_000
	maxWriteSize   = 4_000_000
)

type EnsureAgentFunc func(context.Context) error

type clientConnection struct {
	connection   *websocket.Conn
	write        sync.Mutex
	closed       atomic.Bool
	closedSignal chan struct{}
}

func (clientConnectionInstance *clientConnection) send(data []byte) error {
	if clientConnectionInstance == nil || clientConnectionInstance.closed.Load() {
		return errors.New("client closed")
	}
	clientConnectionInstance.write.Lock()
	defer clientConnectionInstance.write.Unlock()
	_ = clientConnectionInstance.connection.SetWriteDeadline(time.Now().Add(20 * time.Second))
	return clientConnectionInstance.connection.WriteMessage(websocket.TextMessage, data)
}

func (clientConnectionInstance *clientConnection) close() {
	if clientConnectionInstance != nil && clientConnectionInstance.closed.CompareAndSwap(false, true) {
		close(clientConnectionInstance.closedSignal)
		_ = clientConnectionInstance.connection.Close()
	}
}

func (clientConnectionInstance *clientConnection) sendControl(messageType int, payload []byte) error {
	if clientConnectionInstance == nil || clientConnectionInstance.closed.Load() {
		return errors.New("client closed")
	}
	clientConnectionInstance.write.Lock()
	defer clientConnectionInstance.write.Unlock()
	return clientConnectionInstance.connection.WriteControl(messageType, payload, time.Now().Add(5*time.Second))
}

type pendingRequest struct {
	client   *clientConnection
	original any
	init     bool
	detached bool
}

type Hub struct {
	agentURL    string
	ensureAgent EnsureAgentFunc
	workspace   func() string
	logger      *slog.Logger

	upgrader websocket.Upgrader

	connectionMutex sync.Mutex
	agentMutex      sync.RWMutex
	agent           *websocket.Conn
	agentWriteMutex sync.Mutex
	lastError       string

	stateMutex  sync.Mutex
	clients     map[*clientConnection]struct{}
	pending     map[int64]pendingRequest
	internal    map[int64]chan map[string]any
	reverseIDs  map[string]struct{}
	nextID      int64
	initResult  any
	initCached  bool
	watchCancel context.CancelFunc
	terminals   *terminalManager
	closed      atomic.Bool

	lifetimeContext   context.Context
	lifetimeCancel    context.CancelFunc
	reverseWait       sync.WaitGroup
	reverseActive     atomic.Int64
	closeComplete     chan struct{}
	heartbeatInterval time.Duration
	clientReadTimeout time.Duration
}

func New(agentURL string, workspace func() string, ensureAgent EnsureAgentFunc, logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	lifetimeContext, lifetimeCancel := context.WithCancel(context.Background())
	return &Hub{
		agentURL:    agentURL,
		ensureAgent: ensureAgent,
		workspace:   workspace,
		logger:      logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  64 * 1024,
			WriteBufferSize: 64 * 1024,
			CheckOrigin:     func(*http.Request) bool { return true },
		},
		clients:           make(map[*clientConnection]struct{}),
		pending:           make(map[int64]pendingRequest),
		internal:          make(map[int64]chan map[string]any),
		reverseIDs:        make(map[string]struct{}),
		terminals:         newTerminalManager(),
		lifetimeContext:   lifetimeContext,
		lifetimeCancel:    lifetimeCancel,
		closeComplete:     make(chan struct{}),
		heartbeatInterval: 20 * time.Second,
		clientReadTimeout: 60 * time.Second,
	}
}

func (hubInstance *Hub) Start(parent context.Context) {
	if hubInstance.closed.Load() {
		return
	}
	hubInstance.stateMutex.Lock()
	if hubInstance.closed.Load() || hubInstance.watchCancel != nil {
		hubInstance.stateMutex.Unlock()
		return
	}
	operationContext, cancel := context.WithCancel(parent)
	hubInstance.watchCancel = cancel
	hubInstance.stateMutex.Unlock()
	go hubInstance.watch(operationContext)
}

func (hubInstance *Hub) Close() {
	if !hubInstance.closed.CompareAndSwap(false, true) {
		<-hubInstance.closeComplete
		return
	}
	defer close(hubInstance.closeComplete)
	hubInstance.lifetimeCancel()
	hubInstance.stateMutex.Lock()
	if hubInstance.watchCancel != nil {
		hubInstance.watchCancel()
		hubInstance.watchCancel = nil
	}
	clients := make([]*clientConnection, 0, len(hubInstance.clients))
	for client := range hubInstance.clients {
		clients = append(clients, client)
	}
	hubInstance.clients = make(map[*clientConnection]struct{})
	hubInstance.stateMutex.Unlock()
	for _, client := range clients {
		client.close()
	}
	// Ensure owns connectionMutex from its first closed check through publishing
	// a successful dial. Waiting here makes Close a barrier: no in-flight ensure
	// can publish an upstream connection after Close returns.
	hubInstance.connectionMutex.Lock()
	hubInstance.disconnectAgent(errors.New("hub closed"))
	hubInstance.connectionMutex.Unlock()
	hubInstance.terminals.close()
	hubInstance.reverseWait.Wait()
}

func (hubInstance *Hub) HandleWebSocket(responseWriter http.ResponseWriter, request *http.Request) {
	if hubInstance.closed.Load() {
		http.Error(responseWriter, "hub closed", http.StatusServiceUnavailable)
		return
	}
	connection, operationError := hubInstance.upgrader.Upgrade(responseWriter, request, nil)
	if operationError != nil {
		hubInstance.logger.Warn("client websocket upgrade failed", "error", operationError)
		return
	}
	connection.SetReadLimit(maxMessageSize)
	client := &clientConnection{connection: connection, closedSignal: make(chan struct{})}
	hubInstance.stateMutex.Lock()
	if hubInstance.closed.Load() {
		hubInstance.stateMutex.Unlock()
		client.close()
		return
	}
	hubInstance.clients[client] = struct{}{}
	count := len(hubInstance.clients)
	hubInstance.stateMutex.Unlock()
	hubInstance.logger.Info("remote client connected", "clients", count, "peer", request.RemoteAddr)
	defer hubInstance.removeClient(client)
	refreshReadDeadline := func() {
		_ = connection.SetReadDeadline(time.Now().Add(hubInstance.clientReadTimeout))
	}
	connection.SetPongHandler(func(string) error {
		refreshReadDeadline()
		return nil
	})
	connection.SetPingHandler(func(payload string) error {
		refreshReadDeadline()
		return client.sendControl(websocket.PongMessage, []byte(payload))
	})
	go hubInstance.keepClientAlive(client)

	operationContext, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	_ = hubInstance.Ensure(operationContext)
	cancel()

	for {
		refreshReadDeadline()
		messageType, raw, operationError := connection.ReadMessage()
		if operationError != nil {
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		hubInstance.handleClientMessage(client, raw)
	}
}

func (hubInstance *Hub) keepClientAlive(client *clientConnection) {
	heartbeatTicker := time.NewTicker(hubInstance.heartbeatInterval)
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-hubInstance.lifetimeContext.Done():
			return
		case <-client.closedSignal:
			return
		case <-heartbeatTicker.C:
			if operationError := client.sendControl(websocket.PingMessage, nil); operationError != nil {
				client.close()
				return
			}
		}
	}
}

func (hubInstance *Hub) Ensure(operationContext context.Context) error {
	if hubInstance.closed.Load() {
		return errors.New("hub closed")
	}
	ensureContext, ensureCancel := context.WithCancel(operationContext)
	stopLifetimeCancellation := context.AfterFunc(hubInstance.lifetimeContext, ensureCancel)
	defer func() {
		stopLifetimeCancellation()
		ensureCancel()
	}()
	operationContext = ensureContext

	hubInstance.connectionMutex.Lock()
	defer hubInstance.connectionMutex.Unlock()
	if hubInstance.closed.Load() {
		return errors.New("hub closed")
	}
	if hubInstance.AgentConnected() {
		return nil
	}

	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		if hubInstance.closed.Load() {
			return errors.New("hub closed")
		}
		if attempt == 1 && hubInstance.ensureAgent != nil {
			if operationError := hubInstance.ensureAgent(operationContext); operationError != nil {
				last = operationError
			}
		}
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		if hubInstance.closed.Load() {
			return errors.New("hub closed")
		}
		dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second, EnableCompression: true}
		connection, _, operationError := dialer.DialContext(operationContext, hubInstance.agentURL, nil)
		if operationError == nil {
			connection.SetReadLimit(maxMessageSize)
			if !hubInstance.publishAgent(connection) {
				_ = connection.Close()
				return errors.New("hub closed")
			}
			hubInstance.logger.Info("upstream agent connected")
			go hubInstance.readAgent(connection)
			hubInstance.broadcastHubState(true)
			return nil
		}
		last = operationError
		hubInstance.agentMutex.Lock()
		hubInstance.lastError = redactAgentURL(operationError.Error())
		hubInstance.agentMutex.Unlock()
		select {
		case <-operationContext.Done():
			return operationContext.Err()
		case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
		}
	}
	if last == nil {
		last = errors.New("agent unavailable")
	}
	return last
}

func (hubInstance *Hub) AgentConnected() bool {
	hubInstance.agentMutex.RLock()
	connected := hubInstance.agent != nil
	hubInstance.agentMutex.RUnlock()
	return connected
}

func (hubInstance *Hub) ClientCount() int {
	hubInstance.stateMutex.Lock()
	defer hubInstance.stateMutex.Unlock()
	return len(hubInstance.clients)
}

func (hubInstance *Hub) InitCached() bool {
	hubInstance.stateMutex.Lock()
	defer hubInstance.stateMutex.Unlock()
	return hubInstance.initCached
}

func (hubInstance *Hub) LastError() string {
	hubInstance.agentMutex.RLock()
	defer hubInstance.agentMutex.RUnlock()
	return hubInstance.lastError
}

// Notify broadcasts a daemon-owned JSON-RPC notification to all native clients.
func (hubInstance *Hub) Notify(method string, params map[string]any) {
	if method == "" {
		return
	}
	hubInstance.broadcastJSON(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// DisconnectAgent drops only the upstream connection. The watcher may reconnect it.
func (hubInstance *Hub) DisconnectAgent(reason string) {
	if reason == "" {
		reason = "agent reset"
	}
	hubInstance.disconnectAgent(errors.New(reason))
}

func (hubInstance *Hub) CallRPC(operationContext context.Context, method string, params map[string]any) (map[string]any, error) {
	if operationError := hubInstance.Ensure(operationContext); operationError != nil {
		return nil, fmt.Errorf("agent offline: %w", operationError)
	}
	identifier := atomic.AddInt64(&hubInstance.nextID, 1)
	response := make(chan map[string]any, 1)
	hubInstance.stateMutex.Lock()
	hubInstance.internal[identifier] = response
	hubInstance.stateMutex.Unlock()
	payload := map[string]any{"jsonrpc": "2.0", "id": identifier, "method": method, "params": params}
	if operationError := hubInstance.sendAgentJSON(payload); operationError != nil {
		hubInstance.stateMutex.Lock()
		delete(hubInstance.internal, identifier)
		hubInstance.stateMutex.Unlock()
		return nil, operationError
	}
	select {
	case <-operationContext.Done():
		hubInstance.stateMutex.Lock()
		delete(hubInstance.internal, identifier)
		hubInstance.stateMutex.Unlock()
		return nil, operationContext.Err()
	case result := <-response:
		if rpcError, present := result["error"].(map[string]any); present {
			return result, fmt.Errorf("%s", stringValue(rpcError["message"]))
		}
		return result, nil
	}
}

func (hubInstance *Hub) readAgent(connection *websocket.Conn) {
	var readFailure error
	defer func() { hubInstance.agentDisconnected(connection, readFailure) }()
	for {
		messageType, raw, operationError := connection.ReadMessage()
		if operationError != nil {
			readFailure = operationError
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		hubInstance.handleAgentMessage(raw)
	}
}

func (hubInstance *Hub) handleAgentMessage(raw []byte) {
	var object map[string]any
	if operationError := json.Unmarshal(raw, &object); operationError != nil {
		hubInstance.broadcast(raw)
		return
	}
	identifier, hasID := object["id"]
	method, _ := object["method"].(string)
	if hasID && method == "" {
		numericID, present := numericID(identifier)
		if !present {
			return
		}
		hubInstance.stateMutex.Lock()
		if response := hubInstance.internal[numericID]; response != nil {
			delete(hubInstance.internal, numericID)
			hubInstance.stateMutex.Unlock()
			response <- object
			return
		}
		pending, found := hubInstance.pending[numericID]
		if found {
			delete(hubInstance.pending, numericID)
			if pending.init {
				if result, present := object["result"]; present {
					hubInstance.initResult = result
					hubInstance.initCached = true
				}
			}
		}
		hubInstance.stateMutex.Unlock()
		if !found {
			return
		}
		if pending.client != nil && !pending.detached {
			object["id"] = pending.original
			_ = pending.client.send(mustJSON(object))
		} else {
			hubInstance.broadcastJSON(map[string]any{
				"jsonrpc": "2.0", "method": "_x.ai/remote/rpc_done",
				"params": map[string]any{"id": pending.original, "ok": object["error"] == nil, "detached": true},
			})
		}
		return
	}
	if hasID && method != "" {
		if hubInstance.handleReverseAsync(object) {
			return
		}
		hubInstance.stateMutex.Lock()
		hubInstance.reverseIDs[idKey(identifier)] = struct{}{}
		hubInstance.stateMutex.Unlock()
	}
	hubInstance.broadcast(raw)
}

func (hubInstance *Hub) handleClientMessage(client *clientConnection, raw []byte) {
	var object map[string]any
	if operationError := json.Unmarshal(raw, &object); operationError != nil {
		if operationError := hubInstance.ensureAndSend(raw, false); operationError != nil {
			hubInstance.sendRPCError(client, nil, operationError.Error(), -32001)
		}
		return
	}
	method, _ := object["method"].(string)
	identifier, hasID := object["id"]
	if method == "initialize" {
		ensureToolCapabilities(object)
	}
	if method == "_x.ai/remote/ping" {
		params, _ := object["params"].(map[string]any)
		hubInstance.broadcastPong(client, params["t"])
		return
	}
	if method == "initialize" && hasID {
		hubInstance.stateMutex.Lock()
		cached := hubInstance.initCached
		result := hubInstance.initResult
		hubInstance.stateMutex.Unlock()
		if cached {
			_ = client.send(mustJSON(map[string]any{"jsonrpc": "2.0", "id": identifier, "result": result}))
			return
		}
	}
	if hasID && method != "" {
		timeout := 5 * time.Second
		if method == "initialize" || method == "session/load" || method == "session/new" {
			timeout = 18 * time.Second
		}
		operationContext, cancel := context.WithTimeout(context.Background(), timeout)
		operationError := hubInstance.Ensure(operationContext)
		cancel()
		if operationError != nil {
			hubInstance.sendRPCError(client, identifier, "agent offline: "+redactAgentURL(operationError.Error()), -32001)
			return
		}
		hubID := atomic.AddInt64(&hubInstance.nextID, 1)
		hubInstance.stateMutex.Lock()
		hubInstance.pending[hubID] = pendingRequest{client: client, original: identifier, init: method == "initialize"}
		hubInstance.stateMutex.Unlock()
		object["id"] = hubID
		if operationError := hubInstance.sendAgentJSON(object); operationError != nil {
			hubInstance.stateMutex.Lock()
			delete(hubInstance.pending, hubID)
			hubInstance.stateMutex.Unlock()
			hubInstance.sendRPCError(client, identifier, operationError.Error(), -32001)
		}
		return
	}
	if hasID && method == "" {
		key := idKey(identifier)
		hubInstance.stateMutex.Lock()
		_, known := hubInstance.reverseIDs[key]
		if known {
			delete(hubInstance.reverseIDs, key)
		}
		hubInstance.stateMutex.Unlock()
		if !known {
			return
		}
	}
	if operationError := hubInstance.ensureAndSend(mustJSON(object), method == "initialize" || method == "session/load" || method == "session/new"); operationError != nil {
		hubInstance.sendRPCError(client, identifier, operationError.Error(), -32001)
	}
}

func (hubInstance *Hub) ensureAndSend(raw []byte, patient bool) error {
	timeout := 3 * time.Second
	if patient {
		timeout = 18 * time.Second
	}
	operationContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if operationError := hubInstance.Ensure(operationContext); operationError != nil {
		return operationError
	}
	return hubInstance.sendAgent(raw)
}

func (hubInstance *Hub) handleReverseAsync(object map[string]any) bool {
	method, _ := object["method"].(string)
	known := method == "fs/read_text_file" || method == "fs/readTextFile" ||
		method == "fs/write_text_file" || method == "fs/writeTextFile" ||
		strings.HasPrefix(method, "terminal/") || isPermissionMethod(method)
	if !known {
		return false
	}
	hubInstance.stateMutex.Lock()
	if hubInstance.closed.Load() {
		hubInstance.stateMutex.Unlock()
		return true
	}
	hubInstance.reverseWait.Add(1)
	hubInstance.reverseActive.Add(1)
	hubInstance.stateMutex.Unlock()
	go func() {
		defer hubInstance.reverseWait.Done()
		defer hubInstance.reverseActive.Add(-1)
		hubInstance.handleReverse(hubInstance.lifetimeContext, object)
	}()
	return true
}

func (hubInstance *Hub) handleReverse(operationContext context.Context, object map[string]any) {
	identifier := object["id"]
	method, _ := object["method"].(string)
	params, _ := object["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	if operationError := operationContext.Err(); operationError != nil {
		return
	}
	var result map[string]any
	var autoPermission map[string]any
	var operationError error

	switch method {
	case "fs/read_text_file", "fs/readTextFile":
		result, operationError = readTextFile(params)
	case "fs/write_text_file", "fs/writeTextFile":
		result, operationError = writeTextFile(params)
	case "terminal/create":
		result, operationError = hubInstance.terminals.create(params)
	case "terminal/output":
		result, operationError = hubInstance.terminals.output(params)
	case "terminal/wait_for_exit", "terminal/waitForExit":
		result, operationError = hubInstance.terminals.waitForExit(operationContext, params)
	case "terminal/kill":
		result, operationError = hubInstance.terminals.kill(params)
	case "terminal/release":
		result, operationError = hubInstance.terminals.release(params)
	default:
		if isPermissionMethod(method) {
			option := permissionOption(params)
			result = map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": option}}
			var tool any
			if toolCall, present := params["toolCall"].(map[string]any); present {
				tool = toolCall["title"]
			}
			autoPermission = map[string]any{"optionId": option, "tool": tool}
		}
	}
	if operationError != nil {
		hubInstance.replyAgent(map[string]any{"jsonrpc": "2.0", "id": identifier, "error": map[string]any{"code": -32000, "message": operationError.Error()}})
		return
	}
	hubInstance.replyAgent(map[string]any{"jsonrpc": "2.0", "id": identifier, "result": result})
	if autoPermission != nil {
		hubInstance.broadcastJSON(map[string]any{
			"jsonrpc": "2.0", "method": "_x.ai/remote/auto_permission",
			"params": autoPermission,
		})
	}
	var clientRPC map[string]any
	switch method {
	case "fs/read_text_file", "fs/readTextFile", "fs/write_text_file", "fs/writeTextFile":
		clientRPC = map[string]any{"method": method, "path": stringValue(params["path"]), "ok": true}
	case "terminal/create":
		clientRPC = map[string]any{
			"method": method, "terminalId": result["terminalId"],
			"command": stringValue(params["command"]), "ok": true,
		}
	}
	if clientRPC != nil {
		hubInstance.broadcastJSON(map[string]any{
			"jsonrpc": "2.0", "method": "_x.ai/remote/client_rpc",
			"params": clientRPC,
		})
	}
}

func readTextFile(params map[string]any) (map[string]any, error) {
	path := stringValue(params["path"])
	if path == "" {
		return nil, errors.New("path required")
	}
	file, operationError := os.Open(path)
	if operationError != nil {
		return nil, operationError
	}
	defer file.Close()
	start := intValue(params["line"], 1)
	if start < 1 {
		start = 1
	}
	limitValue, limitProvided := params["limit"]
	limit := intValue(limitValue, 0)
	if limitProvided && limit < 0 {
		limit = 0
	}
	if limitProvided && limit == 0 {
		return map[string]any{"content": ""}, nil
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	var builder strings.Builder
	line := 1
	writtenLines := 0
	for {
		fragment, operationError := reader.ReadString('\n')
		include := line >= start && (!limitProvided || writtenLines < limit)
		if include && builder.Len() < maxReadSize {
			remaining := maxReadSize - builder.Len()
			if len(fragment) > remaining {
				fragment = fragment[:remaining]
			}
			builder.WriteString(fragment)
		}
		if include {
			writtenLines++
		}
		if builder.Len() >= maxReadSize || (limitProvided && writtenLines >= limit) {
			break
		}
		if operationError != nil {
			if !errors.Is(operationError, io.EOF) {
				return nil, operationError
			}
			break
		}
		line++
	}
	return map[string]any{"content": strings.ToValidUTF8(builder.String(), "�")}, nil
}

func writeTextFile(params map[string]any) (map[string]any, error) {
	path := stringValue(params["path"])
	if path == "" {
		return nil, errors.New("path required")
	}
	content := stringValue(params["content"])
	if len([]byte(content)) > maxWriteSize {
		return nil, errors.New("content too large")
	}
	if operationError := os.MkdirAll(filepath.Dir(path), 0755); operationError != nil {
		return nil, operationError
	}
	if operationError := os.WriteFile(path, []byte(content), 0644); operationError != nil {
		return nil, operationError
	}
	return map[string]any{}, nil
}

func isPermissionMethod(method string) bool {
	lower := strings.ToLower(method)
	return method == "session/request_permission" || method == "session/requestPermission" ||
		strings.Contains(lower, "permission") || strings.Contains(lower, "ask_user")
}

func permissionOption(params map[string]any) string {
	options, _ := params["options"].([]any)
	first := "allow"
	for index, raw := range options {
		option, present := raw.(map[string]any)
		if !present {
			continue
		}
		identifier := stringValue(option["optionId"])
		if identifier == "" {
			identifier = stringValue(option["id"])
		}
		if index == 0 && identifier != "" {
			first = identifier
		}
		lower := strings.ToLower(identifier)
		if strings.Contains(lower, "allow") || strings.Contains(lower, "approve") || strings.Contains(lower, "yes") || strings.Contains(lower, "accept") {
			return identifier
		}
	}
	return first
}

func ensureToolCapabilities(request map[string]any) {
	params, _ := request["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
		request["params"] = params
	}
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

func (hubInstance *Hub) sendAgentJSON(value any) error { return hubInstance.sendAgent(mustJSON(value)) }

func (hubInstance *Hub) publishAgent(connection *websocket.Conn) bool {
	hubInstance.agentMutex.Lock()
	defer hubInstance.agentMutex.Unlock()
	if hubInstance.closed.Load() {
		return false
	}
	hubInstance.agent = connection
	hubInstance.lastError = ""
	return true
}

func (hubInstance *Hub) sendAgent(raw []byte) error {
	hubInstance.agentMutex.RLock()
	connection := hubInstance.agent
	hubInstance.agentMutex.RUnlock()
	if connection == nil {
		return errors.New("agent disconnected")
	}
	hubInstance.agentWriteMutex.Lock()
	defer hubInstance.agentWriteMutex.Unlock()
	_ = connection.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if operationError := connection.WriteMessage(websocket.TextMessage, raw); operationError != nil {
		return fmt.Errorf("send agent: %w", operationError)
	}
	return nil
}

func (hubInstance *Hub) replyAgent(value map[string]any) {
	if operationError := hubInstance.sendAgentJSON(value); operationError != nil {
		hubInstance.logger.Warn("reverse RPC reply failed", "error", operationError)
	}
}

func (hubInstance *Hub) removeClient(client *clientConnection) {
	client.close()
	hubInstance.stateMutex.Lock()
	delete(hubInstance.clients, client)
	for identifier, pending := range hubInstance.pending {
		if pending.client == client {
			pending.client = nil
			pending.detached = true
			hubInstance.pending[identifier] = pending
		}
	}
	count := len(hubInstance.clients)
	hubInstance.stateMutex.Unlock()
	hubInstance.logger.Info("remote client disconnected", "clients", count)
}

func (hubInstance *Hub) agentDisconnected(connection *websocket.Conn, cause error) {
	hubInstance.agentMutex.Lock()
	if hubInstance.agent != connection {
		hubInstance.agentMutex.Unlock()
		return
	}
	hubInstance.agent = nil
	if cause != nil {
		hubInstance.lastError = redactAgentURL(cause.Error())
	}
	hubInstance.agentMutex.Unlock()
	_ = connection.Close()

	hubInstance.stateMutex.Lock()
	pending := hubInstance.pending
	internal := hubInstance.internal
	hubInstance.pending = make(map[int64]pendingRequest)
	hubInstance.internal = make(map[int64]chan map[string]any)
	hubInstance.initCached = false
	hubInstance.initResult = nil
	hubInstance.stateMutex.Unlock()
	for _, request := range pending {
		if request.client != nil {
			hubInstance.sendRPCError(request.client, request.original, "agent disconnected", -32001)
		}
	}
	for _, response := range internal {
		response <- map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": -32001, "message": "agent disconnected"}}
	}
	hubInstance.broadcastHubState(false)
	hubInstance.logger.Warn("upstream agent disconnected", "error", cause)
}

func (hubInstance *Hub) disconnectAgent(cause error) {
	hubInstance.agentMutex.RLock()
	connection := hubInstance.agent
	hubInstance.agentMutex.RUnlock()
	if connection != nil {
		_ = connection.Close()
		hubInstance.agentDisconnected(connection, cause)
	}
}

func (hubInstance *Hub) broadcast(raw []byte) {
	hubInstance.stateMutex.Lock()
	clients := make([]*clientConnection, 0, len(hubInstance.clients))
	for client := range hubInstance.clients {
		clients = append(clients, client)
	}
	hubInstance.stateMutex.Unlock()
	for _, client := range clients {
		if operationError := client.send(raw); operationError != nil {
			hubInstance.removeClient(client)
		}
	}
}

func (hubInstance *Hub) broadcastJSON(value any) { hubInstance.broadcast(mustJSON(value)) }

func (hubInstance *Hub) broadcastHubState(connected bool) {
	hubInstance.broadcastJSON(map[string]any{"jsonrpc": "2.0", "method": "_x.ai/remote/hub", "params": map[string]any{"up": connected}})
}

func (hubInstance *Hub) broadcastPong(client *clientConnection, timestamp any) {
	_ = client.send(mustJSON(map[string]any{
		"jsonrpc": "2.0", "method": "_x.ai/remote/pong",
		"params": map[string]any{"t": timestamp, "s": float64(time.Now().UnixMilli()) / 1000, "clients": hubInstance.ClientCount(), "hub_up": hubInstance.AgentConnected()},
	}))
}

func (hubInstance *Hub) sendRPCError(client *clientConnection, identifier any, message string, code int) {
	if identifier == nil {
		_ = client.send(mustJSON(map[string]any{"jsonrpc": "2.0", "method": "error", "params": map[string]any{"message": message}}))
		return
	}
	_ = client.send(mustJSON(map[string]any{"jsonrpc": "2.0", "id": identifier, "error": map[string]any{"code": code, "message": message}}))
}

func (hubInstance *Hub) watch(operationContext context.Context) {
	ensureTicker := time.NewTicker(5 * time.Second)
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer ensureTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-operationContext.Done():
			return
		case <-ensureTicker.C:
			if !hubInstance.AgentConnected() {
				attempt, cancel := context.WithTimeout(operationContext, 12*time.Second)
				_ = hubInstance.Ensure(attempt)
				cancel()
			}
		case <-heartbeatTicker.C:
			if hubInstance.ClientCount() > 0 {
				hubInstance.broadcastHubState(hubInstance.AgentConnected())
			}
		}
	}
}

func mustJSON(value any) []byte {
	data, operationError := json.Marshal(value)
	if operationError != nil {
		return []byte(`{"jsonrpc":"2.0","method":"error","params":{"message":"JSON encoding failed"}}`)
	}
	return data
}

func numericID(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		converted := int64(typed)
		return converted, float64(converted) == typed
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case json.Number:
		parsed, operationError := typed.Int64()
		return parsed, operationError == nil
	default:
		return 0, false
	}
}

func idKey(value any) string {
	data, operationError := json.Marshal(value)
	if operationError != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func redactAgentURL(value string) string {
	if index := strings.Index(value, "server-key="); index >= 0 {
		end := strings.IndexAny(value[index:], "&' \t\r\n")
		if end < 0 {
			return value[:index] + "server-key=***"
		}
		return value[:index] + "server-key=***" + value[index+end:]
	}
	return value
}
