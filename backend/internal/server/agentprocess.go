// Provider agent process supervision: boot, ensure, authenticated restart, and
// launch-command resolution. Starting the daemon may start an idle provider
// service but never creates, loads, or resumes a session.

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	processapi "github.com/rezoch340/any-aicli-remote/backend/internal/process"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type agentRestartAttempt struct {
	result    processapi.StartResult
	listening bool
	attempted bool
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
