package hub

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type sessionWorkspaceBinding struct {
	rootIdentity *fsapi.RootIdentity
	lastActiveAt int64
}

func (hubInstance *Hub) captureSessionBindingLocked(prepared providerapi.PreparedRequest, response map[string]any) (providerapi.SessionBinding, bool) {
	if hubInstance.protocol == nil {
		return providerapi.SessionBinding{}, false
	}
	binding, valid := hubInstance.protocol.CaptureSessionBinding(prepared, response)
	if !valid || binding.ProviderID != hubInstance.protocol.ID() || binding.SessionID == "" || binding.WorkingDirectory == "" {
		return providerapi.SessionBinding{}, false
	}
	rootIdentity, operationError := fsapi.PinRoot(binding.WorkingDirectory)
	if operationError != nil {
		return providerapi.SessionBinding{}, false
	}
	cacheKey := sessionCacheKey(binding.ProviderID, binding.SessionID)
	if existingBinding := hubInstance.sessions[cacheKey]; existingBinding.rootIdentity != nil {
		if existingBinding.rootIdentity.Validate() != nil || existingBinding.rootIdentity.Path() != rootIdentity.Path() {
			return providerapi.SessionBinding{}, false
		}
		existingBinding.lastActiveAt = time.Now().UnixMilli()
		hubInstance.sessions[cacheKey] = existingBinding
		return binding, true
	}
	hubInstance.sessions[cacheKey] = sessionWorkspaceBinding{
		rootIdentity: rootIdentity,
		lastActiveAt: time.Now().UnixMilli(),
	}
	return binding, true
}

func (hubInstance *Hub) prepareClientRequest(operationContext context.Context, method string, params map[string]any) (providerapi.PreparedRequest, error) {
	prepared, operationError := hubInstance.protocol.PrepareClientRequest(operationContext, method, params)
	if operationError != nil || !prepared.RequiresSession {
		return prepared, operationError
	}
	workingDirectory, operationError := hubInstance.ResolveSessionWorkspace(operationContext, hubInstance.protocol.ID(), prepared.SessionID)
	if operationError != nil {
		return prepared, operationError
	}
	prepared.WorkingDirectory = workingDirectory
	if prepared.RestoresSession {
		params["cwd"] = workingDirectory
	}
	return prepared, nil
}

// ResolveSessionWorkspace is the single resolver for active and persisted
// session bindings. Active bindings win because a provider may not have
// persisted a newly-created session when its first prompt or tool call arrives.
func (hubInstance *Hub) ResolveSessionWorkspace(operationContext context.Context, providerID, sessionID string) (string, error) {
	rootIdentity, operationError := hubInstance.ResolveSessionRoot(operationContext, providerID, sessionID)
	if operationError != nil {
		return "", operationError
	}
	return rootIdentity.Path(), nil
}

// ResolveSessionRoot returns the immutable workspace identity captured when a
// session was bound. A changed path never causes an existing session to bind
// silently to a replacement directory.
func (hubInstance *Hub) ResolveSessionRoot(operationContext context.Context, providerID, sessionID string) (*fsapi.RootIdentity, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, providerapi.SessionRequiredError
	}
	configuredProviderID := ""
	if hubInstance.protocol != nil {
		configuredProviderID = strings.TrimSpace(hubInstance.protocol.ID())
	} else if hubInstance.catalog != nil {
		configuredProviderID = strings.TrimSpace(hubInstance.catalog.ID())
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = configuredProviderID
	}
	if configuredProviderID == "" || providerID != configuredProviderID {
		return nil, fmt.Errorf("%w: %s", providerapi.ProviderNotFoundError, providerID)
	}
	cacheKey := sessionCacheKey(providerID, sessionID)
	hubInstance.stateMutex.Lock()
	cachedBinding := hubInstance.sessions[cacheKey]
	if cachedBinding.rootIdentity != nil {
		cachedBinding.lastActiveAt = time.Now().UnixMilli()
		hubInstance.sessions[cacheKey] = cachedBinding
	}
	hubInstance.stateMutex.Unlock()
	if cachedBinding.rootIdentity != nil {
		if operationError := cachedBinding.rootIdentity.Validate(); operationError != nil {
			return nil, operationError
		}
		return cachedBinding.rootIdentity, nil
	}
	if hubInstance.catalog == nil {
		return nil, fmt.Errorf("workspace unavailable for session %s", sessionID)
	}
	metadata, operationError := hubInstance.catalog.ResolveSession(operationContext, sessionID)
	if operationError != nil {
		return nil, operationError
	}
	if metadata.ProviderID != "" && metadata.ProviderID != providerID {
		return nil, fmt.Errorf("session provider mismatch: %s", metadata.ProviderID)
	}
	workingDirectory, operationError := providerapi.CanonicalDirectory(metadata.ProjectDirectory)
	if operationError != nil {
		return nil, operationError
	}
	rootIdentity, operationError := fsapi.PinRoot(workingDirectory)
	if operationError != nil {
		return nil, operationError
	}
	hubInstance.stateMutex.Lock()
	existingBinding := hubInstance.sessions[cacheKey]
	if existingBinding.rootIdentity == nil {
		hubInstance.sessions[cacheKey] = sessionWorkspaceBinding{rootIdentity: rootIdentity, lastActiveAt: time.Now().UnixMilli()}
	} else {
		existingBinding.lastActiveAt = time.Now().UnixMilli()
		hubInstance.sessions[cacheKey] = existingBinding
		rootIdentity = existingBinding.rootIdentity
	}
	hubInstance.stateMutex.Unlock()
	if operationError := rootIdentity.Validate(); operationError != nil {
		return nil, operationError
	}
	return rootIdentity, nil
}

