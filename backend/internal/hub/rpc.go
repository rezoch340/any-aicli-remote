// Daemon-originated RPC and the JSON-RPC value helpers shared by the hub.

package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const (
	agentUnavailableErrorCode         = -32001
	requestLimitErrorCode             = -32002
	providerReverseOperationErrorCode = -32000
	methodNotCallableErrorCode        = -32601
	invalidParamsErrorCode            = -32602
)

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

func (hubInstance *Hub) ensureAndSend(raw []byte, patient bool) error {
	timeout := hubInstance.policy.NotificationEnsure
	if patient {
		timeout = hubInstance.policy.PatientEnsure
	}
	operationContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if operationError := hubInstance.Ensure(operationContext); operationError != nil {
		return operationError
	}
	return hubInstance.sendAgent(raw)
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
