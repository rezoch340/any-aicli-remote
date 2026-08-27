// Agent WebSocket transport. The connection generation published here is the
// one canonical ownership check: a write or dispatch for a superseded
// generation is dropped rather than applied to the current connection.

package hub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

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
	for attempt := 0; attempt < hubInstance.policy.DialAttempts; attempt++ {
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
		dialer := websocket.Dialer{HandshakeTimeout: hubInstance.policy.DialHandshake, EnableCompression: true}
		connection, _, operationError := dialer.DialContext(operationContext, hubInstance.agentURL, nil)
		if operationError == nil {
			connection.SetReadLimit(hubInstance.policy.AgentMaxMessageBytes)
			agentGeneration, agentContext, published := hubInstance.publishAgent(connection)
			if !published {
				_ = connection.Close()
				return errors.New("hub closed")
			}
			hubInstance.logger.Info("upstream agent connected")
			go hubInstance.readAgent(connection, agentGeneration, agentContext)
			hubInstance.broadcastHubState(true)
			return nil
		}
		last = operationError
		hubInstance.agentMutex.Lock()
		hubInstance.lastError = redactConnectionURL(operationError.Error())
		hubInstance.agentMutex.Unlock()
		select {
		case <-operationContext.Done():
			return operationContext.Err()
		case <-time.After(time.Duration(attempt+1) * hubInstance.policy.RetryDelay):
		}
	}
	if last == nil {
		last = errors.New("agent unavailable")
	}
	return last
}

func (hubInstance *Hub) readAgent(connection *websocket.Conn, agentGeneration uint64, agentContext context.Context) {
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
		hubInstance.handleAgentMessage(raw, agentGeneration, agentContext)
	}
}

func (hubInstance *Hub) sendAgentJSON(value any) error { return hubInstance.sendAgent(mustJSON(value)) }

func (hubInstance *Hub) publishAgent(connection *websocket.Conn) (uint64, context.Context, bool) {
	hubInstance.agentMutex.Lock()
	defer hubInstance.agentMutex.Unlock()
	if hubInstance.closed.Load() {
		return 0, nil, false
	}
	if hubInstance.agentCancel != nil {
		hubInstance.agentCancel()
	}
	hubInstance.agentGeneration++
	agentContext, agentCancel := context.WithCancel(hubInstance.lifetimeContext)
	hubInstance.agent = connection
	hubInstance.agentCancel = agentCancel
	hubInstance.lastError = ""
	return hubInstance.agentGeneration, agentContext, true
}

func (hubInstance *Hub) agentGenerationIsCurrent(agentGeneration uint64) bool {
	hubInstance.agentMutex.RLock()
	current := hubInstance.agent != nil && hubInstance.agentGeneration == agentGeneration
	hubInstance.agentMutex.RUnlock()
	return current
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
	_ = connection.SetWriteDeadline(time.Now().Add(hubInstance.policy.WriteTimeout))
	if operationError := connection.WriteMessage(websocket.TextMessage, raw); operationError != nil {
		return fmt.Errorf("send agent: %w", operationError)
	}
	return nil
}

func (hubInstance *Hub) sendAgentForGeneration(agentGeneration uint64, raw []byte) error {
	hubInstance.agentMutex.RLock()
	connection := hubInstance.agent
	currentGeneration := hubInstance.agentGeneration
	hubInstance.agentMutex.RUnlock()
	if connection == nil || currentGeneration != agentGeneration {
		return errors.New("agent generation changed")
	}
	hubInstance.agentWriteMutex.Lock()
	defer hubInstance.agentWriteMutex.Unlock()
	_ = connection.SetWriteDeadline(time.Now().Add(hubInstance.policy.WriteTimeout))
	if operationError := connection.WriteMessage(websocket.TextMessage, raw); operationError != nil {
		return fmt.Errorf("send agent generation %d: %w", agentGeneration, operationError)
	}
	return nil
}

func (hubInstance *Hub) sendAgentJSONForGeneration(agentGeneration uint64, value any) error {
	return hubInstance.sendAgentForGeneration(agentGeneration, mustJSON(value))
}

func (hubInstance *Hub) replyAgentForGeneration(agentGeneration uint64, value map[string]any) {
	if operationError := hubInstance.sendAgentJSONForGeneration(agentGeneration, value); operationError != nil {
		hubInstance.logger.Warn("reverse RPC reply failed", "error", operationError)
	}
}

func (hubInstance *Hub) agentDisconnected(connection *websocket.Conn, cause error) {
	hubInstance.agentMutex.Lock()
	if hubInstance.agent != connection {
		hubInstance.agentMutex.Unlock()
		return
	}
	hubInstance.agent = nil
	agentCancel := hubInstance.agentCancel
	hubInstance.agentCancel = nil
	if cause != nil {
		hubInstance.lastError = redactConnectionURL(cause.Error())
	}
	hubInstance.agentMutex.Unlock()
	if agentCancel != nil {
		agentCancel()
	}
	_ = connection.Close()

	hubInstance.stateMutex.Lock()
	pending := hubInstance.pending
	internal := hubInstance.internal
	for _, request := range pending {
		if request.timeoutCancel != nil {
			request.timeoutCancel()
		}
	}
	hubInstance.pending = make(map[int64]pendingRequest)
	hubInstance.internal = make(map[int64]internalRequest)
	hubInstance.reverseRequests = make(map[string]reverseRequestRoute)
	hubInstance.initCached = false
	hubInstance.initResult = nil
	hubInstance.stateMutex.Unlock()
	for _, request := range pending {
		if request.client != nil {
			hubInstance.sendRPCError(request.client, request.original, "agent disconnected", agentUnavailableErrorCode)
		}
	}
	for _, request := range internal {
		request.response <- map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": agentUnavailableErrorCode, "message": "agent disconnected"}}
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

func redactConnectionURL(value string) string {
	queryIndex := strings.Index(value, "?")
	if queryIndex < 0 {
		return value
	}
	remainder := value[queryIndex:]
	queryEnd := strings.IndexAny(remainder, "' \t\r\n")
	if queryEnd < 0 {
		return value[:queryIndex] + "?***"
	}
	return value[:queryIndex] + "?***" + remainder[queryEnd:]
}
