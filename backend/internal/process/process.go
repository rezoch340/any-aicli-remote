// Package process owns the native lifecycle for a provider agent service.
package process

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
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

// LifecyclePolicy controls managed provider process termination and restart timing.
// It is supplied by the composition root so process lifecycle behavior has no
// hidden runtime defaults.
type LifecyclePolicy struct {
	KillGrace     time.Duration
	RestartWait   time.Duration
	RestartPoll   time.Duration
	PostKillDelay time.Duration
	StopWait      time.Duration
	StopPoll      time.Duration
}

func (policy LifecyclePolicy) Validate() error {
	if policy.KillGrace <= 0 || policy.RestartWait <= 0 || policy.RestartPoll <= 0 || policy.PostKillDelay <= 0 || policy.StopWait <= 0 || policy.StopPoll <= 0 {
		return errors.New("process lifecycle policy durations must be positive")
	}
	return nil
}

// Config describes the one provider agent instance managed by this daemon.
type Config struct {
	Port             int
	BindHost         string
	Secret           string
	RuntimeDirectory string
	ExecutablePath   string
	Arguments        []string
	Environment      []string
	IdentityTokens   []string
	LogDirectory     string
	StatePath        string
	LifecyclePolicy  LifecyclePolicy
}

// State is persisted so stop/restart can distinguish the managed provider from
// unrelated processes on the same port.
type State struct {
	ProcessID        int      `json:"pid"`
	Port             int      `json:"port"`
	BindHost         string   `json:"bindHost"`
	RuntimeDirectory string   `json:"runtimeDirectory"`
	ExecutablePath   string   `json:"executablePath"`
	Arguments        []string `json:"args"`
	IdentityTokens   []string `json:"identityTokens"`
	SecretHash       string   `json:"secretHash,omitempty"`
	StartedAt        string   `json:"startedAt"`
	ProcessStart     string   `json:"processStart,omitempty"`
}

// Status classifies listeners without killing anything.
type Status struct {
	Running           bool   `json:"running"`
	Listening         bool   `json:"listening"`
	Owned             bool   `json:"owned"`
	Port              int    `json:"port"`
	BindHost          string `json:"bindHost"`
	ProcessIDs        []int  `json:"pids"`
	OwnedProcessIDs   []int  `json:"ownedPids"`
	ForeignProcessIDs []int  `json:"foreignPids"`
	State             *State `json:"state,omitempty"`
	Error             string `json:"error,omitempty"`
}

// StartResult is returned by Start.
type StartResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	Started   bool   `json:"started"`
	ProcessID int    `json:"pid,omitempty"`
	Status    Status `json:"status"`
}

// StopResult is returned by Stop.
type StopResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Killed  []int  `json:"killed"`
	Status  Status `json:"status"`
}

// StartSpecification is passed to Operations.StartProcess.
type StartSpecification struct {
	Path             string
	Arguments        []string
	Environment      []string
	SensitiveValues  []string
	WorkingDirectory string
	LogPath          string
}

// ProcessIdentity is the immutable operating-system identity required before a
// managed process may be signalled. ExecutablePath also supports validating a
// freshly spawned process when its start stamp could not be read.
type ProcessIdentity struct {
	ProcessID      int
	ProcessStart   string
	ExecutablePath string
	IdentityTokens []string
}

// Operations allows unit tests to fake operating-system process state.
type Operations struct {
	ListenProcessIDs func(port int, excludeSelf bool) ([]int, error)
	CommandLine      func(processID int) (string, error)
	ProcessAlive     func(processID int) bool
	ProcessStart     func(processID int) (string, error)
	StartProcess     func(StartSpecification) (int, error)
	KillProcess      func(identity ProcessIdentity, policy LifecyclePolicy) error
	Now              func() time.Time
}

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

func secretHash(secret string) string {
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])[:secretHashPrefixCharacters]
}

func (manager *Manager) LoadState() (*State, error) {
	configuration, errorValue := manager.configuration()
	if errorValue != nil {
		return nil, errorValue
	}
	rawData, errorValue := os.ReadFile(configuration.StatePath)
	if errors.Is(errorValue, os.ErrNotExist) {
		return nil, nil
	}
	if errorValue != nil {
		return nil, errorValue
	}
	var state State
	if errorValue := json.Unmarshal(rawData, &state); errorValue != nil {
		return nil, errorValue
	}
	return &state, nil
}

func (manager *Manager) saveState(state State) error {
	configuration, errorValue := manager.configuration()
	if errorValue != nil {
		return errorValue
	}
	rawData, errorValue := json.MarshalIndent(state, "", "  ")
	if errorValue != nil {
		return errorValue
	}
	return atomicfile.WritePrivate(configuration.StatePath, append(rawData, '\n'))
}

func (manager *Manager) removeState() {
	configuration, errorValue := manager.configuration()
	if errorValue == nil {
		_ = os.Remove(configuration.StatePath)
	}
}

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

func mergeEnvironment(baseEnvironment, overrides []string) []string {
	overriddenKeys := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		key, _, present := strings.Cut(entry, "=")
		if present {
			overriddenKeys[normalizedEnvironmentKey(key)] = struct{}{}
		}
	}
	merged := make([]string, 0, len(baseEnvironment)+len(overrides))
	for _, entry := range baseEnvironment {
		key, _, present := strings.Cut(entry, "=")
		if present {
			if _, overridden := overriddenKeys[normalizedEnvironmentKey(key)]; overridden {
				continue
			}
		}
		merged = append(merged, entry)
	}
	return append(merged, overrides...)
}

func normalizedEnvironmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func redactArguments(arguments []string, secret string) []string {
	output := append([]string(nil), arguments...)
	if secret == "" {
		return output
	}
	for argumentIndex, argument := range output {
		if argument == secret {
			output[argumentIndex] = "***"
		} else if strings.Contains(argument, secret) {
			output[argumentIndex] = strings.ReplaceAll(argument, secret, "***")
		}
	}
	return output
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

// StartProcess starts the process detached enough for a macOS menu-bar/daemon parent.
func StartProcess(specification StartSpecification) (int, error) {
	logPath := specification.LogPath
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "provider-agent.spawn.log")
	}
	if errorValue := os.MkdirAll(filepath.Dir(logPath), 0o700); errorValue != nil {
		return 0, errorValue
	}
	logFile, errorValue := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if errorValue != nil {
		return 0, errorValue
	}
	if errorValue := os.Chmod(logPath, 0o600); errorValue != nil {
		_ = logFile.Close()
		return 0, errorValue
	}
	logWriter := newLiteralRedactingWriter(logFile, specification.SensitiveValues)
	commandValue := exec.Command(specification.Path, specification.Arguments...)
	commandValue.Dir = specification.WorkingDirectory
	commandValue.Env = specification.Environment
	commandValue.Stdout = logWriter
	commandValue.Stderr = logWriter
	commandValue.Stdin = nil
	if runtime.GOOS != "windows" {
		commandValue.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if errorValue := commandValue.Start(); errorValue != nil {
		_ = logWriter.Close()
		_ = logFile.Close()
		return 0, errorValue
	}
	go func() {
		_ = commandValue.Wait()
		_ = logWriter.Close()
		_ = logFile.Close()
	}()
	return commandValue.Process.Pid, nil
}
