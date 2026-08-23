package hub

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const reverseFileReaderBufferBytes = 64 * 1024

type reverseRequestRoute struct {
	identifier      any
	agentGeneration uint64
	clients         map[*clientConnection]struct{}
	permission      bool
}

func (hubInstance *Hub) handleReverseAsync(
	object map[string]any,
	agentGeneration uint64,
	agentContext context.Context,
) bool {
	method, _ := object["method"].(string)
	params, _ := object["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	if hubInstance.protocol == nil {
		return false
	}
	reverseRequest, known := hubInstance.protocol.ClassifyReverseRequest(method, params)
	if !known {
		return false
	}
	if reverseRequest.Operation == providerapi.PermissionOperation {
		hubInstance.forwardReverseRequest(object, reverseRequest.SessionID, true, agentGeneration)
		return true
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
		hubInstance.handleReverse(agentContext, object, reverseRequest, agentGeneration)
	}()
	return true
}

func (hubInstance *Hub) handleReverse(
	operationContext context.Context,
	object map[string]any,
	reverseRequest providerapi.ReverseRequest,
	agentGeneration uint64,
) {
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
	var operationError error

	switch reverseRequest.Operation {
	case providerapi.ReadFileOperation:
		rootIdentity, workspaceError := hubInstance.resolveToolWorkspace(reverseRequest.SessionID)
		if workspaceError != nil {
			operationError = workspaceError
			break
		}
		result, operationError = readTextFilePinned(params, rootIdentity, hubInstance.policy.ReverseReadBytes, hubInstance.policy.FilesystemPolicy)
	case providerapi.WriteFileOperation:
		rootIdentity, workspaceError := hubInstance.resolveToolWorkspace(reverseRequest.SessionID)
		if workspaceError != nil {
			operationError = workspaceError
			break
		}
		result, operationError = writeTextFilePinned(params, rootIdentity, hubInstance.policy.FilesystemPolicy)
	case providerapi.CreateTerminalOperation:
		rootIdentity, workspaceError := hubInstance.resolveToolWorkspace(reverseRequest.SessionID)
		if workspaceError != nil {
			operationError = workspaceError
			break
		}
		result, operationError = hubInstance.terminals.createPinned(params, reverseRequest.SessionID, rootIdentity)
	case providerapi.ReadTerminalOperation:
		result, operationError = hubInstance.terminals.output(params, reverseRequest.SessionID)
	case providerapi.WaitTerminalOperation:
		result, operationError = hubInstance.terminals.waitForExit(operationContext, params, reverseRequest.SessionID)
	case providerapi.KillTerminalOperation:
		result, operationError = hubInstance.terminals.kill(params, reverseRequest.SessionID)
	case providerapi.ReleaseTerminalOperation:
		result, operationError = hubInstance.terminals.release(params, reverseRequest.SessionID)
	default:
		operationError = errors.New("unsupported provider reverse operation")
	}
	if operationError != nil {
		hubInstance.replyAgentForGeneration(agentGeneration, map[string]any{"jsonrpc": "2.0", "id": identifier, "error": map[string]any{"code": providerReverseOperationErrorCode, "message": operationError.Error()}})
		return
	}
	hubInstance.replyAgentForGeneration(agentGeneration, map[string]any{"jsonrpc": "2.0", "id": identifier, "result": result})
	var clientRPC map[string]any
	switch reverseRequest.Operation {
	case providerapi.ReadFileOperation, providerapi.WriteFileOperation:
		clientRPC = map[string]any{"method": method, "path": stringValue(params["path"]), "ok": true}
	case providerapi.CreateTerminalOperation:
		clientRPC = map[string]any{
			"method": method, "terminalId": result["terminalId"],
			"command": stringValue(params["command"]), "ok": true,
		}
	}
	if clientRPC != nil {
		hubInstance.broadcastNotification(providerapi.ClientOperationNotification, hubInstance.scopeSessionParams(reverseRequest.SessionID, clientRPC))
	}
}

