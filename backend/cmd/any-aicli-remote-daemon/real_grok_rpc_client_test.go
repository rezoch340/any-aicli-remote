package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
)

const legacyRequestPermissionMethod = "session/requestPermission"
const realGrokNotificationBufferSize = 1024

var realGrokRPCClientClosedError = errors.New("ACP WebSocket client closed")

type realGrokRPCEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type realGrokRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type realGrokRPCClient struct {
	connection *websocket.Conn
	sequence   atomic.Uint64

	writeMutex sync.Mutex
	stateMutex sync.Mutex
	closeOnce  sync.Once
	pending    map[string]chan realGrokRPCEnvelope
	closeError error

	done               chan struct{}
	notifications      chan realGrokRPCEnvelope
	permissionObserved atomic.Bool
}

func newRealGrokRPCClient(connection *websocket.Conn) *realGrokRPCClient {
	client := &realGrokRPCClient{
		connection:    connection,
		done:          make(chan struct{}),
		pending:       make(map[string]chan realGrokRPCEnvelope),
		notifications: make(chan realGrokRPCEnvelope, realGrokNotificationBufferSize),
	}
	go client.readPump()
	return client
}

func (client *realGrokRPCClient) readPump() {
	for {
		_, payload, readError := client.connection.ReadMessage()
		if readError != nil {
			client.closeWithError(fmt.Errorf("ACP WebSocket read failed: %w", readError))
			return
		}
		var envelope realGrokRPCEnvelope
		if unmarshalError := json.Unmarshal(payload, &envelope); unmarshalError != nil {
			client.closeWithError(fmt.Errorf("ACP JSON-RPC envelope decode failed: %w", unmarshalError))
			return
		}
		client.routeEnvelope(envelope)
	}
}

func (client *realGrokRPCClient) routeEnvelope(envelope realGrokRPCEnvelope) {
	if envelope.Method != "" && !realGrokRPCIDEmpty(envelope.ID) {
		client.handleReverseRequest(envelope)
		return
	}
	if envelope.Method != "" {
		select {
		case client.notifications <- envelope:
		case <-client.done:
		}
		return
	}
	if realGrokRPCIDEmpty(envelope.ID) {
		return
	}
	requestKey := string(envelope.ID)
	client.stateMutex.Lock()
	responseChannel := client.pending[requestKey]
	client.stateMutex.Unlock()
	if responseChannel == nil {
		return
	}
	select {
	case responseChannel <- envelope:
	default:
	}
}

func (client *realGrokRPCClient) handleReverseRequest(envelope realGrokRPCEnvelope) {
	if envelope.Method != acp.ClientMethodSessionRequestPermission && envelope.Method != legacyRequestPermissionMethod {
		_ = client.writeEnvelope(realGrokRPCEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   realGrokRPCErrorPayload(-32601, "Method not found"),
		})
		return
	}

	var request acp.RequestPermissionRequest
	if unmarshalError := json.Unmarshal(envelope.Params, &request); unmarshalError != nil {
		_ = client.writeEnvelope(realGrokRPCEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   realGrokRPCErrorPayload(-32602, "Invalid params"),
		})
		return
	}
	response := acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}}}
	if selectedOption, foundOption := realGrokAllowOnceOption(request.Options); foundOption {
		response = acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: selectedOption.OptionId}}}
	}
	encodedResponse, marshalError := json.Marshal(response)
	if marshalError != nil {
		_ = client.writeEnvelope(realGrokRPCEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   realGrokRPCErrorPayload(-32603, "Internal error"),
		})
		return
	}
	client.permissionObserved.Store(true)
	_ = client.writeEnvelope(realGrokRPCEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: encodedResponse})
}

func realGrokAllowOnceOption(options []acp.PermissionOption) (acp.PermissionOption, bool) {
	for _, option := range options {
		if option.Kind == acp.PermissionOptionKindAllowOnce {
			return option, true
		}
	}
	if len(options) != 0 {
		return options[0], true
	}
	return acp.PermissionOption{}, false
}

func realGrokRPCErrorPayload(code int, message string) json.RawMessage {
	payload, _ := json.Marshal(realGrokRPCError{Code: code, Message: message})
	return payload
}

func (client *realGrokRPCClient) call(requestContext context.Context, method string, parameters any) (realGrokRPCEnvelope, error) {
	if requestContext == nil {
		requestContext = context.Background()
	}
	requestID := client.sequence.Add(1)
	encodedID, marshalError := json.Marshal(requestID)
	if marshalError != nil {
		return realGrokRPCEnvelope{}, fmt.Errorf("ACP request ID encode failed: %w", marshalError)
	}
	encodedParameters, marshalError := json.Marshal(parameters)
	if marshalError != nil {
		return realGrokRPCEnvelope{}, fmt.Errorf("ACP %s params encode failed: %w", method, marshalError)
	}
	responseChannel := make(chan realGrokRPCEnvelope, 1)
	requestKey := string(encodedID)
	client.stateMutex.Lock()
	if client.closeError != nil {
		closeError := client.closeError
		client.stateMutex.Unlock()
		return realGrokRPCEnvelope{}, closeError
	}
	client.pending[requestKey] = responseChannel
	client.stateMutex.Unlock()
	defer client.removePending(requestKey)

	writeError := client.writeEnvelope(realGrokRPCEnvelope{JSONRPC: "2.0", ID: encodedID, Method: method, Params: encodedParameters})
	if writeError != nil {
		return realGrokRPCEnvelope{}, writeError
	}
	select {
	case response, channelOpen := <-responseChannel:
		if !channelOpen {
			return realGrokRPCEnvelope{}, client.terminalError()
		}
		if rpcError := realGrokRPCResponseError(method, response.Error); rpcError != nil {
			return realGrokRPCEnvelope{}, rpcError
		}
		return response, nil
	case <-requestContext.Done():
		return realGrokRPCEnvelope{}, requestContext.Err()
	case <-client.done:
		return realGrokRPCEnvelope{}, client.terminalError()
	}
}

