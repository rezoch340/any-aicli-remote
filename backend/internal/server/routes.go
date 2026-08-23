package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const maxRequestBody = 8 * 1024 * 1024

func (server *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", server.handleIndex)
	mux.HandleFunc("GET /index.html", server.handleIndex)
	mux.HandleFunc("GET /watch", server.handleWatch)
	mux.HandleFunc("GET /watch.html", server.handleWatch)
	mux.HandleFunc("GET /config.json", server.handleConfig)
	mux.HandleFunc("GET /config", server.handleConfig)
	mux.HandleFunc("GET /health", server.handleHealth)
	mux.HandleFunc("GET /health/deep", server.handleHealthDeep)
	mux.HandleFunc("GET /pair", server.handlePair)
	mux.HandleFunc("GET /ws", server.hub.HandleWebSocket)
	mux.HandleFunc("GET /static/", http.NotFound)

	mux.HandleFunc("GET /api/fs/root", server.handleFSRoot)
	mux.HandleFunc("POST /api/fs/root", server.handleFSSetRoot)
	mux.HandleFunc("GET /api/fs/list", server.handleFSList)
	mux.HandleFunc("GET /api/fs/read", server.handleFSRead)
	mux.HandleFunc("POST /api/fs/write", server.handleFSWrite)
	mux.HandleFunc("POST /api/fs/mkdir", server.handleFSMkdir)
	mux.HandleFunc("GET /api/skills/list", server.handleSkills)

	mux.HandleFunc("GET /api/session/history", server.handleSessionHistory)
	mux.HandleFunc("GET /api/sessions", server.handleSessions)
	mux.HandleFunc("GET /api/sessions/{sessionID}/messages", server.handleSessionMessages)
	mux.HandleFunc("POST /api/session/titles", server.handleSessionTitles)
	mux.HandleFunc("GET /api/session/signals", server.handleSessionSignals)
	mux.HandleFunc("GET /api/session/archived", server.handleSessionArchivedGet)
	mux.HandleFunc("POST /api/session/archived", server.handleSessionArchivedSet)
	mux.HandleFunc("POST /api/session/rename", server.handleSessionRename)

	mux.HandleFunc("GET /api/room/feed", server.handleRoomFeed)
	mux.HandleFunc("POST /api/room/say", server.handleRoomSay)
	mux.HandleFunc("GET /api/room/members", server.handleRoomMembers)
	mux.HandleFunc("POST /api/room/clear", server.handleRoomClear)

	mux.HandleFunc("GET /api/voice/status", server.handleVoiceStatus)
	mux.HandleFunc("POST /api/tts", server.handleTTS)
	mux.HandleFunc("GET /api/git/status", server.handleGitStatus)
	mux.HandleFunc("GET /api/git/diff", server.handleGitDiff)
	mux.HandleFunc("GET /api/git/log", server.handleGitLog)
	mux.HandleFunc("GET /api/project/context", server.handleProjectContext)

	mux.HandleFunc("GET /api/stack/status", server.handleStackStatus)
	mux.HandleFunc("POST /api/stack/stop", server.handleStackStop)
	mux.HandleFunc("POST /api/stack/start", server.handleStackStart)
	mux.HandleFunc("POST /api/stack/shortcut", server.handleStackShortcut)

	mux.HandleFunc("GET /api/loops", server.handleLoopsList)
	mux.HandleFunc("POST /api/loops", server.handleLoopsCreate)
	mux.HandleFunc("DELETE /api/loops", server.handleLoopsStop)
	mux.HandleFunc("POST /api/loops/stop", server.handleLoopsStop)
	mux.HandleFunc("POST /api/effort", server.handleEffort)
	return http.MaxBytesHandler(mux, maxRequestBody)
}

func (server *Server) handleIndex(responseWriter http.ResponseWriter, _ *http.Request) {
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(responseWriter, daemonPage("Any AI CLI Remote", "The Any AI CLI Remote daemon is running.", server.configuration.PairingDeepLink(server.lanIP)))
}

func (server *Server) handleWatch(responseWriter http.ResponseWriter, _ *http.Request) {
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(responseWriter, daemonPage("Any AI CLI Remote Watch", "Use the native Android or iOS client for the full chat experience.", server.configuration.PairingDeepLink(server.lanIP)))
}

