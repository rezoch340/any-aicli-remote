package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/grok-remote/grok-remote-app/backend/internal/fsapi"
	"github.com/grok-remote/grok-remote-app/backend/internal/loops"
	processapi "github.com/grok-remote/grok-remote-app/backend/internal/process"
	"github.com/grok-remote/grok-remote-app/backend/internal/room"
	"github.com/grok-remote/grok-remote-app/backend/internal/skills"
	"github.com/grok-remote/grok-remote-app/backend/internal/voice"
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
	_, _ = io.WriteString(responseWriter, daemonPage("Grok Remote", "The native Grok Remote daemon is running.", server.configuration.PairingDeepLink(server.lanIP, server.filesystem.Root())))
}

func (server *Server) handleWatch(responseWriter http.ResponseWriter, _ *http.Request) {
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(responseWriter, daemonPage("Grok Remote Watch", "Use the native Android or iOS client for the full chat experience.", server.configuration.PairingDeepLink(server.lanIP, server.filesystem.Root())))
}

func daemonPage(title, message, deepLink string) string {
	link := ""
	if deepLink != "" {
		link = `<p><a href="` + html.EscapeString(deepLink) + `">Open native app</a></p>`
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + `</title><style>body{font:16px system-ui;max-width:42rem;margin:12vh auto;padding:0 24px;background:#090b0f;color:#eef2f7}main{padding:28px;border:1px solid #29313d;border-radius:18px;background:#11151b}a{color:#67e8f9}</style></head><body><main><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(message) + `</p>` + link + `</main></body></html>`
}

func (server *Server) configPayload() map[string]any {
	pairing := server.configuration.PairingURL(server.lanIP)
	watch := pairing
	if parsed, errorValue := url.Parse(pairing); errorValue == nil {
		parsed.Path = "/watch"
		watch = parsed.String()
	}
	return map[string]any{
		"agent_host": server.configuration.AgentHost,
		"agent_port": server.configuration.AgentPort,
		"secret":     "(held server-side)",
		"cwd":        server.filesystem.Root(),
		"ws_url":     fmt.Sprintf("ws://%s/ws", net.JoinHostPort(server.lanIP, strconv.Itoa(server.configuration.Port))),
		"ws_path":    "/ws",
		"ui":         pairing,
		"watch":      watch,
		"lan_ip":     server.lanIP,
		"proxy":      true,
		"hub":        true,
		"ide":        true,
		"auth":       server.configuration.Secret != "",
		"clients":    server.hub.ClientCount(),
		"features":   []string{"fs", "ide", "review", "multi-client-hub", "skills-scan", "git", "project-context", "stop-turn", "todos", "voice-tts", "voice-go", "xr-ar", "watch-companion", "msg-queue", "remote-loop", "effort"},
	}
}

func (server *Server) writeRuntimeConfig() error {
	data, errorValue := json.MarshalIndent(server.configPayload(), "", "  ")
	if errorValue != nil {
		return errorValue
	}
	return os.WriteFile(filepath.Join(server.configuration.DataDirectory, "runtime-config.json"), data, 0o600)
}

func (server *Server) handleConfig(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.configPayload())
}

func (server *Server) healthPayload() map[string]any {
	status := server.processStatus()
	hubUp := server.hub.AgentConnected()
	return map[string]any{
		"ok": true, "ui": true, "ready": hubUp && status.Listening,
		"agent_ws_local": fmt.Sprintf("ws://%s/ws", net.JoinHostPort(server.configuration.AgentHost, strconv.Itoa(server.configuration.AgentPort))),
		"detail":         "", "cwd": server.filesystem.Root(), "hub_clients": server.hub.ClientCount(),
		"hub_up": hubUp, "hub_err": server.hub.LastError(), "init_cached": server.hub.InitCached(),
		"agent_listening": status.Listening,
	}
}

func (server *Server) handleHealth(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.healthPayload())
}

func (server *Server) handleHealthDeep(responseWriter http.ResponseWriter, request *http.Request) {
	valid := false
	detail := ""
	executionContext, cancel := context.WithTimeout(request.Context(), 6*time.Second)
	defer cancel()
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	connection, _, errorValue := dialer.DialContext(executionContext, server.configuration.AgentWebSocketURL(), nil)
	if errorValue == nil {
		defer connection.Close()
		_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
		_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
		errorValue = connection.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": 1, "clientInfo": map[string]any{"name": "health", "version": "0"}, "clientCapabilities": map[string]any{}}})
		if errorValue == nil {
			var response map[string]any
			errorValue = connection.ReadJSON(&response)
			valid = errorValue == nil && response["result"] != nil
		}
	}
	if errorValue != nil {
		detail = redactSecret(errorValue.Error(), server.configuration.Secret, 200)
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{
		"ok":             valid,
		"agent_ws_local": fmt.Sprintf("ws://%s/ws", net.JoinHostPort(server.configuration.AgentHost, strconv.Itoa(server.configuration.AgentPort))),
		"detail":         detail, "cwd": server.filesystem.Root(), "hub_clients": server.hub.ClientCount(),
		"hub_up": server.hub.AgentConnected(), "hub_err": server.hub.LastError(), "init_cached": server.hub.InitCached(),
	})
}

func (server *Server) handlePair(responseWriter http.ResponseWriter, request *http.Request) {
	if !requestLoopback(request) {
		writeText(responseWriter, http.StatusForbidden, "pair is loopback-only")
		return
	}
	deep := server.configuration.PairingDeepLink(server.lanIP, server.filesystem.Root())
	pairing := server.configuration.PairingURL(server.lanIP)
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(responseWriter, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Pair Grok Remote</title><body style="font:16px system-ui;max-width:42rem;margin:3rem auto;padding:0 1rem;word-break:break-all"><h1>Pair Grok Remote</h1><p><a href="%s">Open native app</a></p><p>%s</p><p>Workspace: %s</p></body>`, html.EscapeString(deep), html.EscapeString(pairing), html.EscapeString(server.filesystem.Root()))
}

