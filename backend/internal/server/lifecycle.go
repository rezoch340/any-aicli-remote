// Daemon lifecycle: serving, the loop scheduler callback, and shutdown.

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/rezoch340/any-aicli-remote/backend/internal/loops"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type stopRequest struct {
	keepAgent bool
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
