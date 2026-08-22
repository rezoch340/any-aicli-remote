// Package process owns the native lifecycle for `grok agent serve`.
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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultAgentPort = 2419
	DefaultBindHost  = "127.0.0.1"
)

var (
	ForeignListenerError        = errors.New("agent port is held by a non-owned process")
	GrokNotFoundError           = errors.New("grok binary not found")
	ProcessIdentityChangedError = errors.New("process identity changed")
)

// Config describes the one Grok agent serve instance managed by this daemon.
type Config struct {
	Port             int
	BindHost         string
	Secret           string
	WorkingDirectory string
	GrokPath         string
	LogDirectory     string
	StatePath        string
	AlwaysApprove    bool
	UseLeader        bool
}

// State is persisted to disk so stop/restart can distinguish our agent from other grok sessions.
type State struct {
	ProcessID        int      `json:"pid"`
	Port             int      `json:"port"`
	BindHost         string   `json:"bindHost"`
	WorkingDirectory string   `json:"cwd"`
	GrokPath         string   `json:"grokPath"`
	Arguments        []string `json:"args"`
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
	BindHost       string
	Port           int
}

// Operations allows unit tests to fake operating-system process state.
type Operations struct {
	ListenProcessIDs func(port int, excludeSelf bool) ([]int, error)
	CommandLine      func(processID int) (string, error)
	ProcessAlive     func(processID int) bool
	ProcessStart     func(processID int) (string, error)
	FindGrok         func() (string, error)
	StartProcess     func(StartSpecification) (int, error)
	KillProcess      func(identity ProcessIdentity, gracePeriod time.Duration) error
	Now              func() time.Time
}

// Manager manages exactly one agent serve port and refuses to kill foreign listeners.
type Manager struct {
	Config     Config
	Operations Operations
}

func (manager *Manager) configuration() (Config, error) {
	configuration := manager.Config
	if configuration.Port == 0 {
		configuration.Port = DefaultAgentPort
	}
	if strings.TrimSpace(configuration.BindHost) == "" {
		configuration.BindHost = DefaultBindHost
	}
	if strings.TrimSpace(configuration.WorkingDirectory) == "" {
		workingDirectory, errorValue := os.Getwd()
		if errorValue != nil {
			return configuration, errorValue
		}
		configuration.WorkingDirectory = workingDirectory
	}
	if strings.TrimSpace(configuration.LogDirectory) == "" {
		configuration.LogDirectory = defaultDataPath("logs")
	}
	if strings.TrimSpace(configuration.StatePath) == "" {
		configuration.StatePath = defaultDataPath("agent-state.json")
	}
	return configuration, nil
}

