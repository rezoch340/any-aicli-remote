// Update-file readers. Each mode bounds how much of updates.jsonl is scanned
// and reports the byte window it consumed so the caller can paginate.

package historydata

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

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