func realGrokRPCResponseError(method string, rawError json.RawMessage) error {
	if len(rawError) == 0 || string(rawError) == "null" {
		return nil
	}
	var rpcError realGrokRPCError
	if unmarshalError := json.Unmarshal(rawError, &rpcError); unmarshalError != nil {
		return fmt.Errorf("ACP %s failed with an invalid JSON-RPC error", method)
	}
	return fmt.Errorf("ACP %s failed: code=%d message=%q", method, rpcError.Code, rpcError.Message)
}

func decodeRealGrokRPCResult(envelope realGrokRPCEnvelope, resultValue any) error {
	if rpcError := realGrokRPCResponseError("response", envelope.Error); rpcError != nil {
		return rpcError
	}
	if unmarshalError := json.Unmarshal(envelope.Result, resultValue); unmarshalError != nil {
		return fmt.Errorf("ACP result decode failed: %w", unmarshalError)
	}
	return nil
}

func (client *realGrokRPCClient) notify(method string, parameters any) error {
	encodedParameters, marshalError := json.Marshal(parameters)
	if marshalError != nil {
		return fmt.Errorf("ACP %s params encode failed: %w", method, marshalError)
	}
	return client.writeEnvelope(realGrokRPCEnvelope{JSONRPC: "2.0", Method: method, Params: encodedParameters})
}

func (client *realGrokRPCClient) writeEnvelope(envelope realGrokRPCEnvelope) error {
	payload, marshalError := json.Marshal(envelope)
	if marshalError != nil {
		return fmt.Errorf("ACP JSON-RPC envelope encode failed: %w", marshalError)
	}
	client.writeMutex.Lock()
	defer client.writeMutex.Unlock()
	select {
	case <-client.done:
		return client.terminalError()
	default:
	}
	if writeError := client.connection.WriteMessage(websocket.TextMessage, payload); writeError != nil {
		client.closeWithError(fmt.Errorf("ACP WebSocket write failed: %w", writeError))
		return client.terminalError()
	}
	return nil
}

func (client *realGrokRPCClient) nextNotification(notificationContext context.Context) (realGrokRPCEnvelope, error) {
	if notificationContext == nil {
		notificationContext = context.Background()
	}
	select {
	case notification := <-client.notifications:
		return notification, nil
	case <-notificationContext.Done():
		return realGrokRPCEnvelope{}, notificationContext.Err()
	case <-client.done:
		return realGrokRPCEnvelope{}, client.terminalError()
	}
}

func (client *realGrokRPCClient) permissionWasObserved() bool {
	return client.permissionObserved.Load()
}

func (client *realGrokRPCClient) removePending(requestKey string) {
	client.stateMutex.Lock()
	delete(client.pending, requestKey)
	client.stateMutex.Unlock()
}

func (client *realGrokRPCClient) terminalError() error {
	client.stateMutex.Lock()
	defer client.stateMutex.Unlock()
	if client.closeError != nil {
		return client.closeError
	}
	return realGrokRPCClientClosedError
}

func (client *realGrokRPCClient) close() {
	client.closeWithError(realGrokRPCClientClosedError)
}

func (client *realGrokRPCClient) closeWithError(closeError error) {
	client.closeOnce.Do(func() {
		client.stateMutex.Lock()
		client.closeError = closeError
		for requestKey := range client.pending {
			delete(client.pending, requestKey)
		}
		client.stateMutex.Unlock()
		close(client.done)
		_ = client.connection.Close()
	})
}

func realGrokRPCIDEmpty(identifier json.RawMessage) bool {
	return len(identifier) == 0 || string(identifier) == "null"
}

func dialRealGrokWebSocket(address string, key string) (*realGrokRPCClient, *http.Response, error) {
	endpointURL, parseError := url.Parse(address)
	if parseError != nil {
		return nil, nil, fmt.Errorf("ACP WebSocket URL parse failed: %w", parseError)
	}
	queryValues := endpointURL.Query()
	queryValues.Set("key", key)
	endpointURL.RawQuery = queryValues.Encode()
	connection, response, dialError := websocket.DefaultDialer.Dial(endpointURL.String(), nil)
	if dialError != nil {
		return nil, response, fmt.Errorf("ACP WebSocket dial failed for %s://%s%s", endpointURL.Scheme, endpointURL.Host, endpointURL.EscapedPath())
	}
	return newRealGrokRPCClient(connection), response, nil
}
