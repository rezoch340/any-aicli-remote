package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/gitapi"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func (server *Server) resolveSessionWorkspace(operationContext context.Context, providerID, sessionID string) (string, error) {
	return server.hub.ResolveSessionWorkspace(operationContext, providerID, sessionID)
}

func (server *Server) filesystemForSession(operationContext context.Context, providerID, sessionID string) (*fsapi.Service, error) {
	rootIdentity, operationError := server.hub.ResolveSessionRoot(operationContext, providerID, sessionID)
	if operationError != nil {
		return nil, operationError
	}
	return fsapi.NewPinned(rootIdentity)
}

func (server *Server) gitForSession(operationContext context.Context, providerID, sessionID string) (*gitapi.Service, error) {
	rootIdentity, operationError := server.hub.ResolveSessionRoot(operationContext, providerID, sessionID)
	if operationError != nil {
		return nil, operationError
	}
	return gitapi.NewPinned(rootIdentity), nil
}

func writeWorkspaceError(responseWriter http.ResponseWriter, operationError error) {
	switch {
	case errors.Is(operationError, providerapi.SessionRequiredError):
		writeText(responseWriter, http.StatusBadRequest, operationError.Error())
	case errors.Is(operationError, providerapi.ProviderNotFoundError), errors.Is(operationError, providerapi.SessionNotFoundError):
		writeAPIError(responseWriter, http.StatusNotFound, operationError)
	default:
		writeAPIError(responseWriter, http.StatusBadRequest, operationError)
	}
}
