package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	processapi "github.com/rezoch340/any-aicli-remote/backend/internal/process"
)

func (server *Server) handleStackStatus(responseWriter http.ResponseWriter, _ *http.Request) {
	daemonProcessIDs, _ := processapi.ListenProcessIDsPort(server.configuration.Port, true)
	status := server.processStatus()
	writeJSON(responseWriter, http.StatusOK, map[string]any{
		"ok": true, "daemon_port": server.configuration.Port, "agent_port": server.configuration.AgentPort,
		"daemon_pids": daemonProcessIDs, "agent_pids": status.ProcessIDs, "self_pid": os.Getpid(),
		"lan": server.lanIP, "provider_id": server.configuration.ProviderID, "hub_up": server.hub.AgentConnected(),
		"hub_err": server.hub.LastError(), "agent_listening": status.Listening,
	})
}

func (server *Server) handleStackStop(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		KeepAgent bool `json:"keep_agent"`
	}
	if !server.decodeLooseJSON(responseWriter, request, &body) {
		return
	}
	status := server.processStatus()
	agentProcessIDs := status.OwnedProcessIDs
	if body.KeepAgent {
		agentProcessIDs = []int{}
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "stopping": true, "self_pid": os.Getpid(), "agent_pids": agentProcessIDs, "message": "daemon stopping; provider agent will stop unless keep_agent"})
	go func() {
		time.Sleep(server.configuration.Canonical.Tuning.Lifecycle.StackSettle.Duration)
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
	if !server.decodeLooseJSON(responseWriter, request, &body) {
		return
	}
	force := boolValue(body["force"]) || boolValue(body["restart"])

	server.agentLifecycleMutex.Lock()
	if server.closing.Load() {
		server.agentLifecycleMutex.Unlock()
		writeJSON(responseWriter, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "server is stopping", "started": false})
		return
	}
	server.processMutex.Lock()
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
		if startError = server.configureProcessCommandLocked(); startError == nil {
			startResult, startError = server.process.Start(force)
		}
		needWait = startError == nil
	}
	server.processMutex.Unlock()
	listening := status.Listening
	if needWait {
		waitContext, waitCancel := context.WithTimeout(request.Context(), server.configuration.Canonical.Tuning.Lifecycle.StackWait.Duration)
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
	executionContext, cancel := context.WithTimeout(request.Context(), server.configuration.Canonical.Tuning.Lifecycle.StartTimeout.Duration)
	errorValue := server.hub.Ensure(executionContext)
	cancel()
	status = server.processStatus()
	if errorValue != nil && status.Owned {
		restartContext, restartCancel := context.WithTimeout(request.Context(), server.configuration.Canonical.Tuning.Lifecycle.StackWait.Duration)
		restartAttempt, restartError := server.restartOwnedAgentForAuthentication(restartContext)
		restartCancel()
		if restartAttempt.attempted {
			attempts = append(attempts, map[string]any{"reason": "hub-auth-retry", "listen": restartAttempt.listening, "killed": []int{}})
		}
		if restartError == nil && restartAttempt.listening {
			started = started || restartAttempt.result.Started
			executionContext, cancel = context.WithTimeout(request.Context(), server.configuration.Canonical.Tuning.Lifecycle.RestartTimeout.Duration)
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
