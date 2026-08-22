package process

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	started       []StartSpecification
	nextProcessID int
	findGrok      string
}

func newFakeOS() *fakeOS {
	return &fakeOS{listeners: map[int][]int{}, commands: map[int]string{}, alive: map[int]bool{}, starts: map[int]string{}, nextProcessID: 500, findGrok: "/usr/local/bin/grok"}
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
		FindGrok: func() (string, error) {
			if fakeSystem.findGrok == "" {
				return "", GrokNotFoundError
			}
			return fakeSystem.findGrok, nil
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
		KillProcess: func(identity ProcessIdentity, _ time.Duration) error {
			fakeSystem.killed = append(fakeSystem.killed, identity.ProcessID)
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
	return &Manager{Config: Config{Port: 2419, BindHost: "127.0.0.1", Secret: "sekret", WorkingDirectory: directoryPath, LogDirectory: filepath.Join(directoryPath, "logs"), StatePath: filepath.Join(directoryPath, "state.json"), AlwaysApprove: true}, Operations: fakeSystem.operations()}
}

func TestAgentArguments(testContext *testing.T) {
	arguments := AgentArguments(Config{Port: 2419, BindHost: "127.0.0.1", Secret: "s", AlwaysApprove: true})
	expected := []string{"agent", "--always-approve", "--no-leader", "serve", "--bind", "127.0.0.1:2419", "--secret", "s"}
	if !reflect.DeepEqual(arguments, expected) {
		testContext.Fatalf("args=%#v", arguments)
	}
	leader := AgentArguments(Config{Port: 2420, BindHost: "127.0.0.1", UseLeader: true})
	if strings.Join(leader, " ") != "agent --leader serve --bind 127.0.0.1:2420" {
		testContext.Fatalf("leader args=%#v", leader)
	}
}

func TestStartRefusesForeignListener(testContext *testing.T) {
	fakeSystem := newFakeOS()
	fakeSystem.listeners[2419] = []int{111}
	fakeSystem.alive[111] = true
	fakeSystem.commands[111] = "/usr/local/bin/grok agent serve --bind 127.0.0.1:2419"
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
	if specification.Path != "/usr/local/bin/grok" || !strings.Contains(strings.Join(specification.Arguments, " "), "agent --always-approve --no-leader serve --bind 127.0.0.1:2419 --secret sekret") {
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
	manager.Operations.KillProcess = func(ProcessIdentity, time.Duration) error { return cleanupFailure }

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
	manager.Operations.KillProcess = func(ProcessIdentity, time.Duration) error { return cleanupFailure }

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
		ExecutablePath: "/Users/Example User/.grok/bin/grok",
		BindHost:       "127.0.0.1",
		Port:           2419,
	}
	commandLine := `/usr/bin/env node "/Users/Example User/.grok/bin/grok" agent --always-approve --no-leader serve --bind 127.0.0.1:2419 --secret hidden`
	if !commandLooksLikeAgent(commandLine, identity) {
		testContext.Fatalf("valid wrapped command was rejected: %q", commandLine)
	}
	lookalikeCommand := `/usr/bin/env node "/Users/Example User/.grok/bin/grok-malicious" agent serve --bind 127.0.0.1:2419`
	if commandLooksLikeAgent(lookalikeCommand, identity) {
		testContext.Fatalf("lookalike executable was accepted: %q", lookalikeCommand)
	}
}

func TestStatusOwnsWrappedAgentFromPathWithSpaces(testContext *testing.T) {
	fakeSystem := newFakeOS()
	manager := testManager(testContext, fakeSystem)
	manager.Config.GrokPath = "/Users/Example User/.grok/bin/grok"
	started, errorValue := manager.Start(false)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	fakeSystem.commands[started.ProcessID] = `/usr/bin/env node "/Users/Example User/.grok/bin/grok" agent --always-approve --no-leader serve --bind 127.0.0.1:2419 --secret sekret`

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
	fakeSystem.commands[reusedProcessID] = "/usr/local/bin/grok agent serve --bind 127.0.0.1:2419"
	identity := ProcessIdentity{
		ProcessID:      reusedProcessID,
		ProcessStart:   "original start",
		ExecutablePath: "/usr/local/bin/grok",
		BindHost:       "127.0.0.1",
		Port:           2419,
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
	fakeSystem.commands[222] = "/usr/local/bin/grok agent serve --bind 127.0.0.1:2419"
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
