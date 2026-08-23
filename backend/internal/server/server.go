// Package server assembles the Any AI CLI Remote HTTP/WebSocket daemon.
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/gitapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/hub"
	"github.com/rezoch340/any-aicli-remote/backend/internal/loops"
	processapi "github.com/rezoch340/any-aicli-remote/backend/internal/process"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	providerfactory "github.com/rezoch340/any-aicli-remote/backend/internal/provider/factory"
	"github.com/rezoch340/any-aicli-remote/backend/internal/room"
	"github.com/rezoch340/any-aicli-remote/backend/internal/sessionapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/skills"
	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

var AlreadyRunningError = errors.New("Any AI CLI Remote is already running on this port")

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
	configuration    config.Config
	logger           *slog.Logger
	lanIP            string
	filesystemPolicy fsapi.Policy
	gitPolicy        gitapi.Policy
	voicePolicy      voice.Policy
	skillsPolicy     skills.Policy

	providers       *providerapi.Registry
	providerCatalog providerapi.Provider
	protocol        providerapi.ProtocolAdapter
	hub             *hub.Hub
	process         *processapi.Manager
	loops           *loops.Manager
	room            *room.Store
	session         *sessionapi.Service
	voice           voice.Service
	skillRoots      providerapi.SkillRootProvider

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
	if errorValue := os.MkdirAll(configuration.DataDirectory, 0o700); errorValue != nil {
		return nil, fmt.Errorf("create data directory: %w", errorValue)
	}
	if operationError := os.MkdirAll(configuration.RuntimeDirectory, 0o700); operationError != nil {
		return nil, fmt.Errorf("create runtime directory: %w", operationError)
	}

	effectiveHistoryPolicy := historyPolicy(configuration.Canonical.Tuning.History)
	effectiveFilesystemPolicy := filesystemPolicy(configuration.Canonical.Tuning.Filesystem)
	effectiveGitPolicy := gitPolicy(configuration.Canonical.Tuning.Git)
	effectiveRoomPolicy := roomPolicy(configuration.Canonical.Tuning.Room)
	effectiveVoicePolicy := voicePolicy(configuration.Canonical.Tuning.Voice)
	effectiveSkillsPolicy := skillsPolicy(configuration.Canonical.Tuning.Skills)
	if errorValue := effectiveFilesystemPolicy.Validate(); errorValue != nil {
		return nil, errorValue
	}
	if errorValue := effectiveSkillsPolicy.Validate(); errorValue != nil {
		return nil, errorValue
	}
	if errorValue := effectiveVoicePolicy.Validate(); errorValue != nil {
		return nil, errorValue
	}
	if errorValue := effectiveGitPolicy.Validate(); errorValue != nil {
		return nil, errorValue
	}
	roomStore, errorValue := room.New(configuration.DataDirectory, effectiveRoomPolicy)
	if errorValue != nil {
		return nil, errorValue
	}
	providerComponents, errorValue := providerfactory.New(providerfactory.Configuration{
		ProviderID:     configuration.ProviderID,
		ExecutablePath: configuration.ProviderPath,
		Options:        configuration.ProviderOptions,
		HistoryPolicy:  effectiveHistoryPolicy,
		VoicePolicy:    effectiveVoicePolicy,
	})
	if errorValue != nil {
		return nil, errorValue
	}
	providerCatalog := providerComponents.Catalog
	protocol := providerComponents.Protocol
	providerRegistry := providerapi.NewRegistry(providerCatalog)

	server := &Server{
		configuration:    configuration,
		logger:           logger,
		lanIP:            discoverLANIP(),
		filesystemPolicy: effectiveFilesystemPolicy,
		gitPolicy:        effectiveGitPolicy,
		voicePolicy:      effectiveVoicePolicy,
		skillsPolicy:     effectiveSkillsPolicy,
		providers:        providerRegistry,
		providerCatalog:  providerCatalog,
		protocol:         protocol,
		room:             roomStore,
		voice:            providerComponents.Voice,
		skillRoots:       providerComponents.SkillRoots,
		stopChannel:      make(chan stopRequest, 1),
	}
	server.session, errorValue = sessionapi.New(providerRegistry, configuration.ProviderID, configuration.DataDirectory, effectiveHistoryPolicy)
	if errorValue != nil {
		return nil, errorValue
	}
	server.process = &processapi.Manager{Config: processapi.Config{
		Port: configuration.AgentPort, BindHost: configuration.AgentHost, Secret: configuration.AgentSecret,
		RuntimeDirectory: configuration.RuntimeDirectory,
		LogDirectory:     logsDirectory(configuration.DataDirectory),
		StatePath:        agentStatePath(configuration.DataDirectory),
		LifecyclePolicy:  processLifecyclePolicy(configuration.Canonical.Tuning.Lifecycle),
	}}

	var ensure hub.EnsureAgentFunc
	if configuration.EnsureAgent && isLoopbackHost(configuration.AgentHost) {
		ensure = server.ensureAgentProcess
	}
	agentURL := protocol.AgentWebSocketURL(configuration.AgentHost, configuration.AgentPort, configuration.AgentSecret).String()
	server.hub, errorValue = hub.New(agentURL, providerCatalog, protocol, ensure, hubPolicy(configuration.Canonical.Tuning), logger)
	if errorValue != nil {
		return nil, errorValue
	}
	server.loops, errorValue = loops.New(loopsStorePath(configuration.DataDirectory), server.fireLoop, loopsPolicy(configuration.Canonical.Tuning.Loops))
	if errorValue != nil {
		return nil, fmt.Errorf("open remote loops: %w", errorValue)
	}
	if errorValue := server.writeRuntimeConfig(); errorValue != nil {
		server.Close()
		return nil, fmt.Errorf("write runtime config: %w", errorValue)
	}
	return server, nil
}

