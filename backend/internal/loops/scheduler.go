// Job execution timing. Each job owns one goroutine whose cancellation and
// generation are checked before every fire.

package loops

import (
	"context"
	"fmt"
	"time"
)

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

		fireContext, cancel := context.WithTimeout(operationContext, manager.policy.FireTimeout)
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
			current.LastError = truncate(fireError.Error(), manager.policy.LastErrorRunes)
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
