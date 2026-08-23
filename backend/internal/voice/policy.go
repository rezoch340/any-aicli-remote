package voice

import (
	"fmt"
	"math"
	"time"
)

// Policy centralizes provider-neutral speech synthesis resource limits.
type Policy struct {
	RequestTimeout      time.Duration
	TextMaxRunes        int
	TruncatedTextRunes  int
	SuccessBodyMaxBytes int64
	ErrorBodyMaxBytes   int64
	ErrorBodyMaxRunes   int
}

func (policy Policy) Validate() error {
	if policy.RequestTimeout <= 0 || policy.TextMaxRunes < 1 || policy.TruncatedTextRunes < 1 || policy.SuccessBodyMaxBytes < 1 || policy.ErrorBodyMaxBytes < 1 || policy.ErrorBodyMaxRunes < 1 {
		return fmt.Errorf("invalid voice policy")
	}
	if policy.TruncatedTextRunes >= policy.TextMaxRunes || policy.SuccessBodyMaxBytes >= math.MaxInt64 || policy.ErrorBodyMaxBytes >= math.MaxInt64 {
		return fmt.Errorf("invalid voice policy limits")
	}
	return nil
}
