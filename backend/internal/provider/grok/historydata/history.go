package historydata

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

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

func readLive(updatesFile *os.File, path string, size, since int64, limit int, maxBytes int64, chatTextMaxRunes int) ([]Event, Meta) {
	windowStart := since
	minimumStart := max64(0, size-maxBytes)
	clamped := windowStart <= 0 || windowStart < minimumStart || windowStart > size
	if clamped {
		windowStart = minimumStart
	}
	if clamped && windowStart > 0 {
		windowStart = advanceAfterLine(updatesFile, windowStart, size)
	}
	blob, operationError := readRange(updatesFile, windowStart, size, maxBytes)
	if operationError != nil {
		return []Event{}, Meta{"path": path, "error": operationError.Error(), "size": size, "has_more": false}
	}
	if cut := bytes.LastIndexByte(blob, '\n') + 1; cut > 0 && cut < len(blob) {
		blob = blob[:cut]
	}
	endPos := windowStart + int64(len(blob))
	var scored []Event
	for _, line := range bytes.Split(blob, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		if !containsAnyBytes(line, [][]byte{[]byte("user_message_chunk"), []byte("agent_message_chunk"), []byte("agent_thought_chunk"), []byte("tool_call"), []byte("turn_completed"), []byte("task_completed"), []byte("plan"), []byte("session_recap"), []byte("subagent_spawned"), []byte("subagent_progress"), []byte("subagent_finished")}) {
			continue
		}
		event := parseUpdateLine(string(bytes.TrimRight(line, "\r")), true)
		if event != nil {
			scored = append(scored, event)
		}
	}
	if len(scored) > limit {
		scored = scored[len(scored)-limit:]
	}
	return stripAndTrim(scored, chatTextMaxRunes), Meta{"path": path, "size": size, "end": endPos, "returned": len(scored), "scanned": len(scored), "since": since, "live": true, "has_more": false, "window_start": windowStart}
}

func readChatOnly(updatesFile *os.File, path string, size, endCap int64, limit int, maxBytes int64, chatTextMaxRunes int) ([]Event, Meta) {
	want := maxInt(1, limit)
	maxScan := min64(backwardMaximumScanBytes, max64(backwardMinimumScanBytes, max64(maxBytes*backwardReadMultiplier, int64(want)*backwardEventByteEstimate)))
	scan := min64(maxScan, max64(backwardInitialScanBytes, int64(want)*backwardEventScanEstimate))
	var acc []Event
	readTo := endCap
	endPos := endCap
	windowStart := endCap
	start := endCap
	scannedLines := 0

	for {
		start = max64(0, endCap-scan)
		windowStart = start
		if start > 0 {
			windowStart = advanceAfterLine(updatesFile, start, readTo)
		}
		if windowStart < readTo {
			blob, operationError := readRange(updatesFile, windowStart, readTo, scan)
			if operationError != nil {
				return []Event{}, Meta{"path": path, "error": operationError.Error(), "size": size, "has_more": false}
			}
			if readTo == endCap {
				endPos = windowStart + int64(len(blob))
			}
			lines := splitLinesKeepEnds(blob)
			scannedLines += len(lines)
			off := windowStart + int64(len(blob))
			stop := false
			for itemIndex := len(lines) - 1; itemIndex >= 0; itemIndex-- {
				rawLine := lines[itemIndex]
				off -= int64(len(rawLine))
				if !bytes.Contains(rawLine, []byte("user_message_chunk")) && !bytes.Contains(rawLine, []byte("agent_message_chunk")) {
					continue
				}
				scanner := strings.TrimRight(string(rawLine), "\r\n")
				if strings.TrimSpace(scanner) == "" {
					continue
				}
				event := parseUpdateLine(scanner, false)
				if event == nil || !isMessageKind(toString(event["_kind"])) {
					continue
				}
				event["_off"] = off
				acc = append(acc, event)
				if len(acc) >= want*backwardEventBufferFactor {
					stop = true
					break
				}
			}
			readTo = windowStart
			if stop {
				// Match server.py's loop condition below rather than scanning more.
			}
		}

		groups := 0
		lastKind := ""
		for itemIndex := len(acc) - 1; itemIndex >= 0; itemIndex-- {
			kind := toString(acc[itemIndex]["_kind"])
			if !isMessageKind(kind) || lastKind == "" || lastKind != kind {
				groups++
			}
			lastKind = kind
		}
		if groups >= want || start <= 0 || scan >= maxScan || len(acc) >= want*backwardEventBufferFactor {
			break
		}
		scan = min64(maxScan, max64(scan*backwardScanGrowthFactor, scan+backwardScanGrowthBytes))
	}

	chron := reverseEvents(acc)
	coalesced := coalesceChat(chron)
	firstOff := windowStart
	if len(coalesced) > 0 {
		firstOff = eventOffset(coalesced[0], windowStart)
	}
	collected := coalesced
	if len(collected) > want {
		collected = collected[len(collected)-want:]
	}
	if len(collected) > 0 {
		firstOff = eventOffset(collected[0], firstOff)
	}
	hasMore := firstOff > 0 && (start > 0 || len(collected) >= want)
	return stripAndTrim(collected, chatTextMaxRunes), Meta{"path": path, "size": size, "returned": len(collected), "scanned": scannedLines, "live": false, "has_more": hasMore, "window_start": firstOff, "window_end": endPos, "end": endPos, "older_before": firstOff, "chat_only": true}
}