func (server *Server) handleFSRoot(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.filesystem.Info())
}

func (server *Server) handleFSSetRoot(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		Path             string `json:"path"`
		WorkingDirectory string `json:"cwd"`
	}
	decodeLooseJSON(request, &body)
	path := firstNonEmpty(body.Path, body.WorkingDirectory)
	info, errorValue := server.filesystem.SetRoot(path)
	if errorValue != nil {
		if errors.Is(errorValue, fsapi.NotDirectoryError) || errors.Is(errorValue, os.ErrNotExist) {
			writeText(responseWriter, http.StatusBadRequest, "not a directory")
			return
		}
		writeFSError(responseWriter, errorValue)
		return
	}
	server.processMutex.Lock()
	server.process.Config.WorkingDirectory = info.Root
	server.processMutex.Unlock()
	_ = server.writeRuntimeConfig()
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "root": info.Root})
}

func (server *Server) handleFSList(responseWriter http.ResponseWriter, request *http.Request) {
	path := request.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	result, errorValue := server.filesystem.List(path)
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
	result, errorValue := server.filesystem.Read(request.URL.Query().Get("path"))
	if errorValue != nil {
		writeFSError(responseWriter, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleFSWrite(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	if errorValue := decodeJSON(responseWriter, request, &body, false); errorValue != nil {
		return
	}
	if strings.TrimSpace(body.Path) == "" || body.Content == nil {
		writeText(responseWriter, http.StatusBadRequest, "path and content required")
		return
	}
	result, errorValue := server.filesystem.Write(body.Path, *body.Content)
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
		Path string `json:"path"`
	}
	if errorValue := decodeJSON(responseWriter, request, &body, false); errorValue != nil {
		return
	}
	path, errorValue := server.filesystem.Mkdir(body.Path)
	if errorValue != nil {
		writeFSError(responseWriter, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (server *Server) handleSkills(responseWriter http.ResponseWriter, request *http.Request) {
	workingDirectory := firstNonEmpty(request.URL.Query().Get("cwd"), server.filesystem.Root())
	items, errorValue := skills.Scan(workingDirectory)
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "cwd": workingDirectory, "count": len(items), "skills": items})
}

func (server *Server) handleRoomFeed(responseWriter http.ResponseWriter, request *http.Request) {
	messages, errorValue := server.room.FeedString(request.URL.Query().Get("since"), request.URL.Query().Get("limit"))
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	last := intQuery(request.URL.Query().Get("since"), 0)
	if len(messages) > 0 {
		last = messages[len(messages)-1].ID
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "messages": messages, "last": last, "limit": room.Limit})
}

func (server *Server) handleRoomSay(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		Who  string `json:"who"`
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	decodeLooseJSON(request, &body)
	result := server.room.Say(firstNonEmpty(body.Who, request.URL.Query().Get("who"), "agent"), firstNonEmpty(body.Text, request.URL.Query().Get("text")), firstNonEmpty(body.Kind, "say"))
	status := http.StatusOK
	if !result.OK {
		status = http.StatusInternalServerError
		if result.Error == "empty message" {
			status = http.StatusBadRequest
		}
	}
	writeJSON(responseWriter, status, result)
}

func (server *Server) handleRoomMembers(responseWriter http.ResponseWriter, _ *http.Request) {
	members, errorValue := server.room.Members(15 * time.Minute)
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "members": members})
}

