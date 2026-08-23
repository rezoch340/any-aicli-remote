// Pending forwarded requests. Every pending entry is removed exactly once,
// whether it completes, expires, or its client disconnects.

package hub

import (
	"context"
	"time"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type pendingRequest struct {
	client        *clientConnection
	original      any
	prepared      providerapi.PreparedRequest
	detached      bool
	timeoutCancel context.CancelFunc
}

// removePendingLocked removes one forwarded request and stops its expiry timer.
// The caller must hold stateMutex.
func (hubInstance *Hub) removePendingLocked(identifier int64) (pendingRequest, bool) {
	pending, present := hubInstance.pending[identifier]
	if !present {
		return pendingRequest{}, false
	}
	delete(hubInstance.pending, identifier)
	if pending.timeoutCancel != nil {
		pending.timeoutCancel()
	}
	return pending, true
}

func (hubInstance *Hub) armPendingTimeout(identifier int64) {
	timeout := hubInstance.pendingTimeout
	timeoutContext, timeoutCancel := context.WithCancel(hubInstance.lifetimeContext)
	hubInstance.stateMutex.Lock()
	pending, present := hubInstance.pending[identifier]
	if present {
		pending.timeoutCancel = timeoutCancel
		hubInstance.pending[identifier] = pending
		hubInstance.pendingWait.Add(1)
	}
	hubInstance.stateMutex.Unlock()
	if !present {
		timeoutCancel()
		return
	}
	go func() {
		defer hubInstance.pendingWait.Done()
		defer timeoutCancel()
		timeoutTimer := time.NewTimer(timeout)
		defer timeoutTimer.Stop()
		select {
		case <-timeoutContext.Done():
			return
		case <-timeoutTimer.C:
			hubInstance.expirePendingRequest(identifier)
		}
	}()
}

func (hubInstance *Hub) expirePendingRequest(identifier int64) {
	hubInstance.stateMutex.Lock()
	pending, present := hubInstance.removePendingLocked(identifier)
	hubInstance.stateMutex.Unlock()
	if !present {
		return
	}
	message := "provider request timed out"
	if pending.client != nil && !pending.detached && !pending.client.closed.Load() {
		hubInstance.sendRPCError(pending.client, pending.original, message, agentUnavailableErrorCode)
		return
	}
	hubInstance.broadcastNotification(providerapi.DetachedRequestNotification, map[string]any{
		"id": pending.original, "ok": false, "detached": true, "error": message,
	})
}