func normalizeConfig(configuration config.Config) (config.Config, error) {
	if configuration.Canonical.Version == 0 {
		return configuration, errors.New("canonical config is required")
	}
	if operationError := config.ValidateDocument(configuration.Canonical); operationError != nil {
		return configuration, fmt.Errorf("validate canonical config: %w", operationError)
	}
	return config.ApplyCanonical(configuration, configuration.Canonical), nil
}

func (server *Server) fireLoop(executionContext context.Context, job loops.Job, note string) error {
	method, params := server.protocol.TextPromptRequest(job.SessionID, note)
	response, errorValue := server.hub.CallRPC(executionContext, method, params)
	if errorValue != nil {
		return errorValue
	}
	if rpcError := response["error"]; rpcError != nil {
		return fmt.Errorf("provider prompt: %v", rpcError)
	}
	method, params = server.protocol.DaemonNotification(providerapi.LoopFiredNotification, map[string]any{
		"id": job.ID, "sessionId": job.SessionID, "fires": job.Fires + 1, "interval": job.IntervalLabel,
	})
	server.hub.Notify(method, params)
	return nil
}

// Handler returns all compatibility routes behind the pairing-key middleware.
func (server *Server) Handler() http.Handler {
	return authMiddleware(server.configuration.PairingSecret, cookieMaxAgeSeconds(server.configuration.Canonical.Tuning.HTTP), server.routes())
}

// Run starts the hub, loop scheduler, and HTTP listener and blocks until the
// context is cancelled, /api/stack/stop is called, or serving fails.
func (server *Server) Run(executionContext context.Context) error {
	if executionContext == nil {
		executionContext = context.Background()
	}
	listener, errorValue := net.Listen("tcp", net.JoinHostPort(server.configuration.Bind, fmt.Sprint(server.configuration.Port)))
	if errorValue != nil {
		if healthyRemote(server.configuration.Port, server.configuration.Canonical.Tuning.HTTP.ExistingDaemonProbeTimeout.Duration, server.configuration.Canonical.Tuning.HTTP.HealthProbeMaxBytes) {
			return AlreadyRunningError
		}
		return errorValue
	}

	httpServer := newHTTPServer(server.Handler(), server.configuration.Canonical.Tuning.HTTP)
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
		shutdownContext, cancel := context.WithTimeout(context.Background(), server.configuration.Canonical.Tuning.HTTP.StartupFailureShutdownTimeout.Duration)
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

	shutdownContext, cancel := context.WithTimeout(context.Background(), server.configuration.Canonical.Tuning.HTTP.ShutdownTimeout.Duration)
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
	bootContext, bootCancel := context.WithTimeout(parent, server.configuration.Canonical.Tuning.Lifecycle.BootAgentTimeout.Duration)
	if errorValue := server.ensureAgentProcess(bootContext); errorValue != nil {
		server.logger.Warn("agent start failed", "error", errorValue)
	}
	bootCancel()
	executionContext, cancel := context.WithTimeout(parent, server.configuration.Canonical.Tuning.Lifecycle.HubEnsureTimeout.Duration)
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
		if errorValue := server.configureProcessCommandLocked(); errorValue != nil {
			server.processMutex.Unlock()
			return errorValue
		}
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
	if commandError := server.configureProcessCommandLocked(); commandError != nil {
		server.processMutex.Unlock()
		return agentRestartAttempt{}, commandError
	}
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

// configureProcessCommandLocked resolves provider-specific launch details at
// start time. The caller must hold processMutex.
func (server *Server) configureProcessCommandLocked() error {
	command, operationError := server.protocol.AgentCommand(providerapi.AgentLaunchConfiguration{
		Host: server.configuration.AgentHost, Port: server.configuration.AgentPort,
		Secret: server.configuration.AgentSecret, RuntimeDirectory: server.configuration.RuntimeDirectory,
	})
	if operationError != nil {
		return operationError
	}
	server.process.Config.ExecutablePath = command.ExecutablePath
	server.process.Config.Arguments = append([]string(nil), command.Arguments...)
	server.process.Config.Environment = append([]string(nil), command.Environment...)
	server.process.Config.IdentityTokens = append([]string(nil), command.IdentityTokens...)
	return nil
}

func (server *Server) waitForAgent(executionContext context.Context) bool {
	address := net.JoinHostPort(server.configuration.AgentHost, strconv.Itoa(server.configuration.AgentPort))
	ticker := time.NewTicker(server.configuration.Canonical.Tuning.Lifecycle.ListenerPoll.Duration)
	defer ticker.Stop()
	for {
		attempt, cancel := context.WithTimeout(executionContext, server.configuration.Canonical.Tuning.Lifecycle.DialTimeout.Duration)
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

// Close releases in-process resources. It deliberately does not kill the
// provider agent; explicit /api/stack/stop controls that.
func (server *Server) Close() {
	server.closeOnce.Do(func() {
		server.closing.Store(true)
		// Linearize shutdown with every process start/restart transaction. Once
		// this lock is observed, no later ensure path can pass the closing check.
		server.agentLifecycleMutex.Lock()
		server.agentLifecycleMutex.Unlock()
		server.loops.Close()
		server.hub.Close()
	})
}

func (server *Server) requestStop(keepAgent bool) {
	select {
	case server.stopChannel <- stopRequest{keepAgent: keepAgent}:
	default:
	}
}

func healthyRemote(port int, timeout time.Duration, maxBytes int64) bool {
	client := &http.Client{Timeout: timeout}
	response, errorValue := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if errorValue != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	data, _ := io.ReadAll(io.LimitReader(response.Body, maxBytes))
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