func (server *Server) handleRoomClear(responseWriter http.ResponseWriter, _ *http.Request) {
	if errorValue := server.room.Clear(); errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true})
}

func (server *Server) handleVoiceStatus(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.voice.Status())
}

func (server *Server) handleTTS(responseWriter http.ResponseWriter, request *http.Request) {
	var input voice.Request
	decodeLooseJSON(request, &input)
	audio, errorValue := server.voice.Synthesize(request.Context(), input)
	if errorValue != nil {
		switch {
		case errors.Is(errorValue, voice.APIKeyMissingError):
			writeJSON(responseWriter, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": errorValue.Error()})
		case errors.Is(errorValue, voice.TextRequiredError):
			writeText(responseWriter, http.StatusBadRequest, errorValue.Error())
		default:
			var upstream *voice.UpstreamError
			if errors.As(errorValue, &upstream) {
				status := http.StatusBadRequest
				if upstream.Status >= 500 {
					status = http.StatusBadGateway
				}
				writeJSON(responseWriter, status, map[string]any{"ok": false, "error": upstream.Error(), "status": upstream.Status})
				return
			}
			writeJSON(responseWriter, http.StatusBadGateway, map[string]any{"ok": false, "error": truncate(errorValue.Error(), 300)})
		}
		return
	}
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", audio.ContentType)
	responseWriter.Header().Set("X-Voice-Id", audio.VoiceID)
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write(audio.Data)
}

func (server *Server) handleGitStatus(responseWriter http.ResponseWriter, request *http.Request) {
	result, errorValue := server.git.Status(request.Context())
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": false, "error": errorValue.Error(), "git": false})
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleGitDiff(responseWriter http.ResponseWriter, request *http.Request) {
	result, errorValue := server.git.Diff(request.Context(), request.URL.Query().Get("path"), parseBool(request.URL.Query().Get("staged")))
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": false, "error": errorValue.Error()})
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleGitLog(responseWriter http.ResponseWriter, request *http.Request) {
	result, errorValue := server.git.Log(request.Context(), intQuery(request.URL.Query().Get("n"), 12))
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": false, "error": errorValue.Error(), "commits": []any{}})
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleProjectContext(responseWriter http.ResponseWriter, request *http.Request) {
	result, errorValue := server.git.Project(request.Context())
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}

func (server *Server) handleStackStatus(responseWriter http.ResponseWriter, _ *http.Request) {
	uiProcessIDs, _ := processapi.ListenProcessIDsPort(server.configuration.Port, true)
	status := server.processStatus()
	writeJSON(responseWriter, http.StatusOK, map[string]any{
		"ok": true, "ui_port": server.configuration.Port, "agent_port": server.configuration.AgentPort,
		"ui_pids": uiProcessIDs, "agent_pids": status.ProcessIDs, "self_pid": os.Getpid(),
		"lan": server.lanIP, "cwd": server.filesystem.Root(), "hub_up": server.hub.AgentConnected(),
		"hub_err": server.hub.LastError(), "agent_listening": status.Listening,
	})
}

func (server *Server) handleStackStop(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		KeepAgent bool `json:"keep_agent"`
	}
	decodeLooseJSON(request, &body)
	status := server.processStatus()
	agentProcessIDs := status.OwnedProcessIDs
	if body.KeepAgent {
		agentProcessIDs = []int{}
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "stopping": true, "self_pid": os.Getpid(), "agent_pids": agentProcessIDs, "message": "Remote UI stopping; agent serve will stop unless keep_agent"})
	go func() {
		time.Sleep(350 * time.Millisecond)
		server.requestStop(body.KeepAgent)
	}()
}

func (server *Server) handleStackStart(responseWriter http.ResponseWriter, request *http.Request) {
	server.stackStartMutex.Lock()
	defer server.stackStartMutex.Unlock()
	if server.closing.Load() {
		writeJSON(responseWriter, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "server is stopping", "started": false})
		return
	}

	var body map[string]any
	decodeLooseJSON(request, &body)
	force := boolValue(body["force"]) || boolValue(body["restart"])
	workingDirectory := stringValue(body["cwd"])

	server.agentLifecycleMutex.Lock()
	if server.closing.Load() {
		server.agentLifecycleMutex.Unlock()
		writeJSON(responseWriter, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "server is stopping", "started": false})
		return
	}
	server.processMutex.Lock()
	if workingDirectory != "" {
		if info, errorValue := os.Stat(workingDirectory); errorValue == nil && info.IsDir() {
			server.process.Config.WorkingDirectory = workingDirectory
		}
	}
	status := server.process.Status()
	attempts := []map[string]any{}
	started := false
	message := "agent ready"
	var startResult processapi.StartResult
	var startError error
	needWait := false
	reason := ""
	if force || !status.Listening {
		reason = "missing"
		if force {
			reason = "force"
			server.hub.DisconnectAgent("agent force restart")
		}
		startResult, startError = server.process.Start(force)
		needWait = startError == nil
	}
	server.processMutex.Unlock()
	listening := status.Listening
	if needWait {
		waitContext, waitCancel := context.WithTimeout(request.Context(), 24*time.Second)
		listening = server.waitForAgent(waitContext)
		waitCancel()
	}
	server.agentLifecycleMutex.Unlock()

	if reason != "" {
		if startError != nil {
			attempts = append(attempts, map[string]any{"reason": reason, "listen": false, "killed": []int{}})
			writeJSON(responseWriter, http.StatusInternalServerError, map[string]any{"ok": false, "error": startError.Error(), "started": false, "attempts": attempts, "hub_err": server.hub.LastError()})
			return
		}
		attempts = append(attempts, map[string]any{"reason": reason, "listen": listening, "killed": []int{}})
		if !listening {
			writeJSON(responseWriter, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("agent did not bind :%d after %s", server.configuration.AgentPort, reason), "started": startResult.Started, "attempts": attempts, "hub_err": server.hub.LastError()})
			return
		}
		started = startResult.Started
		message = startResult.Message
	}
	executionContext, cancel := context.WithTimeout(request.Context(), 18*time.Second)
	errorValue := server.hub.Ensure(executionContext)
	cancel()
	status = server.processStatus()
	if errorValue != nil && status.Owned {
		restartContext, restartCancel := context.WithTimeout(request.Context(), 24*time.Second)
		restartAttempt, restartError := server.restartOwnedAgentForAuthentication(restartContext)
		restartCancel()
		if restartAttempt.attempted {
			attempts = append(attempts, map[string]any{"reason": "hub-auth-retry", "listen": restartAttempt.listening, "killed": []int{}})
		}
		if restartError == nil && restartAttempt.listening {
			started = started || restartAttempt.result.Started
			executionContext, cancel = context.WithTimeout(request.Context(), 18*time.Second)
			errorValue = server.hub.Ensure(executionContext)
			cancel()
		} else {
			errorValue = restartError
		}
		status = server.processStatus()
	}
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": errorValue.Error(), "message": message, "started": started, "attempts": attempts, "agent_pids": status.ProcessIDs, "hub_up": false, "hub_err": server.hub.LastError(), "hint": "Check agent secret and logs; foreign listeners are never killed automatically"})
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "message": message, "killed": []int{}, "started": started, "attempts": attempts, "agent_pids": status.ProcessIDs, "hub_up": true, "hub_err": ""})
}

