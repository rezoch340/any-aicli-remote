package provider

import (
	"fmt"
	"math"
)

// HistoryPolicy centralizes daemon history limits for every provider adapter.
type HistoryPolicy struct {
	DefaultLimit            int
	LiveLimit               int
	MinLimit                int
	MaxLimit                int
	DefaultMaxBytes         int64
	LiveMaxBytes            int64
	BeforeMaxBytes          int64
	MinMaxBytes             int64
	MaxMaxBytes             int64
	AdapterEventLimit       int
	AdapterReadBytes        int64
	TitleBatchLimit         int
	ChatTextMaxRunes        int
	MessageScanInitialBytes int
	MessageScanMaxBytes     int
	MetadataTitleMaxRunes   int
	MetadataSummaryMaxRunes int
	RenameTitleMaxRunes     int
}

func (policy HistoryPolicy) Validate() error {
	if policy.DefaultLimit < 1 || policy.LiveLimit < 1 || policy.MinLimit < 1 || policy.MaxLimit < policy.MinLimit {
		return fmt.Errorf("invalid history limit policy")
	}
	if policy.DefaultLimit < policy.MinLimit || policy.DefaultLimit > policy.MaxLimit || policy.LiveLimit < policy.MinLimit || policy.LiveLimit > policy.MaxLimit {
		return fmt.Errorf("history default limits outside bounds")
	}
	if policy.DefaultMaxBytes < 1 || policy.LiveMaxBytes < 1 || policy.BeforeMaxBytes < 1 || policy.MinMaxBytes < 1 || policy.MaxMaxBytes < policy.MinMaxBytes {
		return fmt.Errorf("invalid history byte policy")
	}
	for _, value := range []int64{policy.DefaultMaxBytes, policy.LiveMaxBytes, policy.BeforeMaxBytes} {
		if value < policy.MinMaxBytes || value > policy.MaxMaxBytes {
			return fmt.Errorf("history default byte limits outside bounds")
		}
	}
	if policy.AdapterEventLimit < 1 || policy.AdapterReadBytes < 1 || policy.AdapterReadBytes >= math.MaxInt64 || policy.TitleBatchLimit < 1 || policy.ChatTextMaxRunes < 1 || policy.MessageScanInitialBytes < 1 || policy.MessageScanMaxBytes < policy.MessageScanInitialBytes || policy.MetadataTitleMaxRunes < 1 || policy.MetadataSummaryMaxRunes < 1 || policy.RenameTitleMaxRunes < 1 {
		return fmt.Errorf("invalid adapter history policy")
	}
	return nil
}

func (policy HistoryPolicy) NormalizeRequest(live bool, beforeBytes *int64, limit int, maxBytes int64) (int, int64) {
	if limit == 0 {
		if live {
			limit = policy.LiveLimit
		} else {
			limit = policy.DefaultLimit
		}
	}
	if limit < policy.MinLimit {
		limit = policy.MinLimit
	}
	if limit > policy.MaxLimit {
		limit = policy.MaxLimit
	}
	if maxBytes == 0 {
		if live {
			maxBytes = policy.LiveMaxBytes
		} else if beforeBytes != nil {
			maxBytes = policy.BeforeMaxBytes
		} else {
			maxBytes = policy.DefaultMaxBytes
		}
	}
	if maxBytes < policy.MinMaxBytes {
		maxBytes = policy.MinMaxBytes
	}
	if maxBytes > policy.MaxMaxBytes {
		maxBytes = policy.MaxMaxBytes
	}
	return limit, maxBytes
}
