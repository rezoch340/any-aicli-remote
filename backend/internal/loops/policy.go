package loops

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	secondsPerSecond = 1
	secondsPerMinute = 60
	secondsPerHour   = 60 * secondsPerMinute
	secondsPerDay    = 24 * secondsPerHour
)

type Policy struct {
	MinInterval     time.Duration
	MaxInterval     time.Duration
	DefaultInterval time.Duration
	Retention       time.Duration
	MaxJobs         int
	FireTimeout     time.Duration
	LastErrorRunes  int
}

func (policy Policy) Validate() error {
	if policy.MinInterval <= 0 || policy.MaxInterval <= 0 || policy.DefaultInterval <= 0 || policy.Retention <= 0 || policy.MaxJobs <= 0 || policy.FireTimeout <= 0 || policy.LastErrorRunes <= 0 {
		return errors.New("loop policy values must be positive")
	}
	if policy.MinInterval > policy.DefaultInterval || policy.DefaultInterval > policy.MaxInterval {
		return errors.New("loop interval policy order invalid")
	}
	if policy.MinInterval%time.Second != 0 || policy.MaxInterval%time.Second != 0 || policy.DefaultInterval%time.Second != 0 {
		return errors.New("loop intervals must be whole seconds")
	}
	maxInteger := int64(^uint(0) >> 1)
	if strconv.IntSize == 32 {
		maxInteger = int64(^uint32(0) >> 1)
	}
	if policy.MaxInterval/time.Second > time.Duration(maxInteger) {
		return errors.New("maximum loop interval exceeds platform integer range")
	}
	return nil
}

func (policy Policy) minSeconds() int     { return int(policy.MinInterval / time.Second) }
func (policy Policy) maxSeconds() int     { return int(policy.MaxInterval / time.Second) }
func (policy Policy) defaultSeconds() int { return int(policy.DefaultInterval / time.Second) }
func (policy Policy) NormalizeInterval(seconds int) (int, string) {
	if seconds < policy.minSeconds() {
		seconds = policy.minSeconds()
	}
	if seconds > policy.maxSeconds() {
		seconds = policy.maxSeconds()
	}
	return formatInterval(seconds)
}
func (policy Policy) ParseInterval(raw string) (int, string, error) {
	seconds, parseError := parseInterval(raw)
	if parseError != nil {
		return 0, "", parseError
	}
	normalizedSeconds, normalizedLabel := policy.NormalizeInterval(seconds)
	return normalizedSeconds, normalizedLabel, nil
}
func formatInterval(seconds int) (int, string) {
	switch {
	case seconds >= secondsPerDay && seconds%secondsPerDay == 0:
		return seconds, fmt.Sprintf("%dd", seconds/secondsPerDay)
	case seconds >= secondsPerHour && seconds%secondsPerHour == 0:
		return seconds, fmt.Sprintf("%dh", seconds/secondsPerHour)
	case seconds%secondsPerMinute == 0:
		return seconds, fmt.Sprintf("%dm", seconds/secondsPerMinute)
	default:
		return seconds, fmt.Sprintf("%ds", seconds)
	}
}