func (server *Server) handleStackShortcut(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusNotFound, map[string]any{"ok": false, "error": "install-shortcut.ps1 missing"})
}

func (server *Server) handleLoopsList(responseWriter http.ResponseWriter, request *http.Request) {
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "jobs": server.loops.List(strings.TrimSpace(request.URL.Query().Get("sessionId")))})
}

func (server *Server) handleLoopsCreate(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	if errorValue := decodeJSON(responseWriter, request, &body, false); errorValue != nil {
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
	seconds, label, errorValue := loopInterval(interval)
	if errorValue != nil {
		writeText(responseWriter, http.StatusBadRequest, "bad interval")
		return
	}
	job, errorValue := server.loops.Create(sessionID, prompt, seconds, label, firstNonEmpty(stringValue(body["cwd"]), server.filesystem.Root()))
	if errorValue != nil {
		writeJSON(responseWriter, http.StatusBadRequest, map[string]any{"ok": false, "error": errorValue.Error()})
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "job": job})
}

func (server *Server) handleLoopsStop(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	decodeLooseJSON(request, &body)
	identifier := firstNonEmpty(stringValue(body["id"]), request.URL.Query().Get("id"))
	sessionID := firstNonEmpty(stringValue(body["sessionId"]), request.URL.Query().Get("sessionId"))
	removed, errorValue := server.loops.Stop(identifier, sessionID)
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "removed": removed, "count": len(removed)})
}