func defaultDataPath(name string) string {
	if base := strings.TrimSpace(os.Getenv("GROK_PLUGIN_DATA")); base != "" {
		return filepath.Join(base, name)
	}
	home, errorValue := os.UserHomeDir()
	if errorValue != nil || home == "" {
		return name
	}
	return filepath.Join(home, ".grok", "plugin-data", "grok-remote", name)
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
	if operations.FindGrok == nil {
		operations.FindGrok = FindGrok
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
	return hex.EncodeToString(sum[:])[:16]
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
	if errorValue := os.MkdirAll(filepath.Dir(configuration.StatePath), 0o755); errorValue != nil {
		return errorValue
	}
	rawData, errorValue := json.MarshalIndent(state, "", "  ")
	if errorValue != nil {
		return errorValue
	}
	temporaryPath := configuration.StatePath + ".tmp"
	defer os.Remove(temporaryPath)
	if errorValue := os.WriteFile(temporaryPath, rawData, 0o644); errorValue != nil {
		return errorValue
	}
	return os.Rename(temporaryPath, configuration.StatePath)
}

func (manager *Manager) removeState() {
	configuration, errorValue := manager.configuration()
	if errorValue == nil {
		_ = os.Remove(configuration.StatePath)
	}
}

func processIdentityFromState(state *State, configuration Config) ProcessIdentity {
	identity := ProcessIdentity{
		ProcessID:      state.ProcessID,
		ProcessStart:   strings.TrimSpace(state.ProcessStart),
		ExecutablePath: state.GrokPath,
		BindHost:       state.BindHost,
		Port:           state.Port,
	}
	if identity.ExecutablePath == "" {
		identity.ExecutablePath = configuration.GrokPath
	}
	if identity.BindHost == "" {
		identity.BindHost = configuration.BindHost
	}
	if identity.Port == 0 {
		identity.Port = configuration.Port
	}
	return identity
}

func commandContainsExecutablePath(commandLine, executablePath string) bool {
	executablePath = strings.TrimSpace(executablePath)
	if executablePath == "" {
		return false
	}
	commandCandidates := []string{executablePath}
	if canonicalPath, pathError := filepath.EvalSymlinks(executablePath); pathError == nil && canonicalPath != executablePath {
		commandCandidates = append(commandCandidates, canonicalPath)
	}
	for _, candidatePath := range commandCandidates {
		if containsDelimitedCommandPart(commandLine, candidatePath, runtime.GOOS == "windows") {
			return true
		}
	}
	return false
}

func containsDelimitedCommandPart(commandLine, expectedPart string, caseInsensitive bool) bool {
	searchLine := commandLine
	searchPart := expectedPart
	if caseInsensitive {
		searchLine = strings.ToLower(searchLine)
		searchPart = strings.ToLower(searchPart)
	}
	searchOffset := 0
	for searchOffset <= len(searchLine)-len(searchPart) {
		relativeIndex := strings.Index(searchLine[searchOffset:], searchPart)
		if relativeIndex < 0 {
			return false
		}
		matchStart := searchOffset + relativeIndex
		matchEnd := matchStart + len(searchPart)
		beforeDelimited := matchStart == 0 || isCommandDelimiter(searchLine[matchStart-1])
		afterDelimited := matchEnd == len(searchLine) || isCommandDelimiter(searchLine[matchEnd])
		if beforeDelimited && afterDelimited {
			return true
		}
		searchOffset = matchStart + 1
	}
	return false
}

func isCommandDelimiter(character byte) bool {
	return character == 0 || character == ' ' || character == '\t' || character == '\r' || character == '\n' || character == '\'' || character == '"' || character == '='
}

func commandLooksLikeAgent(commandLine string, identity ProcessIdentity) bool {
	normalizedCommandLine := " " + strings.Join(strings.Fields(strings.ReplaceAll(commandLine, "\x00", " ")), " ") + " "
	if !commandContainsExecutablePath(commandLine, identity.ExecutablePath) {
		return false
	}
	if !strings.Contains(normalizedCommandLine, " agent ") || !strings.Contains(normalizedCommandLine, " serve ") {
		return false
	}
	expectedBindAddress := identity.BindHost + ":" + strconv.Itoa(identity.Port)
	return strings.Contains(normalizedCommandLine, " --bind ") && strings.Contains(normalizedCommandLine, expectedBindAddress)
}

func (manager *Manager) ownsProcessID(processID int, state *State, configuration Config, operations Operations) bool {
	if state == nil || state.ProcessID != processID || state.Port != configuration.Port || state.SecretHash != secretHash(configuration.Secret) {
		return false
	}
	if operations.ProcessAlive != nil && !operations.ProcessAlive(processID) {
		return false
	}
	// The persisted process-start stamp prevents a stale PID from being reused
	// to stop an unrelated Grok session on the same port. Older state without
	// this stamp fails closed and is treated as foreign.
	if state.ProcessStart == "" {
		return false
	}
	currentProcessStart, errorValue := operations.ProcessStart(processID)
	if errorValue != nil || strings.TrimSpace(currentProcessStart) != state.ProcessStart {
		return false
	}
	commandLine, errorValue := operations.CommandLine(processID)
	if errorValue != nil || strings.TrimSpace(commandLine) == "" {
		return false
	}
	return commandLooksLikeAgent(commandLine, processIdentityFromState(state, configuration))
}

// Status returns current listener ownership without side effects.
func (manager *Manager) Status() Status {
	configuration, errorValue := manager.configuration()
	if errorValue != nil {
		return Status{Port: configuration.Port, BindHost: configuration.BindHost, Error: errorValue.Error()}
	}
	operations := manager.operationsWithDefaults()
	processIDs, errorValue := operations.ListenProcessIDs(configuration.Port, false)
	if errorValue != nil {
		return Status{Port: configuration.Port, BindHost: configuration.BindHost, Error: errorValue.Error()}
	}
	sort.Ints(processIDs)
	state, stateError := manager.LoadState()
	ownedProcessIDs := []int{}
	foreignProcessIDs := []int{}
	for _, processID := range processIDs {
		if manager.ownsProcessID(processID, state, configuration, operations) {
			ownedProcessIDs = append(ownedProcessIDs, processID)
		} else {
			foreignProcessIDs = append(foreignProcessIDs, processID)
		}
	}
	// A freshly spawned agent is owned before it binds the TCP port. Keep that
	// persisted PID visible so another ensure/start cannot create a second child
	// merely because the first one is still starting.
	if stateError == nil && state != nil && !containsProcessID(ownedProcessIDs, state.ProcessID) && manager.ownsProcessID(state.ProcessID, state, configuration, operations) {
		ownedProcessIDs = append(ownedProcessIDs, state.ProcessID)
		sort.Ints(ownedProcessIDs)
	}
	if stateError != nil {
		return Status{Port: configuration.Port, BindHost: configuration.BindHost, ProcessIDs: processIDs, OwnedProcessIDs: ownedProcessIDs, ForeignProcessIDs: foreignProcessIDs, Listening: len(processIDs) > 0, Owned: len(ownedProcessIDs) > 0, Running: len(ownedProcessIDs) > 0, State: state, Error: stateError.Error()}
	}
	return Status{Port: configuration.Port, BindHost: configuration.BindHost, ProcessIDs: processIDs, OwnedProcessIDs: ownedProcessIDs, ForeignProcessIDs: foreignProcessIDs, Listening: len(processIDs) > 0, Owned: len(ownedProcessIDs) > 0, Running: len(ownedProcessIDs) > 0, State: state}
}

func containsProcessID(processIDs []int, target int) bool {
	for _, processID := range processIDs {
		if processID == target {
			return true
		}
	}
	return false
}

// AgentArgs builds `grok agent serve` args. The secret is passed for compatibility with the old hub.
func AgentArguments(configuration Config) []string {
	if configuration.Port == 0 {
		configuration.Port = DefaultAgentPort
	}
	if configuration.BindHost == "" {
		configuration.BindHost = DefaultBindHost
	}
	arguments := []string{"agent"}
	if configuration.AlwaysApprove {
		arguments = append(arguments, "--always-approve")
	}
	if configuration.UseLeader {
		arguments = append(arguments, "--leader")
	} else {
		arguments = append(arguments, "--no-leader")
	}
	arguments = append(arguments, "serve", "--bind", configuration.BindHost+":"+strconv.Itoa(configuration.Port))
	if configuration.Secret != "" {
		arguments = append(arguments, "--secret", configuration.Secret)
	}
	return arguments
}

func processIdentityForOwnedProcess(status Status, configuration Config, processID int) (ProcessIdentity, error) {
	if status.State == nil || status.State.ProcessID != processID {
		return ProcessIdentity{}, fmt.Errorf("owned process %d has no matching state", processID)
	}
	identity := processIdentityFromState(status.State, configuration)
	if identity.ProcessStart == "" {
		return ProcessIdentity{}, fmt.Errorf("owned process %d has no process-start identity", processID)
	}
	return identity, nil
}

func cleanupSpawnedProcess(operations Operations, identity ProcessIdentity, primaryError error) error {
	cleanupError := operations.KillProcess(identity, 4*time.Second)
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
			if errorValue := operations.KillProcess(identity, 4*time.Second); errorValue != nil {
				return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
			}
		}
		waitUntil(3*time.Second, 100*time.Millisecond, func() bool {
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

	grokPath := configuration.GrokPath
	if grokPath == "" {
		grokPath, errorValue = operations.FindGrok()
		if errorValue != nil {
			return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
		}
	}
	arguments := AgentArguments(configuration)
	if errorValue := os.MkdirAll(configuration.LogDirectory, 0o755); errorValue != nil {
		return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
	}
	specification := StartSpecification{Path: grokPath, Arguments: arguments, WorkingDirectory: configuration.WorkingDirectory, LogPath: filepath.Join(configuration.LogDirectory, "agent.spawn.log"), Environment: append(os.Environ(), "GROK_AGENT_SECRET="+configuration.Secret, "GROK_REMOTE_AGENT_PORT="+strconv.Itoa(configuration.Port), "GROK_REMOTE_DAEMON=1")}
	processID, errorValue := operations.StartProcess(specification)
	if errorValue != nil {
		return StartResult{OK: false, Message: errorValue.Error(), Status: status}, errorValue
	}
	processStartStamp, processStartError := operations.ProcessStart(processID)
	if processStartError != nil || strings.TrimSpace(processStartStamp) == "" {
		if processStartError == nil {
			processStartError = errors.New("could not identify spawned agent process")
		}
		identity := ProcessIdentity{ProcessID: processID, ExecutablePath: grokPath, BindHost: configuration.BindHost, Port: configuration.Port}
		combinedError := cleanupSpawnedProcess(operations, identity, processStartError)
		return StartResult{OK: false, Message: combinedError.Error(), Status: status}, combinedError
	}
	state := State{ProcessID: processID, Port: configuration.Port, BindHost: configuration.BindHost, WorkingDirectory: configuration.WorkingDirectory, GrokPath: grokPath, Arguments: redactArguments(arguments), SecretHash: secretHash(configuration.Secret), StartedAt: operations.Now().UTC().Format(time.RFC3339Nano), ProcessStart: strings.TrimSpace(processStartStamp)}
	if errorValue := manager.saveState(state); errorValue != nil {
		combinedError := cleanupSpawnedProcess(operations, processIdentityFromState(&state, configuration), errorValue)
		return StartResult{OK: false, Message: combinedError.Error(), Status: status}, combinedError
	}
	return StartResult{OK: true, Message: fmt.Sprintf("agent started on :%d", configuration.Port), Started: true, ProcessID: processID, Status: manager.Status()}, nil
}

func redactArguments(arguments []string) []string {
	output := append([]string(nil), arguments...)
	for argumentIndex := 0; argumentIndex < len(output)-1; argumentIndex++ {
		if output[argumentIndex] == "--secret" {
			output[argumentIndex+1] = "***"
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
		if errorValue := operations.KillProcess(identity, 4*time.Second); errorValue != nil {
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

// FindGrok locates the native grok executable.
func FindGrok() (string, error) {
	home, _ := os.UserHomeDir()
	candidates := []string{}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".grok", "bin", "grok"), filepath.Join(home, ".grok", "bin", "grok.exe"))
	}
	if candidatePath, errorValue := exec.LookPath("grok"); errorValue == nil && candidatePath != "" {
		candidates = append(candidates, candidatePath)
	}
	for _, candidatePath := range candidates {
		if state, errorValue := os.Stat(candidatePath); errorValue == nil && !state.IsDir() {
			return candidatePath, nil
		}
	}
	return "", GrokNotFoundError
}

// StartProcess starts the process detached enough for a macOS menu-bar/daemon parent.
func StartProcess(specification StartSpecification) (int, error) {
	logPath := specification.LogPath
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "grok-agent.spawn.log")
	}
	if errorValue := os.MkdirAll(filepath.Dir(logPath), 0o755); errorValue != nil {
		return 0, errorValue
	}
	logf, errorValue := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if errorValue != nil {
		return 0, errorValue
	}
	commandValue := exec.Command(specification.Path, specification.Arguments...)
	commandValue.Dir = specification.WorkingDirectory
	commandValue.Env = specification.Environment
	commandValue.Stdout = logf
	commandValue.Stderr = logf
	commandValue.Stdin = nil
	if runtime.GOOS != "windows" {
		commandValue.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if errorValue := commandValue.Start(); errorValue != nil {
		_ = logf.Close()
		return 0, errorValue
	}
	go func() {
		_ = commandValue.Wait()
		_ = logf.Close()
	}()
	return commandValue.Process.Pid, nil
}

func verifyProcessIdentity(identity ProcessIdentity, operations Operations) (bool, error) {
	if identity.ProcessID <= 0 || !operations.ProcessAlive(identity.ProcessID) {
		return false, nil
	}
	if identity.ProcessStart != "" {
		currentProcessStart, processStartError := operations.ProcessStart(identity.ProcessID)
		if processStartError != nil {
			if !operations.ProcessAlive(identity.ProcessID) {
				return false, nil
			}
			return false, fmt.Errorf("verify process %d start time: %w", identity.ProcessID, processStartError)
		}
		if strings.TrimSpace(currentProcessStart) != strings.TrimSpace(identity.ProcessStart) {
			return false, fmt.Errorf("%w for process %d", ProcessIdentityChangedError, identity.ProcessID)
		}
	}
	commandLine, commandLineError := operations.CommandLine(identity.ProcessID)
	if commandLineError != nil {
		if !operations.ProcessAlive(identity.ProcessID) {
			return false, nil
		}
		return false, fmt.Errorf("verify process %d command: %w", identity.ProcessID, commandLineError)
	}
	if !commandLooksLikeAgent(commandLine, identity) {
		return false, fmt.Errorf("%w for process %d", ProcessIdentityChangedError, identity.ProcessID)
	}
	return true, nil
}

// KillProcess verifies the immutable identity before every signal so PID reuse
// cannot turn a managed-agent shutdown into a signal to an unrelated process.
func KillProcess(identity ProcessIdentity, gracePeriod time.Duration) error {
	identityOperations := Operations{CommandLine: CommandLine, ProcessAlive: ProcessAlive, ProcessStart: ProcessStart}
	identityMatches, verificationError := verifyProcessIdentity(identity, identityOperations)
	if verificationError != nil || !identityMatches {
		return verificationError
	}
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/F", "/PID", strconv.Itoa(identity.ProcessID)).Run()
	}
	if signalError := syscall.Kill(identity.ProcessID, syscall.SIGTERM); signalError != nil {
		if errors.Is(signalError, syscall.ESRCH) {
			return nil
		}
		return signalError
	}
	deadline := time.Now().Add(gracePeriod)
	for time.Now().Before(deadline) {
		identityMatches, verificationError = verifyProcessIdentity(identity, identityOperations)
		if verificationError != nil {
			return verificationError
		}
		if !identityMatches {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	identityMatches, verificationError = verifyProcessIdentity(identity, identityOperations)
	if verificationError != nil {
		return verificationError
	}
	if !identityMatches {
		return nil
	}
	if signalError := syscall.Kill(identity.ProcessID, syscall.SIGKILL); signalError != nil {
		if errors.Is(signalError, syscall.ESRCH) {
			return nil
		}
		return signalError
	}
	stopped := waitUntil(2*time.Second, 50*time.Millisecond, func() bool {
		identityMatches, verificationError = verifyProcessIdentity(identity, identityOperations)
		return verificationError != nil || !identityMatches
	})
	if verificationError != nil {
		return verificationError
	}
	if !stopped {
		return fmt.Errorf("process %d did not exit after SIGKILL", identity.ProcessID)
	}
	return nil
}

// ProcessAlive reports whether pid exists.
func ProcessAlive(processID int) bool {
	if processID <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		errorValue := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(processID)).Run()
		return errorValue == nil
	}
	errorValue := syscall.Kill(processID, 0)
	return errorValue == nil || errors.Is(errorValue, syscall.EPERM)
}

// CommandLine returns `ps -p pid -o command=` output.
func CommandLine(processID int) (string, error) {
	if processID <= 0 {
		return "", os.ErrInvalid
	}
	if runtime.GOOS == "windows" {
		output, errorValue := exec.Command("wmic", "process", "where", "ProcessId="+strconv.Itoa(processID), "get", "CommandLine", "/value").Output()
		return strings.TrimSpace(string(output)), errorValue
	}
	output, errorValue := exec.Command("ps", "-ww", "-p", strconv.Itoa(processID), "-o", "command=").Output()
	return strings.TrimSpace(string(output)), errorValue
}

// ProcessStart returns the OS process start stamp used to reject stale PID reuse.
func ProcessStart(processID int) (string, error) {
	if processID <= 0 {
		return "", os.ErrInvalid
	}
	if runtime.GOOS == "windows" {
		output, errorValue := exec.Command("wmic", "process", "where", "ProcessId="+strconv.Itoa(processID), "get", "CreationDate", "/value").Output()
		return strings.TrimSpace(string(output)), errorValue
	}
	output, errorValue := exec.Command("ps", "-p", strconv.Itoa(processID), "-o", "lstart=").Output()
	return strings.TrimSpace(string(output)), errorValue
}

// ListenPIDsPort returns PIDs listening on TCP port. On macOS it uses lsof.
func ListenProcessIDsPort(port int, excludeSelf bool) ([]int, error) {
	if port <= 0 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	add := func(destination map[int]bool, value string) {
		processID, errorValue := strconv.Atoi(strings.TrimSpace(value))
		if errorValue == nil && processID > 0 && !(excludeSelf && processID == os.Getpid()) {
			destination[processID] = true
		}
	}
	found := map[int]bool{}
	if candidatePath, errorValue := exec.LookPath("lsof"); errorValue == nil {
		output, _ := exec.Command(candidatePath, "-nP", "-t", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN").Output()
		for _, line := range strings.Split(string(output), "\n") {
			add(found, line)
		}
	}
	if len(found) == 0 && runtime.GOOS == "linux" {
		if candidatePath, errorValue := exec.LookPath("ss"); errorValue == nil {
			output, _ := exec.Command(candidatePath, "-ltnp").Output()
			needle := ":" + strconv.Itoa(port)
			for _, line := range strings.Split(string(output), "\n") {
				if !strings.Contains(line, needle) || !strings.Contains(line, "pid=") {
					continue
				}
				rest := strings.SplitN(line, "pid=", 2)[1]
				processID := strings.FieldsFunc(rest, func(currentRune rune) bool { return currentRune < '0' || currentRune > '9' })
				if len(processID) > 0 {
					add(found, processID[0])
				}
			}
		}
	}
	processIDs := make([]int, 0, len(found))
	for processID := range found {
		processIDs = append(processIDs, processID)
	}
	sort.Ints(processIDs)
	return processIDs, nil
}