func (hubInstance *Hub) forwardReverseRequest(object map[string]any, sessionID string, permission bool, agentGeneration uint64) {
	identifier, present := object["id"]
	if !present {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if permission && sessionID == "" {
		hubInstance.replyReverseUnavailable(identifier, true, "permission request missing sessionId", agentGeneration)
		return
	}
	params, _ := object["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	if sessionID != "" {
		params = hubInstance.scopeSessionParams(sessionID, params)
		object["params"] = params
	}

	targetClients := make(map[*clientConnection]struct{})
	hubInstance.stateMutex.Lock()
	if sessionID != "" && hubInstance.protocol != nil {
		cacheKey := sessionCacheKey(hubInstance.protocol.ID(), sessionID)
		for client := range hubInstance.sessionClients[cacheKey] {
			if _, connected := hubInstance.clients[client]; connected && !client.closed.Load() {
				targetClients[client] = struct{}{}
			}
		}
	} else if !permission {
		for client := range hubInstance.clients {
			if !client.closed.Load() {
				targetClients[client] = struct{}{}
			}
		}
	}
	if len(targetClients) == 0 {
		hubInstance.stateMutex.Unlock()
		hubInstance.replyReverseUnavailable(identifier, permission, "no matching remote client", agentGeneration)
		return
	}
	clientIdentifier := atomic.AddInt64(&hubInstance.nextID, 1)
	object["id"] = clientIdentifier
	routeKey := idKey(clientIdentifier)
	hubInstance.reverseRequests[routeKey] = reverseRequestRoute{
		identifier: identifier, agentGeneration: agentGeneration, clients: targetClients, permission: permission,
	}
	hubInstance.stateMutex.Unlock()

	raw := mustJSON(object)
	successfulSends := 0
	for client := range targetClients {
		if operationError := client.send(raw); operationError != nil {
			hubInstance.removeClient(client)
			continue
		}
		successfulSends++
	}
	if successfulSends == 0 {
		return
	}
	go hubInstance.expireReverseRequest(routeKey)
}

func (hubInstance *Hub) expireReverseRequest(routeKey string) {
	timer := time.NewTimer(hubInstance.policy.ReverseOperationTimeout)
	defer timer.Stop()
	select {
	case <-hubInstance.lifetimeContext.Done():
		return
	case <-timer.C:
	}
	hubInstance.stateMutex.Lock()
	route, present := hubInstance.reverseRequests[routeKey]
	if present {
		delete(hubInstance.reverseRequests, routeKey)
	}
	hubInstance.stateMutex.Unlock()
	if present {
		hubInstance.replyReverseUnavailable(route.identifier, route.permission, "remote client response timed out", route.agentGeneration)
	}
}

func (hubInstance *Hub) replyReverseUnavailable(identifier any, permission bool, message string, agentGeneration uint64) {
	if permission {
		hubInstance.replyAgentForGeneration(agentGeneration, map[string]any{
			"jsonrpc": "2.0", "id": identifier,
			"result": map[string]any{"outcome": map[string]any{"outcome": "cancelled"}},
		})
		return
	}
	hubInstance.replyAgentForGeneration(agentGeneration, map[string]any{
		"jsonrpc": "2.0", "id": identifier,
		"error": map[string]any{"code": providerReverseOperationErrorCode, "message": message},
	})
}

func (hubInstance *Hub) resolveToolWorkspace(sessionID string) (*fsapi.RootIdentity, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("sessionId required for workspace operation")
	}
	return hubInstance.ResolveSessionRoot(hubInstance.lifetimeContext, "", sessionID)
}

func readTextFile(params map[string]any, workingDirectory string, limitBytes int64) (map[string]any, error) {
	rootIdentity, operationError := fsapi.PinRoot(workingDirectory)
	if operationError != nil {
		return nil, operationError
	}
	return readTextFilePinned(params, rootIdentity, limitBytes, fsapi.Policy{MaxReadBytes: limitBytes, MaxWriteBytes: limitBytes, MaxListItems: 1})
}

func readTextFilePinned(params map[string]any, rootIdentity *fsapi.RootIdentity, limitBytes int64, policy fsapi.Policy) (map[string]any, error) {
	rawPath := stringValue(params["path"])
	if rawPath == "" {
		return nil, errors.New("path required")
	}
	filesystem, operationError := fsapi.NewPinned(rootIdentity, policy)
	if operationError != nil {
		return nil, operationError
	}
	defer filesystem.Close()
	file, operationError := filesystem.OpenRead(rawPath)
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
	reader := bufio.NewReaderSize(file, reverseFileReaderBufferBytes)
	var builder strings.Builder
	line := 1
	writtenLines := 0
	for {
		fragment, operationError := reader.ReadString('\n')
		include := line >= start && (!limitProvided || writtenLines < limit)
		if include && int64(builder.Len()) < limitBytes {
			remaining := int(limitBytes - int64(builder.Len()))
			if len(fragment) > remaining {
				fragment = fragment[:remaining]
			}
			builder.WriteString(fragment)
		}
		if include {
			writtenLines++
		}
		if int64(builder.Len()) >= limitBytes || (limitProvided && writtenLines >= limit) {
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

func writeTextFilePinned(params map[string]any, rootIdentity *fsapi.RootIdentity, policy fsapi.Policy) (map[string]any, error) {
	rawPath := stringValue(params["path"])
	if rawPath == "" {
		return nil, errors.New("path required")
	}
	filesystem, operationError := fsapi.NewPinned(rootIdentity, policy)
	if operationError != nil {
		return nil, operationError
	}
	defer filesystem.Close()
	content := stringValue(params["content"])
	if _, operationError := filesystem.Write(rawPath, content); operationError != nil {
		return nil, operationError
	}
	return map[string]any{}, nil
}