func (server *Server) handleEffort(responseWriter http.ResponseWriter, request *http.Request) {
	var body map[string]any
	if errorValue := decodeJSON(responseWriter, request, &body, false); errorValue != nil {
		return
	}
	sessionID := strings.TrimSpace(stringValue(body["sessionId"]))
	effort := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringValue(body["effort"]), stringValue(body["reasoningEffort"]))))
	model := firstNonEmpty(stringValue(body["modelId"]), stringValue(body["model"]), "grok-4.5")
	if sessionID == "" || effort == "" {
		writeText(responseWriter, http.StatusBadRequest, "sessionId and effort required")
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
	executionContext, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	response, errorValue := server.hub.CallRPC(executionContext, "session/set_model", map[string]any{"sessionId": sessionID, "modelId": model, "_meta": map[string]any{"reasoningEffort": effort}})
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

func loopInterval(raw any) (int, string, error) {
	if raw == nil || strings.TrimSpace(stringValue(raw)) == "" {
		return loops.ParseInterval("5m")
	}
	switch value := raw.(type) {
	case float64:
		seconds, label := loops.NormalizeInterval(int(value))
		return seconds, label, nil
	case int:
		seconds, label := loops.NormalizeInterval(value)
		return seconds, label, nil
	case json.Number:
		if count, errorValue := value.Int64(); errorValue == nil {
			seconds, label := loops.NormalizeInterval(int(count))
			return seconds, label, nil
		}
		if count, errorValue := value.Float64(); errorValue == nil {
			seconds, label := loops.NormalizeInterval(int(count))
			return seconds, label, nil
		}
	}
	return loops.ParseInterval(stringValue(raw))
}

func writeJSON(responseWriter http.ResponseWriter, status int, value any) {
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	responseWriter.WriteHeader(status)
	encoder := json.NewEncoder(responseWriter)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeText(responseWriter http.ResponseWriter, status int, value string) {
	responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.WriteHeader(status)
	_, _ = io.WriteString(responseWriter, value)
}

func writeAPIError(responseWriter http.ResponseWriter, status int, errorValue error) {
	writeJSON(responseWriter, status, map[string]any{"ok": false, "error": errorValue.Error()})
}

func decodeJSON(responseWriter http.ResponseWriter, request *http.Request, target any, allowEmpty bool) error {
	decoder := json.NewDecoder(http.MaxBytesReader(responseWriter, request.Body, maxRequestBody))
	decoder.UseNumber()
	errorValue := decoder.Decode(target)
	if errors.Is(errorValue, io.EOF) && allowEmpty {
		return nil
	}
	if errorValue != nil {
		writeText(responseWriter, http.StatusBadRequest, "json required")
		return errorValue
	}
	return nil
}

func decodeLooseJSON(request *http.Request, target any) {
	if request.Body == nil {
		return
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRequestBody))
	decoder.UseNumber()
	_ = decoder.Decode(target)
}

func writeFSError(responseWriter http.ResponseWriter, errorValue error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(errorValue, fsapi.OutsideWorkspaceError):
		status = http.StatusForbidden
	case errors.Is(errorValue, fsapi.NotFileError), errors.Is(errorValue, os.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(errorValue, fsapi.FileTooLargeError), errors.Is(errorValue, fsapi.ContentTooLargeError):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(errorValue, fsapi.PathRequiredError), errors.Is(errorValue, fsapi.NotDirectoryError), errors.Is(errorValue, fsapi.ContentRequiredError):
		status = http.StatusBadRequest
	}
	writeText(responseWriter, status, errorValue.Error())
}

func requestLoopback(request *http.Request) bool {
	host, _, errorValue := net.SplitHostPort(request.RemoteAddr)
	if errorValue != nil {
		host = request.RemoteAddr
	}
	ipAddress := net.ParseIP(strings.Trim(host, "[]"))
	return ipAddress != nil && ipAddress.IsLoopback()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return parseBool(typed)
	case float64:
		return typed != 0
	case json.Number:
		count, _ := typed.Int64()
		return count != 0
	default:
		return false
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, valid := value.(string); valid {
		return text
	}
	return fmt.Sprint(value)
}

func intQuery(value string, fallback int) int {
	parsed, errorValue := strconv.Atoi(strings.TrimSpace(value))
	if errorValue != nil {
		return fallback
	}
	return parsed
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func redactSecret(value, secret string, limit int) string {
	if secret != "" {
		value = strings.ReplaceAll(value, secret, "***")
	}
	return truncate(value, limit)
}
