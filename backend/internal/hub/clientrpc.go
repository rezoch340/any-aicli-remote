// Client-to-agent RPC. Public client requests are classified by the provider
// adapter and fail closed before anything is forwarded upstream.

package hub

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

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
		timeout := hubInstance.policy.NormalEnsure
		if prepared.Patient {
			timeout = hubInstance.policy.PatientEnsure
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
		if route.interactionKind != "" {
			hubInstance.relayInteractionAnswer(object, route, client, identifier)
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

func (hubInstance *Hub) rejectReverseClientRequest(method string, params map[string]any) error {
	if hubInstance.protocol == nil {
		return nil
	}
	if _, known := hubInstance.protocol.ClassifyReverseRequest(method, params); known {
		return errors.New("reverse provider method is not accepted from client")
	}
	return nil
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

// relayInteractionAnswer denormalizes a client's neutral interaction answer into
// the provider result and relays it to the agent. A malformed answer fails
// closed: the agent receives an error rather than an unparseable payload, and
// the client is told its answer was rejected.
func (hubInstance *Hub) relayInteractionAnswer(object map[string]any, route reverseRequestRoute, client *clientConnection, clientIdentifier any) {
	if hubInstance.protocol == nil {
		return
	}
	if errorPayload, present := object["error"]; present {
		hubInstance.sendAgentJSONForGeneration(route.agentGeneration, map[string]any{
			"jsonrpc": "2.0", "id": route.identifier, "error": errorPayload,
		})
		return
	}
	rawResult, _ := object["result"].(map[string]any)
	var response providerapi.InteractionResponse
	if decodeError := decodeInteractionResponse(rawResult, &response); decodeError != nil {
		hubInstance.replyInteractionUnavailable(route.identifier, "invalid interaction answer", route.agentGeneration)
		hubInstance.sendRPCError(client, clientIdentifier, decodeError.Error(), methodNotCallableErrorCode)
		return
	}
	providerResult, denormalizeError := hubInstance.protocol.DenormalizeInteractionResponse(route.interactionKind, response)
	if denormalizeError != nil {
		hubInstance.replyInteractionUnavailable(route.identifier, "invalid interaction answer", route.agentGeneration)
		hubInstance.sendRPCError(client, clientIdentifier, denormalizeError.Error(), methodNotCallableErrorCode)
		return
	}
	if operationError := hubInstance.sendAgentJSONForGeneration(route.agentGeneration, map[string]any{
		"jsonrpc": "2.0", "id": route.identifier, "result": providerResult,
	}); operationError != nil {
		hubInstance.logger.Warn("interaction answer dropped", "error", operationError)
	}
}

func decodeInteractionResponse(rawResult map[string]any, response *providerapi.InteractionResponse) error {
	if rawResult == nil {
		return errors.New("interaction answer missing result")
	}
	encoded, marshalError := json.Marshal(rawResult)
	if marshalError != nil {
		return marshalError
	}
	return json.Unmarshal(encoded, response)
}
