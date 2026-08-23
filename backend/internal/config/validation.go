// Configuration validation: schema version, required fields, secret-bearing
// provider options, and tuning invariants.

package config

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

func ValidateDocument(document Document) error {
	if document.Version != DocumentVersion {
		return errors.New("invalid config version")
	}
	if strings.TrimSpace(document.Network.Bind) == "" || strings.TrimSpace(document.Agent.Host) == "" {
		return errors.New("bind and agent host are required")
	}
	if document.Network.Port < 1 || document.Network.Port > 65535 || document.Agent.Port < 1 || document.Agent.Port > 65535 || document.Network.Port == document.Agent.Port {
		return errors.New("ports must be distinct values between 1 and 65535")
	}
	if strings.TrimSpace(document.Storage.DataDirectory) == "" || strings.TrimSpace(document.Storage.RuntimeDirectory) == "" {
		return errors.New("storage directories are required")
	}
	if strings.TrimSpace(document.Provider.ID) == "" {
		return errors.New("provider id is required")
	}
	if operationError := validateOptions(document.Provider.Options); operationError != nil {
		return operationError
	}
	return validateTuning(document.Tuning)
}
func validateOptions(options map[string]string) error {
	for key := range options {
		lowered := strings.ToLower(key)
		for _, word := range []string{"secret", "token", "password", "credential", "api_key", "key"} {
			if strings.Contains(lowered, word) {
				return fmt.Errorf("provider option %q may contain secret material", key)
			}
		}
	}
	return nil
}
func validateTuning(tuning TuningDocument) error {
	if operationError := validatePositiveTuning(reflect.ValueOf(tuning)); operationError != nil {
		return operationError
	}
	if tuning.History.MinLimit > tuning.History.DefaultLimit || tuning.History.DefaultLimit > tuning.History.MaxLimit || tuning.History.MinLimit > tuning.History.LiveLimit || tuning.History.LiveLimit > tuning.History.MaxLimit {
		return errors.New("history limits are inconsistent")
	}
	if tuning.Room.CompactionRetainMessages > tuning.Room.CompactionThreshold || tuning.Room.FeedDefaultLimit > tuning.Room.FeedMaxLimit || tuning.Room.ScannerInitialBytes > tuning.Room.ScannerMaxBytes {
		return errors.New("room limits are inconsistent")
	}
	if tuning.Git.LogDefaultLimit > tuning.Git.LogMaxLimit {
		return errors.New("git log limits are inconsistent")
	}
	if tuning.Filesystem.MaxReadBytes >= math.MaxInt64 || tuning.Filesystem.MaxWriteBytes >= math.MaxInt64 || tuning.Filesystem.MaxListItems >= int(^uint(0)>>1) {
		return errors.New("filesystem limits must leave room for sentinels")
	}
	if tuning.Skills.MaxFileBytes >= math.MaxInt64 {
		return errors.New("skills file limit must leave room for sentinel")
	}
	if tuning.Git.ContextFileReadBytes >= math.MaxInt64 || tuning.Git.CommandOutputMaxBytes >= math.MaxInt64 {
		return errors.New("git context read limit overflows sentinel")
	}
	if tuning.Hub.PendingClientLimit > tuning.Hub.PendingLimit {
		return errors.New("pending client limit exceeds pending limit")
	}
	if tuning.History.MessageScanInitialBytes > tuning.History.MessageScanMaxBytes {
		return errors.New("history message scanner limits are inconsistent")
	}
	if tuning.Voice.TruncatedTextRunes >= tuning.Voice.TextMaxRunes || tuning.Voice.SuccessBodyMaxBytes >= math.MaxInt64 || tuning.Voice.ErrorBodyMaxBytes >= math.MaxInt64 {
		return errors.New("voice limits are inconsistent")
	}
	if tuning.History.MinMaxBytes > tuning.History.DefaultMaxBytes || tuning.History.DefaultMaxBytes > tuning.History.MaxMaxBytes || tuning.History.MinMaxBytes > tuning.History.LiveMaxBytes || tuning.History.LiveMaxBytes > tuning.History.MaxMaxBytes || tuning.History.MinMaxBytes > tuning.History.BeforeMaxBytes || tuning.History.BeforeMaxBytes > tuning.History.MaxMaxBytes {
		return errors.New("history byte limits are inconsistent")
	}
	if tuning.Loops.MinInterval.Duration > tuning.Loops.DefaultInterval.Duration || tuning.Loops.DefaultInterval.Duration > tuning.Loops.MaxInterval.Duration {
		return errors.New("loop intervals are inconsistent")
	}
	if tuning.Loops.MinInterval.Duration%time.Second != 0 || tuning.Loops.DefaultInterval.Duration%time.Second != 0 || tuning.Loops.MaxInterval.Duration%time.Second != 0 {
		return errors.New("loop intervals must be whole seconds")
	}
	cookieSeconds := int64(tuning.HTTP.AuthenticationCookieMaxAge.Duration / time.Second)
	maximumInt := int64(^uint(0) >> 1)
	if tuning.HTTP.AuthenticationCookieMaxAge.Duration%time.Second != 0 || cookieSeconds > maximumInt {
		return errors.New("authentication cookie max age must be whole seconds within platform int range")
	}
	return nil
}

func validatePositiveTuning(value reflect.Value) error {
	if value.Kind() == reflect.Struct {
		for fieldIndex := 0; fieldIndex < value.NumField(); fieldIndex++ {
			if operationError := validatePositiveTuning(value.Field(fieldIndex)); operationError != nil {
				return operationError
			}
		}
		return nil
	}
	if value.Kind() == reflect.Int || value.Kind() == reflect.Int64 {
		if value.Int() <= 0 {
			return errors.New("tuning values must be positive")
		}
	}
	return nil
}
