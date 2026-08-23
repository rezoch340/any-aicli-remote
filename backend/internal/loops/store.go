// Job persistence. The store is rewritten atomically so a crash cannot leave a
// partially written schedule.

package loops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
)

type storeFile struct {
	Jobs []Job `json:"jobs"`
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
		if len(manager.jobs) >= manager.policy.MaxJobs {
			break
		}
		loaded.ID = strings.TrimSpace(loaded.ID)
		loaded.SessionID = strings.TrimSpace(loaded.SessionID)
		loaded.Prompt = strings.TrimSpace(loaded.Prompt)
		if loaded.ID == "" || loaded.SessionID == "" || loaded.Prompt == "" {
			continue
		}
		var defaultLabel string
		loaded.IntervalSeconds, defaultLabel = manager.policy.NormalizeInterval(loaded.IntervalSeconds)
		if strings.TrimSpace(loaded.IntervalLabel) == "" {
			loaded.IntervalLabel = defaultLabel
		}
		if loaded.CreatedAt <= 0 {
			loaded.CreatedAt = float64(now.UnixNano()) / float64(time.Second)
		}
		if loaded.ExpiresAt <= 0 {
			loaded.ExpiresAt = loaded.CreatedAt + manager.policy.Retention.Seconds()
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
	data, operationError := json.MarshalIndent(storeFile{Jobs: jobs}, "", "  ")
	if operationError != nil {
		return fmt.Errorf("encode loops: %w", operationError)
	}
	if operationError := atomicfile.WritePrivate(manager.store, append(data, '\n')); operationError != nil {
		return fmt.Errorf("replace loops: %w", operationError)
	}
	return nil
}
