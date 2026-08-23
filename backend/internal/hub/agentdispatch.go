// Agent-to-daemon dispatch. Notifications and responses arriving from the
// provider agent are checked against the connection generation that produced
// them before any state is touched.

package hub

import (
	"context"
	"encoding/json"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

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
