package hub

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type EnsureAgentFunc func(context.Context) error

type Hub struct {
	agentURL    string
	ensureAgent EnsureAgentFunc
	catalog     providerapi.Provider
	protocol    providerapi.ProtocolAdapter
	logger      *slog.Logger

	upgrader websocket.Upgrader

	connectionMutex sync.Mutex
	agentMutex      sync.RWMutex
	agent           *websocket.Conn
	agentGeneration uint64
	agentCancel     context.CancelFunc
	agentWriteMutex sync.Mutex
	lastError       string

	stateMutex      sync.Mutex
	clients         map[*clientConnection]struct{}
	pending         map[int64]pendingRequest
	internal        map[int64]internalRequest
	reverseRequests map[string]reverseRequestRoute
	sessionClients  map[string]map[*clientConnection]struct{}
	sessions        map[string]sessionWorkspaceBinding
	nextID          int64
	initResult      any
	initCached      bool
	watchCancel     context.CancelFunc
	terminals       *terminalManager
	closed          atomic.Bool

	lifetimeContext    context.Context
	lifetimeCancel     context.CancelFunc
	reverseWait        sync.WaitGroup
	reverseActive      atomic.Int64
	pendingWait        sync.WaitGroup
	closeComplete      chan struct{}
	heartbeatInterval  time.Duration
	clientReadTimeout  time.Duration
	pendingLimit       int
	pendingClientLimit int
	pendingTimeout     time.Duration
}

func New(agentURL string, catalog providerapi.Provider, protocol providerapi.ProtocolAdapter, ensureAgent EnsureAgentFunc, logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	lifetimeContext, lifetimeCancel := context.WithCancel(context.Background())
	return &Hub{
		agentURL:    agentURL,
		ensureAgent: ensureAgent,
		catalog:     catalog,
		protocol:    protocol,
		logger:      logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  64 * 1024,
			WriteBufferSize: 64 * 1024,
		},
		clients:            make(map[*clientConnection]struct{}),
		pending:            make(map[int64]pendingRequest),
		internal:           make(map[int64]internalRequest),
		reverseRequests:    make(map[string]reverseRequestRoute),
		sessionClients:     make(map[string]map[*clientConnection]struct{}),
		sessions:           make(map[string]sessionWorkspaceBinding),
		terminals:          newTerminalManager(),
		lifetimeContext:    lifetimeContext,
		lifetimeCancel:     lifetimeCancel,
		closeComplete:      make(chan struct{}),
		heartbeatInterval:  20 * time.Second,
		clientReadTimeout:  60 * time.Second,
		pendingLimit:       defaultPendingRequestLimit,
		pendingClientLimit: defaultPendingClientRequestLimit,
		pendingTimeout:     defaultPendingRequestTimeout,
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
	for identifier := range hubInstance.pending {
		_, _ = hubInstance.removePendingLocked(identifier)
	}
	hubInstance.clients = make(map[*clientConnection]struct{})
	hubInstance.sessionClients = make(map[string]map[*clientConnection]struct{})
	hubInstance.reverseRequests = make(map[string]reverseRequestRoute)
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
	hubInstance.pendingWait.Wait()
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
