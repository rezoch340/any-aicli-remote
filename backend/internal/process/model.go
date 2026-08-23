// Managed process contract: tunable lifecycle timing, the managed instance
// description, persisted ownership state, and the results returned to callers.

package process

import (
	"errors"
	"time"
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
