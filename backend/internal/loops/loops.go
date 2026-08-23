package loops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MinInterval       = 60
	MaxInterval       = 7 * 24 * 60 * 60
	DefaultInterval   = 5 * 60
	DefaultExpiration = 7 * 24 * time.Hour
	MaxJobs           = 50
	fireTimeout       = 10 * time.Minute
)

var (
	StoreRequiredError   = errors.New("loop store path required")
	SessionRequiredError = errors.New("sessionId required")
	PromptRequiredError  = errors.New("prompt required")
	BadIntervalError     = errors.New("bad interval")
	MaximumJobsError     = errors.New("max 50 loops")
)

type Job struct {
	ID               string  `json:"id"`
	SessionID        string  `json:"sessionId"`
	Prompt           string  `json:"prompt"`
	IntervalSeconds  int     `json:"interval_sec"`
	IntervalLabel    string  `json:"interval_label"`
	WorkingDirectory string  `json:"cwd"`
	CreatedAt        float64 `json:"created_at"`
	Fires            int     `json:"fires"`
	LastFire         float64 `json:"last_fire"`
	LastError        string  `json:"last_error"`
	ExpiresAt        float64 `json:"expires_at"`
}

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
}

type storeFile struct {
	Jobs []Job `json:"jobs"`
}

func New(store string, fire FireFunc) (*Manager, error) {
	store = strings.TrimSpace(store)
	if store == "" {
		return nil, StoreRequiredError
	}
	manager := &Manager{
		store: store,
		fire:  fire,
		jobs:  make(map[string]*Job),
		tasks: make(map[string]context.CancelFunc),
		now:   time.Now,
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

func (manager *Manager) List(sessionID string) []Job {
	sessionID = strings.TrimSpace(sessionID)
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	jobs := make([]Job, 0, len(manager.jobs))
	for _, job := range manager.jobs {
		if sessionID == "" || job.SessionID == sessionID {
			jobs = append(jobs, *job)
		}
	}
	sort.Slice(jobs, func(leftIndex, rightIndex int) bool {
		if jobs[leftIndex].CreatedAt == jobs[rightIndex].CreatedAt {
			return jobs[leftIndex].ID < jobs[rightIndex].ID
		}
		return jobs[leftIndex].CreatedAt < jobs[rightIndex].CreatedAt
	})
	return jobs
}

func (manager *Manager) Create(sessionID, prompt string, intervalSeconds int, intervalLabel, workingDirectory string) (Job, error) {
	sessionID = strings.TrimSpace(sessionID)
	prompt = strings.TrimSpace(prompt)
	if sessionID == "" {
		return Job{}, SessionRequiredError
	}
	if prompt == "" {
		return Job{}, PromptRequiredError
	}
	intervalSeconds, normalizedLabel := NormalizeInterval(intervalSeconds)
	if strings.TrimSpace(intervalLabel) == "" {
		intervalLabel = normalizedLabel
	}

	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if len(manager.jobs) >= MaxJobs {
		return Job{}, MaximumJobsError
	}
	identifier, operationError := manager.newIDLocked()
	if operationError != nil {
		return Job{}, operationError
	}
	now := manager.now()
	job := &Job{
		ID:               identifier,
		SessionID:        sessionID,
		Prompt:           prompt,
		IntervalSeconds:  intervalSeconds,
		IntervalLabel:    strings.TrimSpace(intervalLabel),
		WorkingDirectory: strings.TrimSpace(workingDirectory),
		CreatedAt:        float64(now.UnixNano()) / float64(time.Second),
		ExpiresAt:        float64(now.Add(DefaultExpiration).UnixNano()) / float64(time.Second),
	}
	manager.jobs[identifier] = job
	if operationError := manager.saveLocked(); operationError != nil {
		delete(manager.jobs, identifier)
		return Job{}, operationError
	}
	if manager.running {
		manager.startLocked(identifier, false)
	}
	return *job, nil
}

// Stop removes either one job by ID or all jobs for a session. Job ID takes
// precedence when both selectors are present.
func (manager *Manager) Stop(jobID, sessionID string) ([]Job, error) {
	jobID = strings.TrimSpace(jobID)
	sessionID = strings.TrimSpace(sessionID)
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	removed := []Job{}
	remove := func(identifier string, job *Job) {
		removed = append(removed, *job)
		delete(manager.jobs, identifier)
		if cancel := manager.tasks[identifier]; cancel != nil {
			cancel()
			delete(manager.tasks, identifier)
		}
	}
	if jobID != "" {
		if job := manager.jobs[jobID]; job != nil {
			remove(jobID, job)
		}
	} else if sessionID != "" {
		for identifier, job := range manager.jobs {
			if job.SessionID == sessionID {
				remove(identifier, job)
			}
		}
	}
	if len(removed) > 0 {
		if operationError := manager.saveLocked(); operationError != nil {
			return removed, operationError
		}
	}
	sort.Slice(removed, func(leftIndex, rightIndex int) bool {
		return removed[leftIndex].CreatedAt < removed[rightIndex].CreatedAt
	})
	return removed, nil
}

func ParseInterval(raw string) (seconds int, label string, operationError error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "every"))
	if raw == "" {
		return 0, "", BadIntervalError
	}
	match := intervalPattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return 0, "", BadIntervalError
	}
	quantity, parseError := strconv.ParseInt(match[1], 10, 64)
	if parseError != nil || quantity < 0 {
		return 0, "", BadIntervalError
	}
	unit := match[2]
	multiplier := int64(60)
	if unit != "" {
		switch unit[0] {
		case 's':
			multiplier = 1
		case 'm':
			multiplier = 60
		case 'h':
			multiplier = 60 * 60
		case 'd':
			multiplier = 24 * 60 * 60
		}
	}
	if quantity > int64(MaxInterval) {
		seconds = MaxInterval
	} else {
		seconds = int(quantity * multiplier)
	}
	seconds, label = NormalizeInterval(seconds)
	return seconds, label, nil
}

