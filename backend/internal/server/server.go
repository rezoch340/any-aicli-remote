// Package server assembles the native Grok Remote HTTP/WebSocket daemon.
package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grok-remote/grok-remote-app/backend/internal/config"
	"github.com/grok-remote/grok-remote-app/backend/internal/fsapi"
	"github.com/grok-remote/grok-remote-app/backend/internal/gitapi"
	"github.com/grok-remote/grok-remote-app/backend/internal/history"
	"github.com/grok-remote/grok-remote-app/backend/internal/hub"
	"github.com/grok-remote/grok-remote-app/backend/internal/loops"
	processapi "github.com/grok-remote/grok-remote-app/backend/internal/process"
	"github.com/grok-remote/grok-remote-app/backend/internal/room"
	"github.com/grok-remote/grok-remote-app/backend/internal/sessionapi"
	"github.com/grok-remote/grok-remote-app/backend/internal/voice"
)

var AlreadyRunningError = errors.New("grok remote is already running on this port")

type stopRequest struct {
	keepAgent bool
}

type agentRestartAttempt struct {
	result    processapi.StartResult
	listening bool
	attempted bool
}

// Server owns the complete Go replacement for server.py. Its Handler can also
// be mounted in tests without starting a listener.
type Server struct {
	configuration config.Config
	logger        *slog.Logger
	lanIP         string

	filesystem *fsapi.Service
	git        *gitapi.Service
	history    *history.Store
	hub        *hub.Hub
	process    *processapi.Manager
	loops      *loops.Manager
	room       *room.Store
	session    *sessionapi.Service
	voice      *voice.Service

	processMutex        sync.Mutex
	agentLifecycleMutex sync.Mutex
	stackStartMutex     sync.Mutex
	httpMutex           sync.Mutex
	http                *http.Server
	stopChannel         chan stopRequest
	closing             atomic.Bool

	closeOnce sync.Once
}

// New wires all persistent services but does not bind a port or start a child
// process. That makes construction side-effect-light and keeps Handler usable
// with httptest.
func New(raw config.Config, logger *slog.Logger) (*Server, error) {
	configuration, errorValue := normalizeConfig(raw)
	if errorValue != nil {
		return nil, errorValue
	}
	if logger == nil {
		logger = slog.Default()
	}
	if errorValue := os.MkdirAll(configuration.DataDirectory, 0o755); errorValue != nil {
		return nil, fmt.Errorf("create data directory: %w", errorValue)
	}
	filesystem, errorValue := fsapi.New(configuration.WorkingDirectory)
	if errorValue != nil {
		return nil, fmt.Errorf("open workspace: %w", errorValue)
	}

	server := &Server{
		configuration: configuration,
		logger:        logger,
		lanIP:         discoverLANIP(),
		filesystem:    filesystem,
		history:       history.NewStore(configuration.SessionsDirectory),
		room:          room.New(configuration.DataDirectory),
		voice:         voice.NewFromEnvironment(),
		stopChannel:   make(chan stopRequest, 1),
	}
	server.git = gitapi.New(server.filesystem.Root)
	server.session = sessionapi.New(server.history, configuration.DataDirectory, server.filesystem.Root)
	server.process = &processapi.Manager{Config: processapi.Config{
		Port:             configuration.AgentPort,
		BindHost:         configuration.AgentHost,
		Secret:           configuration.Secret,
		WorkingDirectory: configuration.WorkingDirectory,
		GrokPath:         configuration.GrokPath,
		LogDirectory:     filepath.Join(configuration.DataDirectory, "logs"),
		StatePath:        filepath.Join(configuration.DataDirectory, "agent-state.json"),
		AlwaysApprove:    configuration.AlwaysApprove,
		UseLeader:        configuration.Leader,
	}}

	var ensure hub.EnsureAgentFunc
	if configuration.EnsureAgent && isLoopbackHost(configuration.AgentHost) {
		ensure = server.ensureAgentProcess
	}
	server.hub = hub.New(configuration.AgentWebSocketURL(), server.filesystem.Root, ensure, logger)
	server.loops, errorValue = loops.New(filepath.Join(configuration.DataDirectory, "loops.json"), server.fireLoop)
	if errorValue != nil {
		_ = filesystem.Close()
		return nil, fmt.Errorf("open remote loops: %w", errorValue)
	}
	_ = server.writeRuntimeConfig()
	return server, nil
}

