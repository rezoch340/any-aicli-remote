package server

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/skills"
)

func (server *Server) handleFSRoot(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	filesystem, operationError := server.filesystemForSession(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if operationError != nil {
		writeWorkspaceError(responseWriter, operationError)
		return
	}
	defer filesystem.Close()
	writeJSON(responseWriter, http.StatusOK, filesystem.Info())
}

func (server *Server) handleFSSetRoot(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		ProviderID string `json:"providerId"`
		SessionID  string `json:"sessionId"`
	}
	if !server.decodeLooseJSON(responseWriter, request, &body) {
		return
	}
	filesystem, operationError := server.filesystemForSession(request.Context(), body.ProviderID, body.SessionID)
	if operationError != nil {
		writeWorkspaceError(responseWriter, operationError)
		return
	}
	defer filesystem.Close()
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "root": filesystem.Root(), "sessionId": body.SessionID})
}

func (server *Server) handleFSList(responseWriter http.ResponseWriter, request *http.Request) {
	path := request.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	query := request.URL.Query()
	filesystem, errorValue := server.filesystemForSession(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	defer filesystem.Close()
	result, errorValue := filesystem.List(path)
	if errorValue != nil {
		if errors.Is(errorValue, fsapi.NotDirectoryError) || errors.Is(errorValue, os.ErrNotExist) {
			writeText(responseWriter, http.StatusBadRequest, "not a directory")
			return
		}
		writeFSError(responseWriter, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleFSRead(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	filesystem, errorValue := server.filesystemForSession(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	defer filesystem.Close()
	result, errorValue := filesystem.Read(query.Get("path"))
	if errorValue != nil {
		writeFSError(responseWriter, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleFSWrite(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		ProviderID string  `json:"providerId"`
		SessionID  string  `json:"sessionId"`
		Path       string  `json:"path"`
		Content    *string `json:"content"`
	}
	if errorValue := server.decodeJSON(responseWriter, request, &body, false); errorValue != nil {
		return
	}
	if strings.TrimSpace(body.Path) == "" || body.Content == nil {
		writeText(responseWriter, http.StatusBadRequest, "path and content required")
		return
	}
	filesystem, errorValue := server.filesystemForSession(request.Context(), body.ProviderID, body.SessionID)
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	defer filesystem.Close()
	result, errorValue := filesystem.Write(body.Path, *body.Content)
	if errorValue != nil {
		if errors.Is(errorValue, fsapi.NotFileError) {
			writeText(responseWriter, http.StatusBadRequest, "path is directory")
			return
		}
		writeFSError(responseWriter, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleFSMkdir(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		ProviderID string `json:"providerId"`
		SessionID  string `json:"sessionId"`
		Path       string `json:"path"`
	}
	if errorValue := server.decodeJSON(responseWriter, request, &body, false); errorValue != nil {
		return
	}
	filesystem, errorValue := server.filesystemForSession(request.Context(), body.ProviderID, body.SessionID)
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	defer filesystem.Close()
	path, errorValue := filesystem.Mkdir(body.Path)
	if errorValue != nil {
		writeFSError(responseWriter, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (server *Server) handleSkills(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	workingDirectory, errorValue := server.resolveSessionWorkspace(request.Context(), query.Get("providerId"), firstNonEmpty(query.Get("sessionId"), query.Get("id")))
	if errorValue != nil {
		writeWorkspaceError(responseWriter, errorValue)
		return
	}
	roots := server.skillRoots.SkillRoots(workingDirectory)
	items, errorValue := skills.Scan(roots.Roots, server.skillsPolicy)
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "cwd": workingDirectory, "count": len(items), "skills": items})
}
