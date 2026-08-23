package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const (
	defaultPendingRequestLimit        = 256
	defaultPendingClientRequestLimit  = 32
	defaultPendingRequestTimeout      = 30 * time.Minute
	agentUnavailableErrorCode         = -32001
	requestLimitErrorCode             = -32002
	providerReverseOperationErrorCode = -32000
	methodNotCallableErrorCode        = -32601
	invalidParamsErrorCode            = -32602
)

type pendingRequest struct {
	client        *clientConnection
	original      any
	prepared      providerapi.PreparedRequest
	detached      bool
	timeoutCancel context.CancelFunc
}

type internalRequest struct {
	response chan map[string]any
	prepared providerapi.PreparedRequest
}

func (hubInstance *Hub) CallRPC(operationContext context.Context, method string, params map[string]any) (map[string]any, error) {
	if hubInstance.protocol == nil {
		return nil, errors.New("provider protocol unavailable")
	}
	if params == nil {
		params = map[string]any{}
	}
	if operationError := hubInstance.rejectReverseClientRequest(method, params); operationError != nil {
		return nil, operationError
	}
	prepared, operationError := hubInstance.prepareClientRequest(operationContext, method, params)
	if operationError != nil {
		return nil, operationError
	}
	payload := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if operationError = hubInstance.Ensure(operationContext); operationError != nil {
		return nil, fmt.Errorf("agent offline: %w", operationError)
	}
	identifier := atomic.AddInt64(&hubInstance.nextID, 1)
	response := make(chan map[string]any, 1)
	hubInstance.stateMutex.Lock()
	hubInstance.internal[identifier] = internalRequest{response: response, prepared: prepared}
	hubInstance.stateMutex.Unlock()
	payload["id"] = identifier
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

func (hubInstance *Hub) handleAgentMessage(raw []byte, agentGeneration uint64, agentContext context.Context) {
	if !hubInstance.agentGenerationIsCurrent(agentGeneration) {
		return
	}
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
		if internal, present := hubInstance.internal[numericID]; present {
			delete(hubInstance.internal, numericID)
			_, _ = hubInstance.captureSessionBindingLocked(internal.prepared, object)
			hubInstance.stateMutex.Unlock()
			internal.response <- object
			return
		}
		pending, found := hubInstance.removePendingLocked(numericID)
		if found {
			if pending.prepared.Kind == providerapi.InitializationRequest {
				if result, present := object["result"]; present {
					hubInstance.initResult = result
					hubInstance.initCached = true
				}
			}
			binding, captured := hubInstance.captureSessionBindingLocked(pending.prepared, object)
			if captured && pending.client != nil {
				hubInstance.subscribeSessionClientLocked(pending.client, binding.ProviderID, binding.SessionID)
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
			hubInstance.broadcastNotification(providerapi.DetachedRequestNotification, map[string]any{"id": pending.original, "ok": object["error"] == nil, "detached": true})
		}
		return
	}
	if hasID && method != "" {
		if hubInstance.handleReverseAsync(object, agentGeneration, agentContext) {
			return
		}
		reverseParams, _ := object["params"].(map[string]any)
		hubInstance.forwardReverseRequest(object, stringValue(reverseParams["sessionId"]), false, agentGeneration)
		return
	}
	if !hasID && method != "" && hubInstance.protocol != nil {
		params, _ := object["params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		normalizedMethod, normalizedParams := hubInstance.protocol.NormalizeAgentNotification(method, params)
		if normalizedMethod == "" {
			return
		}
		normalizedParams = hubInstance.scopeSessionParams(stringValue(normalizedParams["sessionId"]), normalizedParams)
		object["method"] = normalizedMethod
		object["params"] = normalizedParams
		raw = mustJSON(object)
	}
	hubInstance.broadcast(raw)
}

func (hubInstance *Hub) handleClientMessage(client *clientConnection, raw []byte) {
	var object map[string]any
	if operationError := json.Unmarshal(raw, &object); operationError != nil {
		if operationError := hubInstance.ensureAndSend(raw, false); operationError != nil {
			hubInstance.sendRPCError(client, nil, operationError.Error(), agentUnavailableErrorCode)
		}
		return
	}
	method, _ := object["method"].(string)
	identifier, hasID := object["id"]
	var prepared providerapi.PreparedRequest
	if method != "" {
		if hubInstance.protocol == nil {
			hubInstance.sendRPCError(client, identifier, "provider protocol unavailable", agentUnavailableErrorCode)
			return
		}
		params, _ := object["params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
			object["params"] = params
		}
		if operationError := hubInstance.rejectReverseClientRequest(method, params); operationError != nil {
			if hasID {
				hubInstance.sendRPCError(client, identifier, operationError.Error(), methodNotCallableErrorCode)
			}
			return
		}
		var operationError error
		prepared, operationError = hubInstance.prepareClientRequest(hubInstance.lifetimeContext, method, params)
		if operationError != nil {
			hubInstance.sendRPCError(client, identifier, operationError.Error(), invalidParamsErrorCode)
			return
		}
		if prepared.Kind == providerapi.InitializationRequest {
			hubInstance.protocol.ConfigureInitialization(params)
		}
		if prepared.Kind == providerapi.PingRequest {
			hubInstance.sendPingResponse(client, prepared)
			return
		}
		if prepared.RequiresSession && prepared.SessionID != "" {
			hubInstance.stateMutex.Lock()
			hubInstance.subscribeSessionClientLocked(client, hubInstance.protocol.ID(), prepared.SessionID)
			hubInstance.stateMutex.Unlock()
		}
	}
	if prepared.Kind == providerapi.InitializationRequest && hasID {
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
		if prepared.Patient {
			timeout = 18 * time.Second
		}
		operationContext, cancel := context.WithTimeout(context.Background(), timeout)
		operationError := hubInstance.Ensure(operationContext)
		cancel()
		if operationError != nil {
			hubInstance.sendRPCError(client, identifier, "agent offline: "+redactConnectionURL(operationError.Error()), agentUnavailableErrorCode)
			return
		}
		hubID := atomic.AddInt64(&hubInstance.nextID, 1)
		hubInstance.stateMutex.Lock()
		if hubInstance.closed.Load() {
			hubInstance.stateMutex.Unlock()
			hubInstance.sendRPCError(client, identifier, "hub closed", agentUnavailableErrorCode)
			return
		}
		pendingCount := len(hubInstance.pending)
		clientPendingCount := 0
		for _, pending := range hubInstance.pending {
			if pending.client == client {
				clientPendingCount++
			}
		}
		if pendingCount >= hubInstance.pendingLimit {
			hubInstance.stateMutex.Unlock()
			hubInstance.sendRPCError(client, identifier, "too many in-flight provider requests", requestLimitErrorCode)
			return
		}
		if clientPendingCount >= hubInstance.pendingClientLimit {
			hubInstance.stateMutex.Unlock()
			hubInstance.sendRPCError(client, identifier, "client has too many in-flight provider requests", requestLimitErrorCode)
			return
		}
		hubInstance.pending[hubID] = pendingRequest{
			client: client, original: identifier, prepared: prepared,
		}
		hubInstance.stateMutex.Unlock()
		object["id"] = hubID
		if operationError := hubInstance.sendAgentJSON(object); operationError != nil {
			hubInstance.stateMutex.Lock()
			_, _ = hubInstance.removePendingLocked(hubID)
			hubInstance.stateMutex.Unlock()
			hubInstance.sendRPCError(client, identifier, operationError.Error(), agentUnavailableErrorCode)
			return
		}
		hubInstance.armPendingTimeout(hubID)
		return
	}
	if hasID && method == "" {
		key := idKey(identifier)
		hubInstance.stateMutex.Lock()
		route, known := hubInstance.reverseRequests[key]
		_, allowed := route.clients[client]
		if known && allowed {
			delete(hubInstance.reverseRequests, key)
		}
		hubInstance.stateMutex.Unlock()
		if !known || !allowed {
			return
		}
		object["id"] = route.identifier
		if operationError := hubInstance.sendAgentJSONForGeneration(route.agentGeneration, object); operationError != nil {
			hubInstance.logger.Warn("reverse RPC response dropped", "error", operationError)
		}
		return
	}
	if operationError := hubInstance.ensureAndSend(mustJSON(object), prepared.Patient); operationError != nil {
		hubInstance.sendRPCError(client, identifier, operationError.Error(), agentUnavailableErrorCode)
	}
}

// removePendingLocked removes one forwarded request and stops its expiry timer.
// The caller must hold stateMutex.
func (hubInstance *Hub) removePendingLocked(identifier int64) (pendingRequest, bool) {
	pending, present := hubInstance.pending[identifier]
	if !present {
		return pendingRequest{}, false
	}
	delete(hubInstance.pending, identifier)
	if pending.timeoutCancel != nil {
		pending.timeoutCancel()
	}
	return pending, true
}

func (hubInstance *Hub) armPendingTimeout(identifier int64) {
	timeout := hubInstance.pendingTimeout
	if timeout <= 0 {
		timeout = defaultPendingRequestTimeout
	}
	timeoutContext, timeoutCancel := context.WithCancel(hubInstance.lifetimeContext)
	hubInstance.stateMutex.Lock()
	pending, present := hubInstance.pending[identifier]
	if present {
		pending.timeoutCancel = timeoutCancel
		hubInstance.pending[identifier] = pending
		hubInstance.pendingWait.Add(1)
	}
	hubInstance.stateMutex.Unlock()
	if !present {
		timeoutCancel()
		return
	}
	go func() {
		defer hubInstance.pendingWait.Done()
		defer timeoutCancel()
		timeoutTimer := time.NewTimer(timeout)
		defer timeoutTimer.Stop()
		select {
		case <-timeoutContext.Done():
			return
		case <-timeoutTimer.C:
			hubInstance.expirePendingRequest(identifier)
		}
	}()
}

func (hubInstance *Hub) expirePendingRequest(identifier int64) {
	hubInstance.stateMutex.Lock()
	pending, present := hubInstance.removePendingLocked(identifier)
	hubInstance.stateMutex.Unlock()
	if !present {
		return
	}
	message := "provider request timed out"
	if pending.client != nil && !pending.detached && !pending.client.closed.Load() {
		hubInstance.sendRPCError(pending.client, pending.original, message, agentUnavailableErrorCode)
		return
	}
	hubInstance.broadcastNotification(providerapi.DetachedRequestNotification, map[string]any{
		"id": pending.original, "ok": false, "detached": true, "error": message,
	})
}

func (hubInstance *Hub) rejectReverseClientRequest(method string, params map[string]any) error {
	if hubInstance.protocol == nil {
		return nil
	}
	if _, known := hubInstance.protocol.ClassifyReverseRequest(method, params); known {
		return errors.New("reverse provider method is not accepted from client")
	}
	return nil
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

func (hubInstance *Hub) sendPingResponse(client *clientConnection, prepared providerapi.PreparedRequest) {
	params := make(map[string]any, len(prepared.PingResponseParams)+3)
	for key, value := range prepared.PingResponseParams {
		params[key] = value
	}
	params["s"] = float64(time.Now().UnixMilli()) / 1000
	params["clients"] = hubInstance.ClientCount()
	params["hub_up"] = hubInstance.AgentConnected()
	_ = client.send(mustJSON(map[string]any{"jsonrpc": "2.0", "method": prepared.PingResponseMethod, "params": params}))
}

func (hubInstance *Hub) sendRPCError(client *clientConnection, identifier any, message string, code int) {
	if identifier == nil {
		method, params := hubInstance.daemonNotification(providerapi.ProtocolErrorNotification, map[string]any{"message": message})
		if method != "" {
			_ = client.send(mustJSON(map[string]any{"jsonrpc": "2.0", "method": method, "params": params}))
		}
		return
	}
	_ = client.send(mustJSON(map[string]any{"jsonrpc": "2.0", "id": identifier, "error": map[string]any{"code": code, "message": message}}))
}

func (hubInstance *Hub) daemonNotification(kind providerapi.NotificationKind, params map[string]any) (string, map[string]any) {
	if hubInstance.protocol == nil {
		return "", nil
	}
	return hubInstance.protocol.DaemonNotification(kind, params)
}

func (hubInstance *Hub) broadcastNotification(kind providerapi.NotificationKind, params map[string]any) {
	method, providerParams := hubInstance.daemonNotification(kind, params)
	if method == "" {
		return
	}
	hubInstance.broadcastJSON(map[string]any{"jsonrpc": "2.0", "method": method, "params": providerParams})
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
