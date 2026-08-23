// Client WebSocket transport: upgrade, per-client write serialization,
// heartbeats, removal, and broadcast.

package hub

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type clientConnection struct {
	connection          *websocket.Conn
	write               sync.Mutex
	closed              atomic.Bool
	closedSignal        chan struct{}
	writeTimeout        time.Duration
	controlWriteTimeout time.Duration
}

func (clientConnectionInstance *clientConnection) send(data []byte) error {
	if clientConnectionInstance == nil || clientConnectionInstance.closed.Load() {
		return errors.New("client closed")
	}
	clientConnectionInstance.write.Lock()
	defer clientConnectionInstance.write.Unlock()
	_ = clientConnectionInstance.connection.SetWriteDeadline(time.Now().Add(clientConnectionInstance.writeTimeout))
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
	return clientConnectionInstance.connection.WriteControl(messageType, payload, time.Now().Add(clientConnectionInstance.controlWriteTimeout))
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
	connection.SetReadLimit(hubInstance.policy.MaxMessageBytes)
	client := &clientConnection{connection: connection, closedSignal: make(chan struct{}), writeTimeout: hubInstance.policy.WriteTimeout, controlWriteTimeout: hubInstance.policy.ControlWriteTimeout}
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

	operationContext, cancel := context.WithTimeout(request.Context(), hubInstance.policy.ClientConnectEnsure)
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

func (hubInstance *Hub) removeClient(client *clientConnection) {
	client.close()
	hubInstance.stateMutex.Lock()
	delete(hubInstance.clients, client)
	for cacheKey, subscribedClients := range hubInstance.sessionClients {
		delete(subscribedClients, client)
		if len(subscribedClients) == 0 {
			delete(hubInstance.sessionClients, cacheKey)
		}
	}
	for identifier, pending := range hubInstance.pending {
		if pending.client == client {
			pending.client = nil
			pending.detached = true
			hubInstance.pending[identifier] = pending
		}
	}
	abandonedReverseRequests := make([]reverseRequestRoute, 0)
	for routeKey, route := range hubInstance.reverseRequests {
		if _, subscribed := route.clients[client]; !subscribed {
			continue
		}
		delete(route.clients, client)
		if len(route.clients) == 0 {
			delete(hubInstance.reverseRequests, routeKey)
			abandonedReverseRequests = append(abandonedReverseRequests, route)
		} else {
			hubInstance.reverseRequests[routeKey] = route
		}
	}
	count := len(hubInstance.clients)
	hubInstance.stateMutex.Unlock()
	for _, route := range abandonedReverseRequests {
		hubInstance.replyReverseUnavailable(route.identifier, route.permission, "remote client disconnected", route.agentGeneration)
	}
	hubInstance.logger.Info("remote client disconnected", "clients", count)
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
	hubInstance.broadcastNotification(providerapi.HubStateNotification, map[string]any{"up": connected})
}
