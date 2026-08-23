package process

import (
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

	systemnetwork "github.com/shirou/gopsutil/v4/net"
	systemprocess "github.com/shirou/gopsutil/v4/process"
)

func processIdentityFromState(state *State, configuration Config) ProcessIdentity {
	identity := ProcessIdentity{
		ProcessID:      state.ProcessID,
		ProcessStart:   strings.TrimSpace(state.ProcessStart),
		ExecutablePath: state.ExecutablePath,
		IdentityTokens: append([]string(nil), state.IdentityTokens...),
	}
	if identity.ExecutablePath == "" {
		identity.ExecutablePath = configuration.ExecutablePath
	}
	if len(identity.IdentityTokens) == 0 {
		identity.IdentityTokens = append([]string(nil), configuration.IdentityTokens...)
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
	if !commandContainsExecutablePath(commandLine, identity.ExecutablePath) {
		return false
	}
	for _, identityToken := range identity.IdentityTokens {
		if strings.TrimSpace(identityToken) == "" {
			continue
		}
		if !containsDelimitedCommandPart(commandLine, identityToken, runtime.GOOS == "windows") {
			return false
		}
	}
	return len(identity.IdentityTokens) > 0
}

func (manager *Manager) ownsProcessID(processID int, state *State, configuration Config, operations Operations) bool {
	if state == nil || state.ProcessID != processID || state.Port != configuration.Port || state.SecretHash != secretHash(configuration.Secret) {
		return false
	}
	if operations.ProcessAlive != nil && !operations.ProcessAlive(processID) {
		return false
	}
	// The persisted process-start stamp prevents a stale PID from being reused
	// to stop an unrelated provider session on the same port. Older state without
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

func inspectProcess(processID int) (*systemprocess.Process, error) {
	if processID <= 0 || int64(processID) > int64(1<<31-1) {
		return nil, os.ErrInvalid
	}
	return systemprocess.NewProcess(int32(processID))
}

// ProcessAlive reports whether the operating system still considers the
// process running.
func ProcessAlive(processID int) bool {
	processInstance, operationError := inspectProcess(processID)
	if operationError != nil {
		return false
	}
	running, operationError := processInstance.IsRunning()
	return operationError == nil && running
}

// CommandLine returns the command line reported by gopsutil.
func CommandLine(processID int) (string, error) {
	processInstance, operationError := inspectProcess(processID)
	if operationError != nil {
		return "", operationError
	}
	commandLine, operationError := processInstance.Cmdline()
	return strings.TrimSpace(commandLine), operationError
}

// ProcessStart returns the immutable process creation time used to reject PID
// reuse. Its representation preserves the former ps/wmic state-file format so
// an upgrade can continue recognizing an already managed process.
func ProcessStart(processID int) (string, error) {
	processInstance, operationError := inspectProcess(processID)
	if operationError != nil {
		return "", operationError
	}
	createdMilliseconds, operationError := processInstance.CreateTime()
	if operationError != nil {
		return "", operationError
	}
	if createdMilliseconds <= 0 {
		return "", errors.New("process creation time unavailable")
	}
	createdAt := time.UnixMilli(createdMilliseconds).In(time.Local)
	if runtime.GOOS == "windows" {
		_, offsetSeconds := createdAt.Zone()
		offsetMinutes := offsetSeconds / 60
		return fmt.Sprintf("CreationDate=%s.%06d%+04d", createdAt.Format("20060102150405"), createdAt.Nanosecond()/1_000, offsetMinutes), nil
	}
	return createdAt.Format("Mon Jan _2 15:04:05 2006"), nil
}

// ListenProcessIDsPort returns the process IDs listening on a TCP port.
func ListenProcessIDsPort(port int, excludeSelf bool) ([]int, error) {
	if port <= 0 || port > 65_535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	connections, operationError := systemnetwork.Connections("tcp")
	if operationError != nil {
		return nil, fmt.Errorf("inspect TCP listeners: %w", operationError)
	}
	found := map[int]struct{}{}
	for _, connection := range connections {
		if connection.Laddr.Port != uint32(port) || !strings.EqualFold(connection.Status, "LISTEN") {
			continue
		}
		if connection.Pid <= 0 {
			return nil, fmt.Errorf("TCP port %d has a listener with unavailable process ownership", port)
		}
		processID := int(connection.Pid)
		if excludeSelf && processID == os.Getpid() {
			continue
		}
		found[processID] = struct{}{}
	}
	processIDs := make([]int, 0, len(found))
	for processID := range found {
		processIDs = append(processIDs, processID)
	}
	sort.Ints(processIDs)
	return processIDs, nil
}