func daemonPage(title, message, deepLink string) string {
	link := ""
	if deepLink != "" {
		link = `<p><a href="` + html.EscapeString(deepLink) + `">Open native app</a></p>`
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + `</title><style>body{font:16px system-ui;max-width:42rem;margin:12vh auto;padding:0 24px;background:#090b0f;color:#eef2f7}main{padding:28px;border:1px solid #29313d;border-radius:18px;background:#11151b}a{color:#67e8f9}</style></head><body><main><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(message) + `</p>` + link + `</main></body></html>`
}

func (server *Server) configPayload() map[string]any {
	return map[string]any{
		"agent_host":  server.configuration.AgentHost,
		"agent_port":  server.configuration.AgentPort,
		"secret":      "(held server-side)",
		"provider_id": server.configuration.ProviderID,
		"runtime_dir": server.configuration.RuntimeDirectory,
		"ws_path":     "/ws",
		"lan_ip":      server.lanIP,
		"proxy":       false,
		"hub":         true,
		"ide":         false,
		"auth":        server.configuration.PairingSecret != "",
		"clients":     server.hub.ClientCount(),
		"features":    []string{"fs", "terminal", "multi-client-hub", "skills-scan", "git", "project-context", "session-cancel", "session-history", "permission-routing", "voice-tts", "remote-loop", "effort"},
	}
}

func (server *Server) writeRuntimeConfig() error {
	payload := server.configPayload()
	data, errorValue := json.MarshalIndent(payload, "", "  ")
	if errorValue != nil {
		return errorValue
	}
	return os.WriteFile(filepath.Join(server.configuration.DataDirectory, "runtime-config.json"), data, 0o600)
}

func (server *Server) handleConfig(responseWriter http.ResponseWriter, _ *http.Request) {
	responseWriter.Header().Set("Cache-Control", "no-store")
	writeJSON(responseWriter, http.StatusOK, server.configPayload())
}

func (server *Server) healthPayload() map[string]any {
	status := server.processStatus()
	hubUp := server.hub.AgentConnected()
	return map[string]any{
		"ok": true, "ui": true, "ready": hubUp && status.Listening,
		"agent_ws_local": fmt.Sprintf("ws://%s/ws", net.JoinHostPort(server.configuration.AgentHost, strconv.Itoa(server.configuration.AgentPort))),
		"detail":         "", "provider_id": server.configuration.ProviderID, "hub_clients": server.hub.ClientCount(),
		"hub_up": hubUp, "hub_err": server.hub.LastError(), "init_cached": server.hub.InitCached(),
		"agent_listening": status.Listening,
	}
}

func (server *Server) handleHealth(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.healthPayload())
}

func (server *Server) handleHealthDeep(responseWriter http.ResponseWriter, request *http.Request) {
	executionContext, cancel := context.WithTimeout(request.Context(), 6*time.Second)
	defer cancel()
	errorValue := server.hub.Ensure(executionContext)
	valid := errorValue == nil && server.hub.AgentConnected()
	detail := ""
	if errorValue != nil {
		detail = redactSecret(errorValue.Error(), server.configuration.AgentSecret, 200)
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{
		"ok":             valid,
		"agent_ws_local": fmt.Sprintf("ws://%s/ws", net.JoinHostPort(server.configuration.AgentHost, strconv.Itoa(server.configuration.AgentPort))),
		"detail":         detail, "provider_id": server.configuration.ProviderID, "hub_clients": server.hub.ClientCount(),
		"hub_up": server.hub.AgentConnected(), "hub_err": server.hub.LastError(), "init_cached": server.hub.InitCached(),
	})
}

func (server *Server) handlePair(responseWriter http.ResponseWriter, request *http.Request) {
	if !requestLoopback(request) {
		writeText(responseWriter, http.StatusForbidden, "pair is loopback-only")
		return
	}
	deep := server.configuration.PairingDeepLink(server.lanIP)
	pairing := server.configuration.PairingURL(server.lanIP)
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(responseWriter, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Pair Any AI CLI Remote</title><body style="font:16px system-ui;max-width:42rem;margin:3rem auto;padding:0 1rem;word-break:break-all"><h1>Pair Any AI CLI Remote</h1><p><a href="%s">Open native app</a></p><p>%s</p><p>Choose a workspace only when creating a session.</p></body>`, html.EscapeString(deep), html.EscapeString(pairing))
}
