// Package loops schedules recurring prompts for a session.
//
// This file owns the manager and its lifecycle. Job records and interval
// parsing live in job.go, per-job timing in scheduler.go, persistence in
// store.go, and tunable limits in policy.go.
package loops

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	jobIDGenerationAttempts = 8
	jobIDRandomBytes        = 5
)

var (
	StoreRequiredError   = errors.New("loop store path required")
	SessionRequiredError = errors.New("sessionId required")
	PromptRequiredError  = errors.New("prompt required")
	BadIntervalError     = errors.New("bad interval")
	MaximumJobsError     = errors.New("maximum loop jobs exceeded")
)

// FireFunc delivers one scheduled prompt. The note is formatted exactly as the
// original remote loop message and the context is cancelled when the job stops.
type FireFunc func(operationContext context.Context, job Job, note string) error

type Manager struct {
	mutex            sync.Mutex
	store            string
	fire             FireFunc
	jobs             map[string]*Job
	tasks            map[string]context.CancelFunc
	operationContext context.Context
	cancel           context.CancelFunc
	running          bool
	waitGroup        sync.WaitGroup
	now              func() time.Time
	policy           Policy
}

func New(store string, fire FireFunc, policy Policy) (*Manager, error) {
	if operationError := policy.Validate(); operationError != nil {
		return nil, operationError
	}
	store = strings.TrimSpace(store)
	if store == "" {
		return nil, StoreRequiredError
	}
	manager := &Manager{
		store:  store,
		fire:   fire,
		policy: policy,
		jobs:   make(map[string]*Job),
		tasks:  make(map[string]context.CancelFunc),
		now:    time.Now,
	}
	if operationError := manager.load(); operationError != nil {
		return nil, operationError
	}
	return manager, nil
}

func (manager *Manager) Start(operationContext context.Context) error {
	if operationContext == nil {
		operationContext = context.Background()
	}
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.running {
		return nil
	}
	manager.operationContext, manager.cancel = context.WithCancel(operationContext)
	manager.running = true
	now := manager.now().Unix()
	changed := false
	for identifier, job := range manager.jobs {
		if job.ExpiresAt <= float64(now) {
			delete(manager.jobs, identifier)
			changed = true
			continue
		}
		manager.startLocked(identifier, true)
	}
	if changed {
		return manager.saveLocked()
	}
	return nil
}

func (manager *Manager) Close() {
	manager.mutex.Lock()
	if !manager.running {
		manager.mutex.Unlock()
		return
	}
	manager.running = false
	if manager.cancel != nil {
		manager.cancel()
	}
	for _, cancel := range manager.tasks {
		cancel()
	}
	manager.mutex.Unlock()
	manager.waitGroup.Wait()
}

func (manager *Manager) Policy() Policy {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	return manager.policy
}
