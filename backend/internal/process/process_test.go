package process

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeOS struct {
	listeners     map[int][]int
	commands      map[int]string
	alive         map[int]bool
	starts        map[int]string
	killed        []int
	killPolicies  []LifecyclePolicy
	started       []StartSpecification
	nextProcessID int
}

func newFakeOS() *fakeOS {
	return &fakeOS{listeners: map[int][]int{}, commands: map[int]string{}, alive: map[int]bool{}, starts: map[int]string{}, nextProcessID: 500}
}

func (fakeSystem *fakeOS) operations() Operations {
	return Operations{
		ListenProcessIDs: func(port int, excludeSelf bool) ([]int, error) {
			return append([]int(nil), fakeSystem.listeners[port]...), nil
		},
		CommandLine: func(processID int) (string, error) {
			if commandLine, found := fakeSystem.commands[processID]; found {
				return commandLine, nil
			}
			return "", os.ErrNotExist
		},
		ProcessAlive: func(processID int) bool { return fakeSystem.alive[processID] },
		ProcessStart: func(processID int) (string, error) {
			if processStartStamp, found := fakeSystem.starts[processID]; found {
				return processStartStamp, nil
			}
			return "", os.ErrNotExist
		},
		StartProcess: func(specification StartSpecification) (int, error) {
			fakeSystem.nextProcessID++
			processID := fakeSystem.nextProcessID
			fakeSystem.started = append(fakeSystem.started, specification)
			fakeSystem.alive[processID] = true
			fakeSystem.starts[processID] = "Sat Aug 22 21:00:00 2026"
			port := 0
			for argumentIndex := 0; argumentIndex < len(specification.Arguments)-1; argumentIndex++ {
				if specification.Arguments[argumentIndex] == "--bind" {
					if strings.HasSuffix(specification.Arguments[argumentIndex+1], ":2419") {
						port = 2419
					}
					if strings.HasSuffix(specification.Arguments[argumentIndex+1], ":2420") {
						port = 2420
					}
				}
			}
			if port != 0 {
				fakeSystem.listeners[port] = []int{processID}
			}
			fakeSystem.commands[processID] = specification.Path + " " + strings.Join(specification.Arguments, " ")
			return processID, nil
		},
		KillProcess: func(identity ProcessIdentity, policy LifecyclePolicy) error {
			fakeSystem.killed = append(fakeSystem.killed, identity.ProcessID)
			fakeSystem.killPolicies = append(fakeSystem.killPolicies, policy)
			fakeSystem.alive[identity.ProcessID] = false
			for port, processIDs := range fakeSystem.listeners {
				output := processIDs[:0]
				for _, candidateProcessID := range processIDs {
					if candidateProcessID != identity.ProcessID {
						output = append(output, candidateProcessID)
					}
				}
				fakeSystem.listeners[port] = output
			}
			return nil
		},
		Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

func testManager(testContext *testing.T, fakeSystem *fakeOS) *Manager {
	testContext.Helper()
	directoryPath := testContext.TempDir()
	return &Manager{Config: Config{Port: 2419, BindHost: "127.0.0.1", Secret: "sekret", RuntimeDirectory: directoryPath, ExecutablePath: "/usr/local/bin/provider-agent", Arguments: []string{"agent", "serve", "--bind", "127.0.0.1:2419", "--secret", "sekret"}, IdentityTokens: []string{"agent", "serve", "127.0.0.1:2419"}, LogDirectory: filepath.Join(directoryPath, "logs"), StatePath: filepath.Join(directoryPath, "state.json"), LifecyclePolicy: LifecyclePolicy{KillGrace: time.Second, RestartWait: time.Second, RestartPoll: time.Millisecond, PostKillDelay: time.Millisecond, StopWait: time.Second, StopPoll: time.Millisecond}}, Operations: fakeSystem.operations()}
}

func TestManagerRejectsMissingLifecyclePolicy(testContext *testing.T) {
	manager := testManager(testContext, newFakeOS())
	manager.Config.LifecyclePolicy = LifecyclePolicy{}
	if _, operationError := manager.Start(false); operationError == nil {
		testContext.Fatal("Start accepted missing lifecycle policy")
	}
}

func TestForceRestartUsesConfiguredLifecyclePolicy(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	policy := LifecyclePolicy{KillGrace: 17 * time.Millisecond, RestartWait: 19 * time.Millisecond, RestartPoll: 2 * time.Millisecond, PostKillDelay: 3 * time.Millisecond, StopWait: 23 * time.Millisecond, StopPoll: 4 * time.Millisecond}
	manager.Config.LifecyclePolicy = policy
	if _, operationError := manager.Start(false); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := manager.Start(true); operationError != nil {
		testContext.Fatal(operationError)
	}
	if !reflect.DeepEqual(fakeSystem.killPolicies, []LifecyclePolicy{policy}) {
		testContext.Fatalf("kill policies = %#v, expected %#v", fakeSystem.killPolicies, []LifecyclePolicy{policy})
	}
}

func TestStartRefusesForeignListener(testContext *testing.T) {
	fakeSystem := newFakeOS()
	fakeSystem.listeners[2419] = []int{111}
	fakeSystem.alive[111] = true
	fakeSystem.commands[111] = "/usr/local/bin/provider-agent agent serve --bind 127.0.0.1:2419"
	manager := testManager(testContext, fakeSystem)

	result, errorValue := manager.Start(false)
	if !errors.Is(errorValue, ForeignListenerError) || result.OK || len(fakeSystem.killed) != 0 || len(fakeSystem.started) != 0 {
		testContext.Fatalf("res=%+v err=%v killed=%v started=%v", result, errorValue, fakeSystem.killed, fakeSystem.started)
	}
}

func TestStartCreatesStateAndRedactsSecret(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	result, errorValue := manager.Start(false)
	if errorValue != nil || !result.OK || !result.Started || result.ProcessID == 0 {
		testContext.Fatalf("res=%+v err=%v", result, errorValue)
	}
	if len(fakeSystem.started) != 1 {
		testContext.Fatalf("started=%d", len(fakeSystem.started))
	}
	specification := fakeSystem.started[0]
	if specification.Path != "/usr/local/bin/provider-agent" || strings.Join(specification.Arguments, " ") != "agent serve --bind 127.0.0.1:2419 --secret sekret" {
		testContext.Fatalf("bad spec=%+v", specification)
	}
	state, errorValue := manager.LoadState()
	if errorValue != nil || state == nil || state.ProcessID != result.ProcessID || state.SecretHash == "" {
		testContext.Fatalf("state=%+v err=%v", state, errorValue)
	}
	if strings.Contains(strings.Join(state.Arguments, " "), "sekret") || !strings.Contains(strings.Join(state.Arguments, " "), "--secret ***") {
		testContext.Fatalf("secret not redacted in state args=%v", state.Arguments)
	}
	status := manager.Status()
	if !status.Owned || !status.Running || len(status.ForeignProcessIDs) != 0 || len(status.OwnedProcessIDs) != 1 {
		testContext.Fatalf("status=%+v", status)
	}
}

func TestStartingProcessRemainsOwnedAndDoesNotRespawn(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	first, errorValue := manager.Start(false)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	// Model the interval after os.StartProcess succeeds but before the child
	// binds its serve port.
	fakeSystem.listeners[2419] = nil
	status := manager.Status()
	if status.Listening || !status.Running || !status.Owned || !reflect.DeepEqual(status.OwnedProcessIDs, []int{first.ProcessID}) {
		testContext.Fatalf("starting status=%+v", status)
	}
	second, errorValue := manager.Start(false)
	if errorValue != nil || second.Started || second.ProcessID != first.ProcessID || len(fakeSystem.started) != 1 {
		testContext.Fatalf("second=%+v err=%v started=%d", second, errorValue, len(fakeSystem.started))
	}
	stopped, errorValue := manager.Stop()
	if errorValue != nil || !reflect.DeepEqual(stopped.Killed, []int{first.ProcessID}) {
		testContext.Fatalf("stop starting process=%+v err=%v", stopped, errorValue)
	}
}

func TestStartKillsChildWhenIdentityCannotBePersisted(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	manager.Operations.ProcessStart = func(int) (string, error) { return "", os.ErrNotExist }
	result, errorValue := manager.Start(false)
	if errorValue == nil || result.OK || len(fakeSystem.started) != 1 || !reflect.DeepEqual(fakeSystem.killed, []int{501}) {
		testContext.Fatalf("result=%+v err=%v started=%d killed=%v", result, errorValue, len(fakeSystem.started), fakeSystem.killed)
	}
}

func TestStartReturnsCleanupFailure(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	identityFailure := errors.New("identity unavailable")
	cleanupFailure := errors.New("cleanup failed")
	manager.Operations.ProcessStart = func(int) (string, error) { return "", identityFailure }
	manager.Operations.KillProcess = func(ProcessIdentity, LifecyclePolicy) error { return cleanupFailure }

	result, errorValue := manager.Start(false)
	if result.OK || !errors.Is(errorValue, identityFailure) || !errors.Is(errorValue, cleanupFailure) {
		testContext.Fatalf("result=%+v error=%v", result, errorValue)
	}
	if !strings.Contains(result.Message, "clean up spawned agent") {
		testContext.Fatalf("cleanup failure missing from message: %q", result.Message)
	}
}

func TestStartReturnsCleanupFailureWhenStateSaveFails(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	manager.Config.StatePath = filepath.Join(testContext.TempDir(), "state.json")
	startProcess := manager.Operations.StartProcess
	manager.Operations.StartProcess = func(specification StartSpecification) (int, error) {
		processID, startError := startProcess(specification)
		if startError == nil {
			if directoryError := os.Mkdir(manager.Config.StatePath, 0o700); directoryError != nil {
				return 0, directoryError
			}
		}
		return processID, startError
	}
	cleanupFailure := errors.New("cleanup failed")
	manager.Operations.KillProcess = func(ProcessIdentity, LifecyclePolicy) error { return cleanupFailure }

	result, errorValue := manager.Start(false)
	if result.OK || errorValue == nil || !errors.Is(errorValue, cleanupFailure) {
		testContext.Fatalf("result=%+v error=%v", result, errorValue)
	}
	if !strings.Contains(result.Message, "clean up spawned agent") {
		testContext.Fatalf("cleanup failure missing from message: %q", result.Message)
	}
}

func TestCommandIdentitySupportsSpacesAndInterpreterWrappers(testContext *testing.T) {
	identity := ProcessIdentity{
		ProcessID:      901,
		ExecutablePath: "/Users/Example User/bin/provider-agent",
		IdentityTokens: []string{"agent", "serve", "127.0.0.1:2419"},
	}
	commandLine := `/usr/bin/env node "/Users/Example User/bin/provider-agent" agent --always-approve --no-leader serve --bind 127.0.0.1:2419 --secret hidden`
	if !commandLooksLikeAgent(commandLine, identity) {
		testContext.Fatalf("valid wrapped command was rejected: %q", commandLine)
	}
	lookalikeCommand := `/usr/bin/env node "/Users/Example User/bin/provider-agent-malicious" agent serve --bind 127.0.0.1:2419`
	if commandLooksLikeAgent(lookalikeCommand, identity) {
		testContext.Fatalf("lookalike executable was accepted: %q", lookalikeCommand)
	}
}

func TestStatusOwnsWrappedAgentFromPathWithSpaces(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	manager.Config.ExecutablePath = "/Users/Example User/bin/provider-agent"
	started, errorValue := manager.Start(false)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	fakeSystem.commands[started.ProcessID] = `/usr/bin/env node "/Users/Example User/bin/provider-agent" agent --always-approve --no-leader serve --bind 127.0.0.1:2419 --secret sekret`

	status := manager.Status()
	if !status.Owned || !reflect.DeepEqual(status.OwnedProcessIDs, []int{started.ProcessID}) || len(status.ForeignProcessIDs) != 0 {
		testContext.Fatalf("status=%+v", status)
	}
}

func TestVerifyProcessIdentityRejectsReusedProcessID(testContext *testing.T) {
	fakeSystem := newFakeOS()
	const reusedProcessID = 777
	fakeSystem.alive[reusedProcessID] = true
	fakeSystem.starts[reusedProcessID] = "replacement start"
	fakeSystem.commands[reusedProcessID] = "/usr/local/bin/provider-agent agent serve --bind 127.0.0.1:2419"
	identity := ProcessIdentity{
		ProcessID:      reusedProcessID,
		ProcessStart:   "original start",
		ExecutablePath: "/usr/local/bin/provider-agent",
		IdentityTokens: []string{"agent", "serve", "127.0.0.1:2419"},
	}

	identityMatches, errorValue := verifyProcessIdentity(identity, fakeSystem.operations())
	if identityMatches || !errors.Is(errorValue, ProcessIdentityChangedError) {
		testContext.Fatalf("identityMatches=%v error=%v", identityMatches, errorValue)
	}
}

func TestForceRestartKillsOnlyOwned(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	first, errorValue := manager.Start(false)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	second, errorValue := manager.Start(true)
	if errorValue != nil || !second.OK || !second.Started {
		testContext.Fatalf("second=%+v err=%v", second, errorValue)
	}
	if !reflect.DeepEqual(fakeSystem.killed, []int{first.ProcessID}) {
		testContext.Fatalf("killed=%v want [%d]", fakeSystem.killed, first.ProcessID)
	}
	if second.ProcessID == first.ProcessID || len(fakeSystem.started) != 2 {
		testContext.Fatalf("first=%d second=%d started=%d", first.ProcessID, second.ProcessID, len(fakeSystem.started))
	}
}

func TestStopLeavesForeignUntouched(testContext *testing.T) {
	fakeSystem := newFakeOS()
	fakeSystem.listeners[2419] = []int{222}
	fakeSystem.alive[222] = true
	fakeSystem.commands[222] = "/usr/local/bin/provider-agent agent serve --bind 127.0.0.1:2419"
	manager := testManager(testContext, fakeSystem)
	result, errorValue := manager.Stop()
	if !errors.Is(errorValue, ForeignListenerError) || result.OK || len(fakeSystem.killed) != 0 {
		testContext.Fatalf("res=%+v err=%v killed=%v", result, errorValue, fakeSystem.killed)
	}
}

func TestStopOwnedRemovesState(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	start, errorValue := manager.Start(false)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	stop, errorValue := manager.Stop()
	if errorValue != nil || !stop.OK || !reflect.DeepEqual(stop.Killed, []int{start.ProcessID}) {
		testContext.Fatalf("stop=%+v err=%v", stop, errorValue)
	}
	if state, errorValue := manager.LoadState(); errorValue != nil || state != nil {
		testContext.Fatalf("state after stop=%+v err=%v", state, errorValue)
	}
}

func TestMergeEnvironmentReplacesInheritedProviderSecret(testContext *testing.T) {
	merged := mergeEnvironment(
		[]string{"PATH=/usr/bin", "GROK_AGENT_SECRET=inherited"},
		[]string{"GROK_AGENT_SECRET=transport", "EXTRA=value"},
	)
	if !reflect.DeepEqual(merged, []string{"PATH=/usr/bin", "GROK_AGENT_SECRET=transport", "EXTRA=value"}) {
		testContext.Fatalf("merged environment = %#v", merged)
	}
}

func TestSystemProcessInspectionReadsCurrentProcess(testContext *testing.T) {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
	default:
		testContext.Skip("gopsutil CreateTime is not supported by this operating system")
	}
	processID := os.Getpid()
	if !ProcessAlive(processID) {
		testContext.Fatalf("current process %d was not running", processID)
	}
	commandLine, operationError := CommandLine(processID)
	if operationError != nil || strings.TrimSpace(commandLine) == "" {
		testContext.Fatalf("current process command=%q error=%v", commandLine, operationError)
	}
	firstStart, operationError := ProcessStart(processID)
	if operationError != nil || strings.TrimSpace(firstStart) == "" {
		testContext.Fatalf("current process start=%q error=%v", firstStart, operationError)
	}
	secondStart, operationError := ProcessStart(processID)
	if operationError != nil || secondStart != firstStart {
		testContext.Fatalf("unstable process start: first=%q second=%q error=%v", firstStart, secondStart, operationError)
	}
	if runtime.GOOS != "windows" {
		if _, operationError := time.ParseInLocation("Mon Jan _2 15:04:05 2006", firstStart, time.Local); operationError != nil {
			testContext.Fatalf("process start no longer matches persisted ps format: %q: %v", firstStart, operationError)
		}
	}
}

func TestSystemListenerInspectionFindsLocalTCPListener(testContext *testing.T) {
	listener, operationError := net.Listen("tcp", "127.0.0.1:0")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	processIDs, operationError := ListenProcessIDsPort(port, false)
	if operationError != nil {
		testContext.Fatalf("inspect local listener: %v", operationError)
	}
	if !containsProcessID(processIDs, os.Getpid()) {
		testContext.Fatalf("current process %d missing from listener owners %v", os.Getpid(), processIDs)
	}
	withoutSelf, operationError := ListenProcessIDsPort(port, true)
	if operationError != nil {
		testContext.Fatalf("inspect listener excluding self: %v", operationError)
	}
	if containsProcessID(withoutSelf, os.Getpid()) {
		testContext.Fatalf("current process was not excluded: %v", withoutSelf)
	}
}

func TestStartProcessRedactsChildOutputAndProtectsLog(testContext *testing.T) {
	if runtime.GOOS == "windows" {
		testContext.Skip("shell fixture is Unix-specific")
	}
	transportSecret := "child-output-transport-secret"
	logPath := filepath.Join(testContext.TempDir(), "provider.log")
	processID, operationError := StartProcess(StartSpecification{
		Path: "/bin/sh", Arguments: []string{"-c", `printf 'secret=%s\n' "$TEST_TRANSPORT_SECRET"`},
		Environment:     append(os.Environ(), "TEST_TRANSPORT_SECRET="+transportSecret),
		SensitiveValues: []string{transportSecret}, WorkingDirectory: testContext.TempDir(), LogPath: logPath,
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !waitUntil(3*time.Second, 25*time.Millisecond, func() bool { return !ProcessAlive(processID) }) {
		testContext.Fatalf("child process %d did not exit", processID)
	}
	var logData []byte
	if !waitUntil(2*time.Second, 25*time.Millisecond, func() bool {
		logData, operationError = os.ReadFile(logPath)
		return operationError == nil && len(logData) > 0
	}) {
		testContext.Fatalf("read child log: %v", operationError)
	}
	logText := string(logData)
	if strings.Contains(logText, transportSecret) || logText != "secret=[REDACTED]\n" {
		testContext.Fatalf("child log was not redacted: %q", logText)
	}
	fileInfo, operationError := os.Stat(logPath)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		testContext.Fatalf("child log permissions = %o", fileInfo.Mode().Perm())
	}
}

func TestStartCreatesPrivateLogDirectory(testingContext *testing.T) {
	manager := testManager(testingContext, newFakeOS())
	if _, operationError := manager.Start(false); operationError != nil {
		testingContext.Fatal(operationError)
	}
	information, operationError := os.Stat(manager.Config.LogDirectory)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if information.Mode().Perm() != 0o700 {
		testingContext.Fatalf("log directory mode = %o", information.Mode().Perm())
	}
}
