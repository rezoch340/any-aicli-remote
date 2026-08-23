// Start and stop for the one managed agent. Every termination path verifies
// operating-system identity before signalling.

package process

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (manager *Manager) cleanupSpawnedProcess(operations Operations, policy LifecyclePolicy, identity ProcessIdentity, primaryError error) error {
	cleanupError := operations.KillProcess(identity, policy)
	if cleanupError == nil {
		return primaryError
	}
	return errors.Join(primaryError, fmt.Errorf("clean up spawned agent: %w", cleanupError))
}

// Start starts the owned agent if missing. With force=true it restarts only the owned PID.
func (manager *Manager) Start(force bool) (StartResult, error) {
	configuration, errorValue := manager.configuration()
	if errorValue != nil {
		return StartResult{OK: false, Message: errorValue.Error()}, errorValue
	}
	operations := manager.operationsWithDefaults()
	status := manager.Status()
	if status.Error != "" {
		return StartResult{OK: false, Message: status.Error, Status: status}, errors.New(status.Error)
	}
	if !force && len(status.OwnedProcessIDs) > 0 {
		return StartResult{OK: true, Message: fmt.Sprintf("agent already running on :%d", configuration.Port), ProcessID: status.OwnedProcessIDs[0], Status: status}, nil
	}
	if force && len(status.OwnedProcessIDs) > 0 {
		for _, processID := range status.OwnedProcessIDs {
			identity, identityError := processIdentityForOwnedProcess(status, configuration, processID)
			if identityError != nil {
				return StartResult{OK: false, Message: identityError.Error(), Status: status}, identityError
			}
			if errorValue := operations.KillProcess(identity, configuration.LifecyclePolicy); errorValue != nil {
				return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
			}
		}
		waitUntil(configuration.LifecyclePolicy.RestartWait, configuration.LifecyclePolicy.RestartPoll, func() bool {
			value := manager.Status()
			return len(value.OwnedProcessIDs) == 0
		})
		status = manager.Status()
		if status.Error != "" {
			return StartResult{OK: false, Message: status.Error, Status: status}, errors.New(status.Error)
		}
		if len(status.OwnedProcessIDs) > 0 {
			errorValue := fmt.Errorf("owned agent pid(s) did not stop: %v", status.OwnedProcessIDs)
			return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
		}
	}
	if len(status.ForeignProcessIDs) > 0 {
		return StartResult{OK: false, Message: fmt.Sprintf("port :%d is occupied by foreign pid(s): %v", configuration.Port, status.ForeignProcessIDs), Status: status}, ForeignListenerError
	}

	executablePath := strings.TrimSpace(configuration.ExecutablePath)
	if executablePath == "" {
		return StartResult{OK: false, Message: ExecutableRequiredError.Error(), Status: status}, ExecutableRequiredError
	}
	arguments := append([]string(nil), configuration.Arguments...)
	if errorValue := os.MkdirAll(configuration.LogDirectory, 0o700); errorValue != nil {
		return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
	}
	if errorValue := os.MkdirAll(configuration.RuntimeDirectory, 0o700); errorValue != nil {
		return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
	}
	environment := mergeEnvironment(os.Environ(), configuration.Environment)
	specification := StartSpecification{
		Path: executablePath, Arguments: arguments, WorkingDirectory: configuration.RuntimeDirectory,
		LogPath: filepath.Join(configuration.LogDirectory, "provider-agent.spawn.log"), Environment: environment,
		SensitiveValues: []string{configuration.Secret},
	}
	processID, errorValue := operations.StartProcess(specification)
	if errorValue != nil {
		return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
	}
	processStartStamp, processStartError := operations.ProcessStart(processID)
	if processStartError != nil || strings.TrimSpace(processStartStamp) == "" {
		if processStartError == nil {
			processStartError = errors.New("could not identify spawned agent process")
		}
		identity := ProcessIdentity{ProcessID: processID, ExecutablePath: executablePath, IdentityTokens: append([]string(nil), configuration.IdentityTokens...)}
		combinedError := manager.cleanupSpawnedProcess(operations, configuration.LifecyclePolicy, identity, processStartError)
		return StartResult{OK: false, Message: combinedError.Error(), Status: status}, combinedError
	}
	state := State{ProcessID: processID, Port: configuration.Port, BindHost: configuration.BindHost, RuntimeDirectory: configuration.RuntimeDirectory, ExecutablePath: executablePath, Arguments: redactArguments(arguments, configuration.Secret), IdentityTokens: append([]string(nil), configuration.IdentityTokens...), SecretHash: secretHash(configuration.Secret), StartedAt: operations.Now().UTC().Format(time.RFC3339Nano), ProcessStart: strings.TrimSpace(processStartStamp)}
	if errorValue := manager.saveState(state); errorValue != nil {
		combinedError := manager.cleanupSpawnedProcess(operations, configuration.LifecyclePolicy, processIdentityFromState(&state, configuration), errorValue)
		return StartResult{OK: false, Message: combinedError.Error(), Status: status}, combinedError
	}
	return StartResult{OK: true, Message: fmt.Sprintf("agent started on :%d", configuration.Port), Started: true, ProcessID: processID, Status: manager.Status()}, nil
}

// Stop terminates only the PID proven to be owned by this daemon state file.
func (manager *Manager) Stop() (StopResult, error) {
	configuration, configurationError := manager.configuration()
	if configurationError != nil {
		return StopResult{OK: false, Message: configurationError.Error()}, configurationError
	}
	status := manager.Status()
	if status.Error != "" {
		return StopResult{OK: false, Message: status.Error, Status: status}, errors.New(status.Error)
	}
	if len(status.OwnedProcessIDs) == 0 {
		if len(status.ForeignProcessIDs) > 0 {
			return StopResult{OK: false, Message: "no owned agent to stop; foreign listener left untouched", Status: status}, ForeignListenerError
		}
		manager.removeState()
		return StopResult{OK: true, Message: "agent already stopped", Status: status}, nil
	}
	operations := manager.operationsWithDefaults()
	killed := []int{}
	for _, processID := range status.OwnedProcessIDs {
		identity, identityError := processIdentityForOwnedProcess(status, configuration, processID)
		if identityError != nil {
			return StopResult{OK: false, Message: identityError.Error(), Killed: killed, Status: status}, identityError
		}
		if errorValue := operations.KillProcess(identity, configuration.LifecyclePolicy); errorValue != nil {
			return StopResult{OK: false, Message: errorValue.Error(), Killed: killed, Status: status}, errorValue
		}
		killed = append(killed, processID)
	}
	manager.removeState()
	return StopResult{OK: true, Message: "owned agent stopped", Killed: killed, Status: manager.Status()}, nil
}

func waitUntil(timeout, step time.Duration, predicate func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return true
		}
		time.Sleep(step)
	}
	return predicate()
}
