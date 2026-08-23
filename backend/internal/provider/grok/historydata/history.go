// Package historydata reads Grok session update files.
//
// This file owns the read entry point and its options. The per-mode readers
// live in readers.go, byte-window scanning in scan.go, and event normalization
// in normalization.go.
package historydata

import (
	"os"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

const (
	// Grok JSONL backward-scan algorithm invariants preserve parser behavior.
	backwardMaximumScanBytes  int64 = 64_000_000
	backwardMinimumScanBytes  int64 = 16_000_000
	backwardReadMultiplier    int64 = 16
	backwardEventByteEstimate int64 = 800_000
	backwardInitialScanBytes  int64 = 1_200_000
	backwardEventScanEstimate int64 = 80_000
	backwardScanGrowthBytes   int64 = 4_000_000
	backwardEventBufferFactor       = 4
	backwardScanGrowthFactor        = 2
	chatBudgetNumerator             = 3
	chatBudgetDenominator           = 4
	chatBudgetReserve               = 20
)

// ReadOptions matches server.py's read_session_updates keyword arguments.
type ReadOptions struct {
	Limit       int
	MaxBytes    int64
	SinceBytes  int64
	Live        bool
	BeforeBytes *int64
	ChatOnly    bool
}

// Meta is JSON-ready history metadata.
type Meta map[string]any

// Event is a JSON-RPC session/update event, with private scan fields stripped before return.
type Event map[string]any

// ReadSessionUpdatesFromRoot reads updates.jsonl through a caller-pinned
// session root. A symbolic link cannot redirect history into another session.
func ReadSessionUpdatesFromRoot(sessionRoot *os.Root, displayPath string, historyPolicy providerapi.HistoryPolicy, options ReadOptions) ([]Event, Meta, error) {
	if operationError := historyPolicy.Validate(); operationError != nil {
		return nil, nil, operationError
	}
	if options.Limit == 0 {
		options.Limit = historyPolicy.AdapterEventLimit
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = historyPolicy.AdapterReadBytes
	}
	path := displayPath
	fileInfo, operationError := sessionRoot.Lstat("updates.jsonl")
	if operationError != nil {
		return []Event{}, Meta{"path": path, "missing": true, "size": int64(0), "has_more": false}, nil
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
		return []Event{}, Meta{"path": path, "error": "updates.jsonl must be a regular file", "size": int64(0), "has_more": false}, nil
	}
	updatesFile, operationError := sessionRoot.Open("updates.jsonl")
	if operationError != nil {
		return []Event{}, Meta{"path": path, "missing": true, "size": int64(0), "has_more": false}, nil
	}
	defer updatesFile.Close()
	openedInfo, operationError := updatesFile.Stat()
	if operationError != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(fileInfo, openedInfo) {
		return []Event{}, Meta{"path": path, "error": "updates.jsonl changed while opening", "size": int64(0), "has_more": false}, nil
	}
	size := openedInfo.Size()
	since := options.SinceBytes
	if since < 0 {
		since = 0
	}
	if since > size {
		since = 0
	}
	limit := options.Limit
	if limit < 1 {
		limit = 1
	}
	maxBytes := options.MaxBytes
	if maxBytes < 1 {
		maxBytes = historyPolicy.AdapterReadBytes
	}
	if options.Live && since >= size {
		return []Event{}, Meta{"path": path, "size": size, "returned": 0, "scanned": 0, "since": since, "live": true, "has_more": false, "end": size}, nil
	}

	if options.Live {
		events, metadata := readLive(updatesFile, path, size, since, limit, maxBytes, historyPolicy.ChatTextMaxRunes)
		return events, metadata, nil
	}

	endCap := size
	if options.BeforeBytes != nil {
		endCap = *options.BeforeBytes
		if endCap < 0 {
			endCap = 0
		}
		if endCap > size {
			endCap = size
		}
	}
	if endCap <= 0 {
		return []Event{}, Meta{"path": path, "size": size, "has_more": false, "window_start": 0, "window_end": 0, "returned": 0, "live": false}, nil
	}
	if options.ChatOnly {
		events, metadata := readChatOnly(updatesFile, path, size, endCap, limit, maxBytes, historyPolicy.ChatTextMaxRunes)
		return events, metadata, nil
	}
	events, metadata := readFull(updatesFile, path, size, endCap, limit, maxBytes, options.ChatOnly, historyPolicy.ChatTextMaxRunes)
	return events, metadata, nil
}