func readFull(updatesFile *os.File, path string, size, endCap int64, limit int, maxBytes int64, chatOnly bool, chatTextMaxRunes int) ([]Event, Meta) {
	start := max64(0, endCap-maxBytes)
	windowStart := start
	if start > 0 {
		windowStart = advanceAfterLine(updatesFile, start, endCap)
	}
	blob, operationError := readRange(updatesFile, windowStart, endCap, maxBytes)
	if operationError != nil {
		return []Event{}, Meta{"path": path, "error": operationError.Error(), "size": size, "has_more": false}
	}
	endPos := windowStart + int64(len(blob))
	var scored []Event
	lines := splitLinesKeepEnds(blob)
	off := windowStart
	for _, line := range lines {
		lineStart := off
		off += int64(len(line))
		if !bytes.Contains(line, []byte("sessionUpdate")) && !bytes.Contains(line, []byte("session_update")) {
			continue
		}
		scanner := strings.TrimRight(string(line), "\r\n")
		if strings.TrimSpace(scanner) == "" {
			continue
		}
		event := parseUpdateLine(scanner, false)
		if event != nil {
			event["_off"] = lineStart
			scored = append(scored, event)
		}
	}
	if len(scored) == 0 {
		return []Event{}, Meta{"path": path, "size": size, "returned": 0, "scanned": 0, "live": false, "has_more": windowStart > 0, "window_start": windowStart, "window_end": endPos, "end": endPos}
	}

	trimmed := len(scored) > limit
	kept := scored
	if trimmed {
		chatKinds := map[string]struct{}{
			"user_message_chunk":  {},
			"agent_message_chunk": {},
			"agent_thought_chunk": {},
			"plan":                {},
			"session_recap":       {},
			"turn_completed":      {},
			"task_completed":      {},
		}
		var chatIndexes, toolIndexes []int
		for itemIndex, event := range scored {
			if _, valid := chatKinds[toString(event["_kind"])]; valid {
				chatIndexes = append(chatIndexes, itemIndex)
			} else {
				toolIndexes = append(toolIndexes, itemIndex)
			}
		}
		chatBudget := maxInt(limit*chatBudgetNumerator/chatBudgetDenominator, maxInt(1, limit-chatBudgetReserve))
		keep := map[int]struct{}{}
		for _, itemIndex := range lastInts(chatIndexes, chatBudget) {
			keep[itemIndex] = struct{}{}
		}
		room := limit - len(keep)
		if room > 0 {
			for _, itemIndex := range lastInts(toolIndexes, room) {
				keep[itemIndex] = struct{}{}
			}
		}
		kept = make([]Event, 0, len(keep))
		for itemIndex, event := range scored {
			if _, valid := keep[itemIndex]; valid {
				kept = append(kept, event)
			}
		}
	}
	firstOff := windowStart
	if len(kept) > 0 {
		firstOff = eventOffset(kept[0], windowStart)
	}
	events := stripAndTrim(kept, chatTextMaxRunes)
	hasMore := windowStart > 0 || trimmed
	return events, Meta{"path": path, "size": size, "returned": len(events), "scanned": len(scored), "live": false, "has_more": hasMore, "window_start": firstOff, "window_end": endPos, "end": endPos, "older_before": firstOff, "chat_only": chatOnly}
}

func eventOffset(event Event, fallback int64) int64 {
	switch value := event["_off"].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		itemIndex, operationError := value.Int64()
		if operationError == nil {
			return itemIndex
		}
	}
	return fallback
}

func reverseEvents(input []Event) []Event {
	out := make([]Event, len(input))
	for itemIndex := range input {
		out[len(input)-1-itemIndex] = input[itemIndex]
	}
	return out
}

func lastInts(input []int, count int) []int {
	if count <= 0 || len(input) == 0 {
		return nil
	}
	if len(input) <= count {
		return input
	}
	return input[len(input)-count:]
}

func splitLinesKeepEnds(blob []byte) [][]byte {
	if len(blob) == 0 {
		return nil
	}
	var lines [][]byte
	start := 0
	for itemIndex, byteValue := range blob {
		if byteValue == '\n' {
			lines = append(lines, blob[start:itemIndex+1])
			start = itemIndex + 1
		}
	}
	if start < len(blob) {
		lines = append(lines, blob[start:])
	}
	return lines
}

func readRange(updatesFile *os.File, start, end, maxBytes int64) ([]byte, error) {
	if start < 0 || end < start || maxBytes <= 0 {
		return nil, errors.New("invalid byte range")
	}
	if end-start > maxBytes {
		return nil, errors.New("byte range exceeds maximum")
	}
	buffer := make([]byte, end-start)
	readLength, operationError := updatesFile.ReadAt(buffer, start)
	if operationError != nil && operationError != io.EOF {
		return nil, operationError
	}
	return buffer[:readLength], nil
}

func advanceAfterLine(updatesFile *os.File, start, end int64) int64 {
	if start <= 0 || start >= end {
		return start
	}
	remainingBytes := bufio.NewReader(io.NewSectionReader(updatesFile, start, end-start))
	line, operationError := remainingBytes.ReadBytes('\n')
	if operationError != nil && len(line) == 0 {
		return end
	}
	return start + int64(len(line))
}

func containsAnyBytes(haystack []byte, needles [][]byte) bool {
	for _, needle := range needles {
		if bytes.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func maxInt(leftValue, rightValue int) int {
	if leftValue > rightValue {
		return leftValue
	}
	return rightValue
}

func min64(leftValue, rightValue int64) int64 {
	if leftValue < rightValue {
		return leftValue
	}
	return rightValue
}

func max64(leftValue, rightValue int64) int64 {
	if leftValue > rightValue {
		return leftValue
	}
	return rightValue
}
