package hub

import (
	"errors"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
)

// Policy contains daemon-configured Hub resource limits and timeouts.
// It is intentionally independent from config so the Hub stays reusable.
type Policy struct {
	ReadBufferBytes         int
	WriteBufferBytes        int
	MaxMessageBytes         int64
	Heartbeat               time.Duration
	ClientReadTimeout       time.Duration
	WatcherEnsureInterval   time.Duration
	StateBroadcastInterval  time.Duration
	EnsureAttempt           time.Duration
	ClientConnectEnsure     time.Duration
	DialAttempts            int
	DialHandshake           time.Duration
	RetryDelay              time.Duration
	WriteTimeout            time.Duration
	ControlWriteTimeout     time.Duration
	PendingLimit            int
	PendingClientLimit      int
	PendingTimeout          time.Duration
	NormalEnsure            time.Duration
	PatientEnsure           time.Duration
	NotificationEnsure      time.Duration
	ReverseOperationTimeout time.Duration
	ReverseReadBytes        int64
	TerminalOutputBytes     int64
	FilesystemPolicy        fsapi.Policy
}

func (policy Policy) Validate() error {
	if validationError := policy.FilesystemPolicy.Validate(); validationError != nil {
		return validationError
	}
	switch {
	case policy.ReadBufferBytes <= 0 || policy.WriteBufferBytes <= 0:
		return errors.New("hub buffers must be positive")
	case policy.MaxMessageBytes <= 0:
		return errors.New("hub maximum message bytes must be positive")
	case policy.Heartbeat <= 0 || policy.ClientReadTimeout <= 0:
		return errors.New("hub client timeouts must be positive")
	case policy.WatcherEnsureInterval <= 0 || policy.StateBroadcastInterval <= 0:
		return errors.New("hub watcher intervals must be positive")
	case policy.EnsureAttempt <= 0 || policy.ClientConnectEnsure <= 0:
		return errors.New("hub ensure timeouts must be positive")
	case policy.DialAttempts <= 0 || policy.DialHandshake <= 0 || policy.RetryDelay <= 0:
		return errors.New("hub dial policy must be positive")
	case policy.WriteTimeout <= 0 || policy.ControlWriteTimeout <= 0:
		return errors.New("hub write timeouts must be positive")
	case policy.PendingLimit <= 0 || policy.PendingClientLimit <= 0 || policy.PendingTimeout <= 0:
		return errors.New("hub pending policy must be positive")
	case policy.PendingClientLimit > policy.PendingLimit:
		return errors.New("hub pending client limit exceeds pending limit")
	case policy.NormalEnsure <= 0 || policy.PatientEnsure <= 0 || policy.NotificationEnsure <= 0:
		return errors.New("hub request ensure timeouts must be positive")
	case policy.ReverseOperationTimeout <= 0 || policy.ReverseReadBytes <= 0:
		return errors.New("hub reverse limits must be positive")
	case policy.TerminalOutputBytes <= 0:
		return errors.New("terminal output bytes must be positive")
	}
	return nil
}
