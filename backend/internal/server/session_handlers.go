package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	"github.com/rezoch340/any-aicli-remote/backend/internal/sessionapi"
)

func (server *Server) handleSessions(responseWriter http.ResponseWriter, request *http.Request) {
	providerID := strings.TrimSpace(request.URL.Query().Get("providerId"))
	if providerID == "" {
		providerID = server.configuration.ProviderID
	}
	sessions, operationError := server.providers.ScanSessions(request.Context(), providerID)
	if operationError != nil {
		if errors.Is(operationError, providerapi.ProviderNotFoundError) {
			writeAPIError(responseWriter, http.StatusNotFound, operationError)
		} else {
			writeAPIError(responseWriter, http.StatusInternalServerError, operationError)
		}
		return
	}
	activeSessions, operationError := server.hub.ActiveSessions(providerID)
	if operationError != nil {
		writeAPIError(responseWriter, http.StatusNotFound, operationError)
		return
	}
	sessions = providerapi.MergeSessionMetadata(sessions, activeSessions)
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "providerId": providerID, "sessions": sessions, "count": len(sessions)})
}

func (server *Server) handleSessionMessages(responseWriter http.ResponseWriter, request *http.Request) {
	providerID := strings.TrimSpace(request.URL.Query().Get("providerId"))
	if providerID == "" {
		providerID = server.configuration.ProviderID
	}
	providerInstance, operationError := server.providers.Provider(providerID)
	if operationError != nil {
		writeAPIError(responseWriter, http.StatusNotFound, operationError)
		return
	}
	sessionID := strings.TrimSpace(request.PathValue("sessionID"))
	if sessionID == "" {
		writeAPIError(responseWriter, http.StatusBadRequest, providerapi.SessionRequiredError)
		return
	}
	activeSessions, operationError := server.hub.ActiveSessions(providerID)
	if operationError != nil {
		writeAPIError(responseWriter, http.StatusNotFound, operationError)
		return
	}
	catalogMetadata, catalogError := providerInstance.ResolveSession(request.Context(), sessionID)
	if catalogError != nil && !errors.Is(catalogError, providerapi.SessionNotFoundError) {
		writeAPIError(responseWriter, http.StatusInternalServerError, catalogError)
		return
	}
	catalogSessions := make([]providerapi.SessionMetadata, 0, 1)
	if catalogError == nil {
		catalogSessions = append(catalogSessions, catalogMetadata)
	}
	mergedSessions := providerapi.MergeSessionMetadata(catalogSessions, activeSessions)
	var metadata providerapi.SessionMetadata
	for _, candidate := range mergedSessions {
		if candidate.ProviderID == providerID && candidate.SessionID == sessionID {
			metadata = candidate
			break
		}
	}
	if metadata.SessionID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, providerapi.SessionNotFoundError)
		return
	}
	messages := []providerapi.Message{}
	if catalogError == nil {
		messages, operationError = providerInstance.LoadMessages(request.Context(), sessionID)
	}
	if operationError != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, operationError)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{
		"ok": true, "providerId": providerID, "sessionId": sessionID,
		"session": metadata, "messages": messages, "count": len(messages),
	})
}