// ActiveSessions returns authoritative in-memory bindings, including newly
// created sessions that the provider has not persisted to its catalog yet.
func (hubInstance *Hub) ActiveSessions(providerID string) ([]providerapi.SessionMetadata, error) {
	configuredProviderID := ""
	if hubInstance.protocol != nil {
		configuredProviderID = strings.TrimSpace(hubInstance.protocol.ID())
	} else if hubInstance.catalog != nil {
		configuredProviderID = strings.TrimSpace(hubInstance.catalog.ID())
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = configuredProviderID
	}
	if configuredProviderID == "" || providerID != configuredProviderID {
		return nil, fmt.Errorf("%w: %s", providerapi.ProviderNotFoundError, providerID)
	}
	prefix := providerID + "\x00"
	hubInstance.stateMutex.Lock()
	sessions := make([]providerapi.SessionMetadata, 0, len(hubInstance.sessions))
	for cacheKey, binding := range hubInstance.sessions {
		if !strings.HasPrefix(cacheKey, prefix) || binding.rootIdentity == nil {
			continue
		}
		sessionID := strings.TrimPrefix(cacheKey, prefix)
		if sessionID == "" {
			continue
		}
		sessions = append(sessions, providerapi.SessionMetadata{
			ProviderID: providerID, SessionID: sessionID, ProjectDirectory: binding.rootIdentity.Path(),
			LastActiveAt: binding.lastActiveAt,
		})
	}
	hubInstance.stateMutex.Unlock()
	providerapi.SortSessions(sessions)
	return sessions, nil
}

func sessionCacheKey(providerID, sessionID string) string {
	return providerID + "\x00" + sessionID
}

// subscribeSessionClientLocked records only an authenticated client's active
// session traffic so reverse requests can be delivered without leaking them to
// unrelated device conversations. The caller must hold stateMutex.
func (hubInstance *Hub) subscribeSessionClientLocked(client *clientConnection, providerID, sessionID string) {
	if client == nil || client.closed.Load() {
		return
	}
	providerID = strings.TrimSpace(providerID)
	sessionID = strings.TrimSpace(sessionID)
	if providerID == "" || sessionID == "" {
		return
	}
	for existingKey, existingClients := range hubInstance.sessionClients {
		delete(existingClients, client)
		if len(existingClients) == 0 {
			delete(hubInstance.sessionClients, existingKey)
		}
	}
	cacheKey := sessionCacheKey(providerID, sessionID)
	clients := hubInstance.sessionClients[cacheKey]
	if clients == nil {
		clients = make(map[*clientConnection]struct{})
		hubInstance.sessionClients[cacheKey] = clients
	}
	clients[client] = struct{}{}
}

func (hubInstance *Hub) scopeSessionParams(sessionID string, params map[string]any) map[string]any {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || hubInstance.protocol == nil {
		return params
	}
	scopedParams := make(map[string]any, len(params)+2)
	for key, value := range params {
		scopedParams[key] = value
	}
	scopedParams["providerId"] = hubInstance.protocol.ID()
	scopedParams["sessionId"] = sessionID
	return scopedParams
}