func normalizeConfig(configuration config.Config) (config.Config, error) {
	if strings.TrimSpace(configuration.Bind) == "" {
		configuration.Bind = "0.0.0.0"
	}
	if configuration.Port == 0 {
		configuration.Port = config.DefaultPort
	}
	if strings.TrimSpace(configuration.AgentHost) == "" {
		configuration.AgentHost = "127.0.0.1"
	}
	if configuration.AgentPort == 0 {
		configuration.AgentPort = config.DefaultAgentPort
	}
	if configuration.Port < 1 || configuration.Port > 65535 || configuration.AgentPort < 1 || configuration.AgentPort > 65535 {
		return configuration, errors.New("ports must be between 1 and 65535")
	}
	if configuration.Port == configuration.AgentPort {
		return configuration, errors.New("HTTP and agent ports must differ")
	}
	if strings.TrimSpace(configuration.WorkingDirectory) == "" {
		workingDirectory, errorValue := os.Getwd()
		if errorValue != nil {
			return configuration, errorValue
		}
		configuration.WorkingDirectory = workingDirectory
	}
	absolutePath, errorValue := filepath.Abs(configuration.WorkingDirectory)
	if errorValue != nil {
		return configuration, errorValue
	}
	if info, statusError := os.Stat(absolutePath); statusError != nil || !info.IsDir() {
		return configuration, fmt.Errorf("workspace is not a directory: %s", absolutePath)
	}
	configuration.WorkingDirectory = absolutePath
	home, _ := os.UserHomeDir()
	if strings.TrimSpace(configuration.DataDirectory) == "" {
		configuration.DataDirectory = filepath.Join(home, ".grok", "plugin-data", "grok-remote")
	}
	if strings.TrimSpace(configuration.SessionsDirectory) == "" {
		configuration.SessionsDirectory = filepath.Join(home, ".grok", "sessions")
	}
	return configuration, nil
}

func (server *Server) fireLoop(executionContext context.Context, job loops.Job, note string) error {
	response, errorValue := server.hub.CallRPC(executionContext, "session/prompt", map[string]any{
		"sessionId": job.SessionID,
		"prompt":    []map[string]any{{"type": "text", "text": note}},
	})
	if errorValue != nil {
		return errorValue
	}
	if rpcError := response["error"]; rpcError != nil {
		return fmt.Errorf("session/prompt: %v", rpcError)
	}
	server.hub.Notify("_x.ai/remote/loop_fire", map[string]any{
		"id": job.ID, "sessionId": job.SessionID, "fires": job.Fires + 1, "interval": job.IntervalLabel,
	})
	return nil
}

// Handler returns all compatibility routes behind the pairing-key middleware.
func (server *Server) Handler() http.Handler {
	return authMiddleware(server.configuration.Secret, server.lanIP, server.configuration.Port, server.routes())
}