func NormalizeInterval(seconds int) (int, string) {
	if seconds < MinInterval {
		seconds = MinInterval
	}
	if seconds > MaxInterval {
		seconds = MaxInterval
	}
	switch {
	case seconds >= 24*60*60 && seconds%(24*60*60) == 0:
		return seconds, fmt.Sprintf("%dd", seconds/(24*60*60))
	case seconds >= 60*60 && seconds%(60*60) == 0:
		return seconds, fmt.Sprintf("%dh", seconds/(60*60))
	case seconds%60 == 0:
		return seconds, fmt.Sprintf("%dm", seconds/60)
	default:
		return seconds, fmt.Sprintf("%ds", seconds)
	}
}

var intervalPattern = regexp.MustCompile(`^(\d+)\s*(s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)?$`)

func (manager *Manager) startLocked(identifier string, delayBeforeFirstFire bool) {
	if !manager.running || manager.tasks[identifier] != nil || manager.jobs[identifier] == nil {
		return
	}
	taskContext, cancel := context.WithCancel(manager.operationContext)
	manager.tasks[identifier] = cancel
	manager.waitGroup.Add(1)
	go manager.run(taskContext, identifier, delayBeforeFirstFire)
}

func (manager *Manager) run(operationContext context.Context, identifier string, delayBeforeFirstFire bool) {
	defer manager.waitGroup.Done()
	defer func() {
		manager.mutex.Lock()
		delete(manager.tasks, identifier)
		manager.mutex.Unlock()
	}()
	if delayBeforeFirstFire {
		manager.mutex.Lock()
		job := manager.jobs[identifier]
		interval := time.Duration(0)
		if job != nil {
			interval = time.Duration(job.IntervalSeconds) * time.Second
		}
		manager.mutex.Unlock()
		if interval <= 0 {
			return
		}
		timer := time.NewTimer(interval)
		select {
		case <-operationContext.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	for {
		manager.mutex.Lock()
		job := manager.jobs[identifier]
		if job == nil {
			manager.mutex.Unlock()
			return
		}
		if job.ExpiresAt <= float64(manager.now().UnixNano())/float64(time.Second) {
			delete(manager.jobs, identifier)
			_ = manager.saveLocked()
			manager.mutex.Unlock()
			return
		}
		snapshot := *job
		nextFire := snapshot.Fires + 1
		note := fmt.Sprintf("[REMOTE LOOP · %s · fire %d]\n%s", snapshot.IntervalLabel, nextFire, snapshot.Prompt)
		manager.mutex.Unlock()

		fireContext, cancel := context.WithTimeout(operationContext, fireTimeout)
		var fireError error
		if manager.fire != nil {
			fireError = manager.fire(fireContext, snapshot, note)
		}
		cancel()

		manager.mutex.Lock()
		current := manager.jobs[identifier]
		if current == nil {
			manager.mutex.Unlock()
			return
		}
		if fireError != nil {
			current.LastError = truncate(fireError.Error(), 200)
		} else {
			current.Fires++
			current.LastFire = float64(manager.now().UnixNano()) / float64(time.Second)
			current.LastError = ""
		}
		interval := time.Duration(current.IntervalSeconds) * time.Second
		_ = manager.saveLocked()
		manager.mutex.Unlock()

		timer := time.NewTimer(interval)
		select {
		case <-operationContext.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (manager *Manager) load() error {
	data, operationError := os.ReadFile(manager.store)
	if errors.Is(operationError, os.ErrNotExist) {
		return nil
	}
	if operationError != nil {
		return fmt.Errorf("read loops: %w", operationError)
	}
	var stored storeFile
	if operationError := json.Unmarshal(data, &stored); operationError != nil {
		return fmt.Errorf("decode loops: %w", operationError)
	}
	now := manager.now()
	for _, loaded := range stored.Jobs {
		if len(manager.jobs) >= MaxJobs {
			break
		}
		loaded.ID = strings.TrimSpace(loaded.ID)
		loaded.SessionID = strings.TrimSpace(loaded.SessionID)
		loaded.Prompt = strings.TrimSpace(loaded.Prompt)
		if loaded.ID == "" || loaded.SessionID == "" || loaded.Prompt == "" {
			continue
		}
		var defaultLabel string
		loaded.IntervalSeconds, defaultLabel = NormalizeInterval(loaded.IntervalSeconds)
		if strings.TrimSpace(loaded.IntervalLabel) == "" {
			loaded.IntervalLabel = defaultLabel
		}
		if loaded.CreatedAt <= 0 {
			loaded.CreatedAt = float64(now.UnixNano()) / float64(time.Second)
		}
		if loaded.ExpiresAt <= 0 {
			loaded.ExpiresAt = loaded.CreatedAt + DefaultExpiration.Seconds()
		}
		job := loaded
		manager.jobs[job.ID] = &job
	}
	return nil
}

func (manager *Manager) saveLocked() error {
	jobs := make([]Job, 0, len(manager.jobs))
	for _, job := range manager.jobs {
		jobs = append(jobs, *job)
	}
	sort.Slice(jobs, func(leftIndex, rightIndex int) bool {
		if jobs[leftIndex].CreatedAt == jobs[rightIndex].CreatedAt {
			return jobs[leftIndex].ID < jobs[rightIndex].ID
		}
		return jobs[leftIndex].CreatedAt < jobs[rightIndex].CreatedAt
	})
	if operationError := os.MkdirAll(filepath.Dir(manager.store), 0o700); operationError != nil {
		return fmt.Errorf("create loops directory: %w", operationError)
	}
	temporaryPath, operationError := os.CreateTemp(filepath.Dir(manager.store), ".loops-*.json")
	if operationError != nil {
		return fmt.Errorf("create loops file: %w", operationError)
	}
	temporaryName := temporaryPath.Name()
	defer os.Remove(temporaryName)
	if operationError := temporaryPath.Chmod(0o600); operationError != nil {
		_ = temporaryPath.Close()
		return operationError
	}
	encoder := json.NewEncoder(temporaryPath)
	encoder.SetIndent("", "  ")
	if operationError := encoder.Encode(storeFile{Jobs: jobs}); operationError != nil {
		_ = temporaryPath.Close()
		return fmt.Errorf("encode loops: %w", operationError)
	}
	if operationError := temporaryPath.Close(); operationError != nil {
		return operationError
	}
	if operationError := os.Rename(temporaryName, manager.store); operationError != nil {
		return fmt.Errorf("replace loops: %w", operationError)
	}
	return nil
}

func (manager *Manager) newIDLocked() (string, error) {
	for range 8 {
		var data [5]byte
		if _, operationError := rand.Read(data[:]); operationError != nil {
			return "", fmt.Errorf("generate loop id: %w", operationError)
		}
		identifier := "loop-" + hex.EncodeToString(data[:])
		if manager.jobs[identifier] == nil {
			return identifier, nil
		}
	}
	return "", errors.New("could not generate unique loop id")
}

func truncate(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}
