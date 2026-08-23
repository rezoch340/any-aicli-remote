// Scheduled job records: listing, creation, stop selectors, interval parsing,
// and identifier generation.

package loops

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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
	intervalSeconds, normalizedLabel := manager.policy.NormalizeInterval(intervalSeconds)
	if strings.TrimSpace(intervalLabel) == "" {
		intervalLabel = normalizedLabel
	}

	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if len(manager.jobs) >= manager.policy.MaxJobs {
		return Job{}, fmt.Errorf("%w: max %d", MaximumJobsError, manager.policy.MaxJobs)
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
		ExpiresAt:        float64(now.Add(manager.policy.Retention).UnixNano()) / float64(time.Second),
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

func parseInterval(raw string) (seconds int, operationError error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "every"))
	if raw == "" {
		return 0, BadIntervalError
	}
	match := intervalPattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return 0, BadIntervalError
	}
	quantity, parseError := strconv.ParseInt(match[1], 10, 64)
	if parseError != nil || quantity < 0 {
		return 0, BadIntervalError
	}
	multiplier := int64(secondsPerMinute)
	if match[2] != "" {
		switch match[2][0] {
		case 's':
			multiplier = int64(secondsPerSecond)
		case 'm':
			multiplier = int64(secondsPerMinute)
		case 'h':
			multiplier = int64(secondsPerHour)
		case 'd':
			multiplier = int64(secondsPerDay)
		}
	}
	if quantity > int64(^uint(0)>>1)/multiplier {
		return 0, BadIntervalError
	}
	return int(quantity * multiplier), nil
}

var intervalPattern = regexp.MustCompile(`^(\d+)\s*(s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)?$`)

func (manager *Manager) newIDLocked() (string, error) {
	for range jobIDGenerationAttempts {
		var data [jobIDRandomBytes]byte
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
