package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/loops"
)

func (server *Server) handleLoopsList(responseWriter http.ResponseWriter, request *http.Request) {
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "jobs": server.loops.List(strings.TrimSpace(request.URL.Query().Get("sessionId")))})
}

func (server *Server) handleLoopsCreate(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	if errorValue := server.decodeJSON(responseWriter, request, &body, false); errorValue != nil {
		return
	}
	sessionID := strings.TrimSpace(stringValue(body["sessionId"]))
	prompt := strings.TrimSpace(stringValue(body["prompt"]))
	if sessionID == "" || prompt == "" {
		writeText(responseWriter, http.StatusBadRequest, "sessionId and prompt required")
		return
	}
	interval := body["interval"]
	if interval == nil {
		interval = body["interval_sec"]
	}
	if interval == nil {
		interval = body["every"]
	}
	seconds, label, errorValue := loopInterval(server.loops.Policy(), interval)
	if errorValue != nil {
		writeText(responseWriter, http.StatusBadRequest, "bad interval")
		return
	}
	workingDirectory, errorValue := server.resolveSessionWorkspace(request.Context(), stringValue(body["providerId"]), sessionID)
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	job, errorValue := server.loops.Create(sessionID, prompt, seconds, label, workingDirectory)
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusBadRequest, map[string]any{"ok": false, "error": errorValue.Error()})
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "job": job})
}

func (server *Server) handleLoopsStop(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	if !server.decodeLooseJSON(responseWriter, request, &body) {
		return
	}
	identifier := firstNonEmpty(stringValue(body["id"]), request.URL.Query().Get("id"))
	sessionID := firstNonEmpty(stringValue(body["sessionId"]), request.URL.Query().Get("sessionId"))
	removed, errorValue := server.loops.Stop(identifier, sessionID)
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "removed": removed, "count": len(removed)})
}

func (server *Server) providerRequestContext(request *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(request.Context(), server.configuration.Canonical.Tuning.HTTP.ProviderRequestTimeout.Duration)
}

func (server *Server) handleEffort(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	if errorValue := server.decodeJSON(responseWriter, request, &body, false); errorValue != nil {
		return
	}
	sessionID := strings.TrimSpace(stringValue(body["sessionId"]))
	effort := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringValue(body["effort"]), stringValue(body["reasoningEffort"]))))
	model := firstNonEmpty(stringValue(body["modelId"]), stringValue(body["model"]))
	if sessionID == "" || model == "" || effort == "" {
		writeText(responseWriter, http.StatusBadRequest, "sessionId, modelId, and effort required")
		return
	}
	valid := map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}
	if !valid[effort] {
		writeJSON(responseWriter, http.StatusInternalServerError, map[string]any{"ok": false, "error": "effort must be low|medium|high|xhigh (or none/minimal/max)"})
		return
	}
	requested := effort
	if effort == "max" {
		effort = "xhigh"
	}
	executionContext, cancel := server.providerRequestContext(request)
	method, params := server.protocol.EffortRequest(sessionID, model, effort)
	response, errorValue := server.hub.CallRPC(executionContext, method, params)
	cancel()
	if errorValue != nil {
		status := http.StatusInternalServerError
		if response != nil && response["error"] != nil {
			status = http.StatusBadRequest
		}
		writeJSON(responseWriter, status, map[string]any{"ok": false, "error": errorValue.Error(), "raw": response})
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "effort": requested, "modelId": model, "result": response["result"]})
}

func loopInterval(policy loops.Policy, raw any) (int, string, error) {
	if raw == nil || strings.TrimSpace(stringValue(raw)) == "" {
		seconds, label := policy.NormalizeInterval(int(policy.DefaultInterval / time.Second))
		return seconds, label, nil
	}
	switch value := raw.(type) {
	case float64:
		seconds, label := policy.NormalizeInterval(int(value))
		return seconds, label, nil
	case int:
		seconds, label := policy.NormalizeInterval(value)
		return seconds, label, nil
	case json.Number:
		if count, errorValue := value.Int64(); errorValue == nil {
			seconds, label := policy.NormalizeInterval(int(count))
			return seconds, label, nil
		}
		if count, errorValue := value.Float64(); errorValue == nil {
			seconds, label := policy.NormalizeInterval(int(count))
			return seconds, label, nil
		}
	}
	return policy.ParseInterval(stringValue(raw))
}