func (server *Server) handleSessionHistory(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	sessionID := firstNonEmpty(query.Get("sessionId"), query.Get("id"))
	live := parseBool(query.Get("live"))
	defaultLimit := 100
	if live {
		defaultLimit = 400
	}
	limit := 0
	if query.Has("limit") {
		limit = intQuery(query.Get("limit"), defaultLimit)
	}
	since := int64Query(firstNonEmpty(query.Get("since"), query.Get("since_bytes")), 0)
	var before *int64
	beforeRaw := firstNonEmpty(query.Get("before"), query.Get("before_bytes"))
	if beforeRaw != "" {
		if parsed, errorValue := strconv.ParseInt(strings.TrimSpace(beforeRaw), 10, 64); errorValue == nil {
			before = &parsed
		}
	}
	maxBytes := int64(0)
	if query.Has("max_bytes") {
		fallback := int64(400_000)
		if live {
			fallback = 512_000
		}
		maxBytes = int64Query(query.Get("max_bytes"), fallback)
	}
	result, errorValue := server.session.History(request.Context(), sessionapi.HistoryQuery{
		ProviderID:  query.Get("providerId"),
		SessionID:   sessionID,
		Live:        live,
		Limit:       limit,
		SinceBytes:  since,
		BeforeBytes: before,
		MaxBytes:    maxBytes,
		ChatOnly:    parseBool(firstNonEmpty(query.Get("chat_only"), query.Get("messages"))),
	})
	if errorValue != nil {
		switch {
		case errors.Is(errorValue, sessionapi.SessionRequiredError):
			writeText(responseWriter, http.StatusBadRequest, errorValue.Error())
		case errors.Is(errorValue, sessionapi.NotFoundError):
			writeJSON(responseWriter, http.StatusNotFound, map[string]any{"ok": false, "error": sessionapi.NotFoundError.Error(), "sessionId": result.SessionID, "cwd": result.WorkingDirectory, "events": result.Events, "meta": result.Meta})
		default:
			writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		}
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleSessionTitles(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	decodeLooseJSON(request, &body)
	raw := body["ids"]
	if !truthyValue(raw) {
		raw = body["sessionIds"]
	}
	ids := stringList(raw)
	result, errorValue := server.session.Titles(request.Context(), stringValue(body["providerId"]), ids)
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleSessionSignals(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	sessionID := firstNonEmpty(query.Get("sessionId"), query.Get("id"))
	result, errorValue := server.session.Signals(request.Context(), query.Get("providerId"), sessionID)
	if errorValue != nil {
		switch {
		case errors.Is(errorValue, sessionapi.SessionRequiredError):
			writeText(responseWriter, http.StatusBadRequest, errorValue.Error())
		case errors.Is(errorValue, sessionapi.NotFoundError):
			writeJSON(responseWriter, http.StatusNotFound, map[string]any{"ok": false, "error": sessionapi.NotFoundError.Error(), "sessionId": sessionID})
		default:
			writeJSON(responseWriter, http.StatusInternalServerError, map[string]any{"ok": false, "error": errorValue.Error(), "sessionId": sessionID})
		}
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleSessionArchivedGet(responseWriter http.ResponseWriter, _ *http.Request) {
	result, errorValue := server.session.Archived()
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleSessionArchivedSet(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	decodeLooseJSON(request, &body)
	archiveRequest := sessionapi.SetArchivedRequest{
		ID:        stringValue(body["id"]),
		SessionID: stringValue(body["sessionId"]),
	}
	if raw, exists := body["ids"]; exists {
		if _, valid := raw.([]any); valid {
			archiveRequest.IDs = stringList(raw)
		}
	}
	if raw, exists := body["archived"]; exists && raw != nil {
		value := boolValue(raw)
		archiveRequest.Archived = &value
	}
	result, errorValue := server.session.SetArchived(archiveRequest)
	if errorValue != nil {
		if errors.Is(errorValue, sessionapi.BadRequestError) {
			message := strings.TrimPrefix(errorValue.Error(), sessionapi.BadRequestError.Error()+": ")
			writeText(responseWriter, http.StatusBadRequest, message)
		} else {
			writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		}
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleSessionRename(responseWriter http.ResponseWriter, request *http.Request) {
	var renameRequest sessionapi.RenameRequest
	if errorValue := decodeJSON(responseWriter, request, &renameRequest, false); errorValue != nil {
		return
	}
	result, errorValue := server.session.Rename(request.Context(), renameRequest)
	if errorValue != nil {
		switch {
		case errors.Is(errorValue, sessionapi.SessionRequiredError), errors.Is(errorValue, sessionapi.TitleRequiredError):
			writeText(responseWriter, http.StatusBadRequest, errorValue.Error())
		case errors.Is(errorValue, sessionapi.NotFoundError):
			writeJSON(responseWriter, http.StatusNotFound, map[string]any{"ok": false, "error": sessionapi.NotFoundError.Error(), "sessionId": result.SessionID})
		default:
			writeJSON(responseWriter, http.StatusInternalServerError, map[string]any{"ok": false, "error": errorValue.Error(), "sessionId": result.SessionID})
		}
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func int64Query(value string, fallback int64) int64 {
	parsed, errorValue := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if errorValue != nil {
		return fallback
	}
	return parsed
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringValue(item))
		}
		return out
	default:
		return []string{}
	}
}

func truthyValue(value any) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed != ""
	case []any:
		return len(typed) != 0
	case []string:
		return len(typed) != 0
	default:
		return true
	}
}
