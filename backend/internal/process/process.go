// Package process owns the native lifecycle for a provider agent service.
//
// This file owns the manager and its resolved configuration. Contract types
// live in model.go, ownership state in state.go, start/stop in lifecycle.go,
// launch details in spawn.go, listener classification in inspection.go, and
// secret-safe logging in redacting_writer.go.
package process

import (
	"errors"
	"strings"
	"time"
)

const (
	maximumTCPPort             = 65535
	secretHashPrefixCharacters = 16
)

var (
	ForeignListenerError        = errors.New("agent port is held by a non-owned process")
	ExecutableRequiredError     = errors.New("provider executable required")
	ProcessIdentityChangedError = errors.New("process identity changed")
)

// Manager manages exactly one agent serve port and refuses to kill foreign listeners.
type Manager struct {
	Config     Config
	Operations Operations
}

func (manager *Manager) configuration() (Config, error) {
	configuration := manager.Config
	if configuration.Port < 1 || configuration.Port > maximumTCPPort {
		return configuration, errors.New("agent port must be between 1 and 65535")
	}
	if strings.TrimSpace(configuration.BindHost) == "" || strings.TrimSpace(configuration.Secret) == "" || strings.TrimSpace(configuration.RuntimeDirectory) == "" || strings.TrimSpace(configuration.LogDirectory) == "" || strings.TrimSpace(configuration.StatePath) == "" {
		return configuration, errors.New("agent configuration requires bind host, secret, runtime, log, and state paths")
	}
	if errorValue := configuration.LifecyclePolicy.Validate(); errorValue != nil {
		return configuration, errorValue
	}
	return configuration, nil
}

func (manager *Manager) operationsWithDefaults() Operations {
	operations := manager.Operations
	if operations.ListenProcessIDs == nil {
		operations.ListenProcessIDs = ListenProcessIDsPort
	}
	if operations.CommandLine == nil {
		operations.CommandLine = CommandLine
	}
	if operations.ProcessAlive == nil {
		operations.ProcessAlive = ProcessAlive
	}
	if operations.ProcessStart == nil {
		operations.ProcessStart = ProcessStart
	}
	if operations.StartProcess == nil {
		operations.StartProcess = StartProcess
	}
	if operations.KillProcess == nil {
		operations.KillProcess = KillProcess
	}
	if operations.Now == nil {
		operations.Now = time.Now
	}
	return operations
}