// Run starts the hub, loop scheduler, and HTTP listener and blocks until the
// context is cancelled, /api/stack/stop is called, or serving fails.
func (server *Server) Run(executionContext context.Context) error {
	if executionContext == nil {
		executionContext = context.Background()
	}
	listener, errorValue := net.Listen("tcp", net.JoinHostPort(server.configuration.Bind, fmt.Sprint(server.configuration.Port)))
	if errorValue != nil {
		if healthyRemote(server.configuration.Port) {
			return AlreadyRunningError
		}
		return errorValue
	}

	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	server.httpMutex.Lock()
	server.http = httpServer
	server.httpMutex.Unlock()

	errorChannel := make(chan error, 1)
	go func() {
		errorChannel <- httpServer.Serve(listener)
	}()
	if server.configuration.EnsureAgent {
		server.ensureAgentAtBoot(executionContext)
	}
	server.hub.Start(executionContext)
	if errorValue := server.loops.Start(executionContext); errorValue != nil {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = httpServer.Shutdown(shutdownContext)
		cancel()
		return errorValue
	}

	var runError error
	var stop stopRequest
	stoppedByAPI := false
	select {
	case <-executionContext.Done():
		runError = executionContext.Err()
	case stop = <-server.stopChannel:
		stoppedByAPI = true
	case errorValue := <-errorChannel:
		if !errors.Is(errorValue, http.ErrServerClosed) {
			runError = errorValue
		}
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	_ = httpServer.Shutdown(shutdownContext)
	cancel()
	server.Close()
	if shouldStopManagedAgent(stoppedByAPI, stop.keepAgent, server.configuration.StopAgentOnExit) {
		server.agentLifecycleMutex.Lock()
		server.processMutex.Lock()
		_, _ = server.process.Stop()
		server.processMutex.Unlock()
		server.agentLifecycleMutex.Unlock()
	}
	if errors.Is(runError, context.Canceled) || errors.Is(runError, context.DeadlineExceeded) {
		return nil
	}
	return runError
}

func shouldStopManagedAgent(stoppedByAPI, keepAgent, stopAgentOnExit bool) bool {
	if stoppedByAPI {
		return !keepAgent
	}
	return stopAgentOnExit
}

func (server *Server) ensureAgentAtBoot(parent context.Context) {
	if !isLoopbackHost(server.configuration.AgentHost) {
		return
	}
	bootContext, bootCancel := context.WithTimeout(parent, 18*time.Second)
	if errorValue := server.ensureAgentProcess(bootContext); errorValue != nil {
		server.logger.Warn("agent start failed", "error", errorValue)
	}
	bootCancel()
	executionContext, cancel := context.WithTimeout(parent, 18*time.Second)
	defer cancel()
	if errorValue := server.hub.Ensure(executionContext); errorValue != nil {
		server.logger.Warn("upstream agent unavailable", "error", errorValue)
	}
}

func (server *Server) ensureAgentProcess(executionContext context.Context) error {
	if !isLoopbackHost(server.configuration.AgentHost) {
		return nil
	}
	if errorValue := executionContext.Err(); errorValue != nil {
		return errorValue
	}
	if server.closing.Load() {
		return errors.New("server is stopping")
	}
	server.agentLifecycleMutex.Lock()
	defer server.agentLifecycleMutex.Unlock()
	if errorValue := executionContext.Err(); errorValue != nil {
		return errorValue
	}
	if server.closing.Load() {
		return errors.New("server is stopping")
	}

	server.processMutex.Lock()
	status := server.process.Status()
	if !status.Listening {
		if _, errorValue := server.process.Start(false); errorValue != nil {
			server.processMutex.Unlock()
			return errorValue
		}
	}
	server.processMutex.Unlock()
	if !server.waitForAgent(executionContext) {
		if errorValue := executionContext.Err(); errorValue != nil {
			return errorValue
		}
		return fmt.Errorf("agent did not bind :%d", server.configuration.AgentPort)
	}
	return nil
}

func (server *Server) restartOwnedAgentForAuthentication(executionContext context.Context) (agentRestartAttempt, error) {
	if errorValue := executionContext.Err(); errorValue != nil {
		return agentRestartAttempt{}, errorValue
	}
	server.agentLifecycleMutex.Lock()
	defer server.agentLifecycleMutex.Unlock()
	if errorValue := executionContext.Err(); errorValue != nil {
		return agentRestartAttempt{}, errorValue
	}
	if server.closing.Load() {
		return agentRestartAttempt{}, errors.New("server is stopping")
	}

	server.processMutex.Lock()
	status := server.process.Status()
	if !status.Owned {
		server.processMutex.Unlock()
		return agentRestartAttempt{}, errors.New("owned agent is no longer available for authentication retry")
	}
	server.hub.DisconnectAgent("hub authentication retry")
	restartResult, restartError := server.process.Start(true)
	server.processMutex.Unlock()
	attempt := agentRestartAttempt{result: restartResult, attempted: true}
	if restartError != nil {
		return attempt, restartError
	}
	attempt.listening = server.waitForAgent(executionContext)
	if !attempt.listening {
		if errorValue := executionContext.Err(); errorValue != nil {
			return attempt, errorValue
		}
		return attempt, fmt.Errorf("agent did not bind :%d after hub authentication retry", server.configuration.AgentPort)
	}
	return attempt, nil
}

func (server *Server) processStatus() processapi.Status {
	server.processMutex.Lock()
	defer server.processMutex.Unlock()
	return server.process.Status()
}

func (server *Server) waitForAgent(executionContext context.Context) bool {
	address := net.JoinHostPort(server.configuration.AgentHost, strconv.Itoa(server.configuration.AgentPort))
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		attempt, cancel := context.WithTimeout(executionContext, 500*time.Millisecond)
		connection, errorValue := (&net.Dialer{}).DialContext(attempt, "tcp", address)
		cancel()
		if errorValue == nil {
			_ = connection.Close()
			return true
		}
		select {
		case <-executionContext.Done():
			return false
		case <-ticker.C:
		}
	}
}

// Close releases in-process resources. It deliberately does not kill the Grok
// agent; explicit /api/stack/stop controls that, matching server.py shutdown.
func (server *Server) Close() {
	server.closeOnce.Do(func() {
		server.closing.Store(true)
		// Linearize shutdown with every process start/restart transaction. Once
		// this lock is observed, no later ensure path can pass the closing check.
		server.agentLifecycleMutex.Lock()
		server.agentLifecycleMutex.Unlock()
		server.loops.Close()
		server.hub.Close()
		_ = server.filesystem.Close()
	})
}

func (server *Server) requestStop(keepAgent bool) {
	select {
	case server.stopChannel <- stopRequest{keepAgent: keepAgent}:
	default:
	}
}

func healthyRemote(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	response, errorValue := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if errorValue != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	data, _ := io.ReadAll(io.LimitReader(response.Body, 512))
	return bytes.Contains(data, []byte(`"ok"`))
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ipAddress := net.ParseIP(host)
	return ipAddress != nil && ipAddress.IsLoopback()
}

func discoverLANIP() string {
	interfaces, _ := net.Interfaces()
	first := ""
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := networkInterface.Addrs()
		for _, address := range addresses {
			ipAddress, _, errorValue := net.ParseCIDR(address.String())
			if errorValue != nil || ipAddress == nil || ipAddress.IsLoopback() || ipAddress.To4() == nil {
				continue
			}
			if first == "" {
				first = ipAddress.String()
			}
			if ipAddress.IsPrivate() {
				return ipAddress.String()
			}
		}
	}
	if first != "" {
		return first
	}
	return "127.0.0.1"
}
