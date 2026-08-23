package server

import "net/http"

func (server *Server) handleGitStatus(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	gitService, errorValue := server.gitForSession(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	result, errorValue := gitService.Status(request.Context())
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": false, "error": errorValue.Error(), "git": false})
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleGitDiff(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	gitService, errorValue := server.gitForSession(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	result, errorValue := gitService.Diff(request.Context(), query.Get("path"), parseBool(query.Get("staged")))
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": false, "error": errorValue.Error()})
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleGitLog(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	gitService, errorValue := server.gitForSession(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	result, errorValue := gitService.Log(request.Context(), intQuery(query.Get("n"), 12))
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": false, "error": errorValue.Error(), "commits": []any{}})
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleProjectContext(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	gitService, errorValue := server.gitForSession(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	result, errorValue := gitService.Project(request.Context())
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}
