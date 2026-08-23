// Package server assembles the Any AI CLI Remote HTTP/WebSocket daemon.
//
// This file owns construction and the service graph. Serving and shutdown live
// in lifecycle.go, provider agent supervision in agentprocess.go, host probes
// in network.go, and the HTTP surface in routes.go and the *_routes.go files.
package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

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
