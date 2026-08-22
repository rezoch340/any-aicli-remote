package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultLimit    = 1600
	defaultMaxBytes = 8_000_000
	chatTextCap     = 120_000
)

var historyKeep = map[string]struct{}{
	"user_message_chunk":        {},
	"agent_message_chunk":       {},
	"agent_thought_chunk":       {},
	"tool_call":                 {},
	"tool_call_update":          {},
	"plan":                      {},
	"session_recap":             {},
	"turn_completed":            {},
	"task_completed":            {},
	"available_commands_update": {},
}

var terminalToolStatuses = map[string]struct{}{
	"completed": {},
	"failed":    {},
	"cancelled": {},
	"error":     {},
	"done":      {},
	"success":   {},
}

// Store reads Grok's on-disk session history. A Store is safe for concurrent use.
type Store struct {
	Root string

	mutex          sync.Mutex
	index          map[string][]string
	indexAt        time.Time
	directoryCache map[cacheKey]cacheEntry
	now            func() time.Time
}

type cacheKey struct {
	sessionID        string
	workingDirectory string
}

type cacheEntry struct {
	path string
	exp  time.Time
}

// SessionInfo mirrors the small summary payload used by grok-remote's history endpoint.
type SessionInfo struct {
	Title            string `json:"title"`
	WorkingDirectory string `json:"cwd"`
}

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

// NewStore returns a history Store rooted at ~/.grok/sessions when root is empty.
func NewStore(root string) *Store {
	if strings.TrimSpace(root) == "" {
		root = defaultSessionsRoot()
	}
	return &Store{Root: root, directoryCache: map[cacheKey]cacheEntry{}, now: time.Now}
}

func defaultSessionsRoot() string {
	home, operationError := os.UserHomeDir()
	if operationError != nil || home == "" {
		return filepath.Join(".grok", "sessions")
	}
	return filepath.Join(home, ".grok", "sessions")
}

// FindSessionDirectory resolves a session id to its directory, honoring cwd before falling back to an indexed sid lookup.
func (store *Store) FindSessionDirectory(sessionID, workingDirectory string) (string, bool) {
	if store == nil {
		store = NewStore("")
	}
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" || strings.Contains(normalizedSessionID, "..") || strings.ContainsAny(normalizedSessionID, `/\`) {
		return "", false
	}
	root := store.Root
	if fileInfo, operationError := os.Stat(root); operationError != nil || !fileInfo.IsDir() {
		return "", false
	}

	cacheLookupKey := cacheKey{sessionID: normalizedSessionID, workingDirectory: workingDirectory}
	if path, valid := store.cacheHit(cacheLookupKey); valid {
		return path, true
	}

	if strings.TrimSpace(workingDirectory) != "" {
		for _, variant := range workingDirectoryVariants(workingDirectory) {
			enc := EncodeSessionWorkingDirectory(variant)
			if path := filepath.Join(root, enc, normalizedSessionID); sessionDirectoryExists(path) {
				store.cacheSet(cacheLookupKey, path)
				return path, true
			}
			enc2 := EncodeSessionWorkingDirectory(encodeSecondVariant(variant))
			if enc2 != enc {
				if path := filepath.Join(root, enc2, normalizedSessionID); sessionDirectoryExists(path) {
					store.cacheSet(cacheLookupKey, path)
					return path, true
				}
			}
		}
	}

	hits := append([]string(nil), store.sessionIdentifierIndex(false)[normalizedSessionID]...)
	if len(hits) == 0 {
		store.mutex.Lock()
		staleEnough := store.nowFn().Sub(store.indexAt) > 5*time.Second
		store.mutex.Unlock()
		if staleEnough {
			hits = append([]string(nil), store.sessionIdentifierIndex(true)[normalizedSessionID]...)
		}
	}
	hits = filterSessionDirectories(hits)
	if len(hits) == 1 {
		store.cacheSet(cacheLookupKey, hits[0])
		return hits[0], true
	}
	if len(hits) > 1 {
		if strings.TrimSpace(workingDirectory) != "" {
			workingDirectoryMatches := matchWorkingDirectoryHits(hits, workingDirectory)
			if len(workingDirectoryMatches) == 1 {
				store.cacheSet(cacheLookupKey, workingDirectoryMatches[0])
				return workingDirectoryMatches[0], true
			}
			if len(workingDirectoryMatches) > 0 {
				hits = workingDirectoryMatches
			}
		}
		sort.SliceStable(hits, func(itemIndex, comparisonIndex int) bool {
			return updateMTime(hits[itemIndex]).After(updateMTime(hits[comparisonIndex]))
		})
		store.cacheSet(cacheLookupKey, hits[0])
		return hits[0], true
	}
	return "", false
}

func (store *Store) cacheHit(key cacheKey) (string, bool) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.directoryCache == nil {
		store.directoryCache = map[cacheKey]cacheEntry{}
	}
	hit, valid := store.directoryCache[key]
	if !valid {
		return "", false
	}
	if store.nowFn().Before(hit.exp) {
		if sessionDirectoryExists(hit.path) {
			return hit.path, true
		}
		return "", false
	}
	delete(store.directoryCache, key)
	return "", false
}

func (store *Store) cacheSet(key cacheKey, path string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.directoryCache == nil {
		store.directoryCache = map[cacheKey]cacheEntry{}
	}
	store.directoryCache[key] = cacheEntry{path: path, exp: store.nowFn().Add(90 * time.Second)}
}

func (store *Store) nowFn() time.Time {
	if store.now != nil {
		return store.now()
	}
	return time.Now()
}

func sessionDirectoryExists(path string) bool {
	fileInfo, operationError := os.Stat(path)
	if operationError != nil || !fileInfo.IsDir() {
		return false
	}
	if _, operationError := os.Stat(filepath.Join(path, "updates.jsonl")); operationError == nil {
		return true
	}
	if _, operationError := os.Stat(filepath.Join(path, "summary.json")); operationError == nil {
		return true
	}
	return false
}

func filterSessionDirectories(paths []string) []string {
	out := paths[:0]
	for _, path := range paths {
		if sessionDirectoryExists(path) {
			out = append(out, path)
		}
	}
	return out
}

func (store *Store) sessionIdentifierIndex(force bool) map[string][]string {
	store.mutex.Lock()
	if !force && store.index != nil && store.nowFn().Sub(store.indexAt) <= 60*time.Second {
		itemIndex := store.index
		store.mutex.Unlock()
		return itemIndex
	}
	store.mutex.Unlock()

	itemIndex := map[string][]string{}
	entries, operationError := os.ReadDir(store.Root)
	if operationError == nil {
		for _, parent := range entries {
			if !parent.IsDir() {
				continue
			}
			children, operationError := os.ReadDir(filepath.Join(store.Root, parent.Name()))
			if operationError != nil {
				continue
			}
			for _, child := range children {
				if !child.IsDir() {
					continue
				}
				name := child.Name()
				if strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
					continue
				}
				path := filepath.Join(store.Root, parent.Name(), name)
				if sessionDirectoryExists(path) {
					itemIndex[name] = append(itemIndex[name], path)
				}
			}
		}
	}

	store.mutex.Lock()
	store.index = itemIndex
	store.indexAt = store.nowFn()
	store.mutex.Unlock()
	return itemIndex
}

func workingDirectoryVariants(workingDirectory string) []string {
	candidate := ""
	if workingDirectory != "." && workingDirectory != "" {
		if absolutePath, operationError := filepath.Abs(expandUser(os.ExpandEnv(workingDirectory))); operationError == nil {
			if resolved, operationError := filepath.EvalSymlinks(absolutePath); operationError == nil {
				candidate = resolved
			} else {
				candidate = absolutePath
			}
		} else {
			candidate = workingDirectory
		}
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(value string) {
		if value == "" {
			return
		}
		if _, valid := seen[value]; valid {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if candidate != "" {
		base := []string{candidate, strings.ReplaceAll(candidate, "/", `\`), strings.ReplaceAll(candidate, `\`, "/"), filepath.Clean(candidate)}
		for _, value := range base {
			add(value)
			if runtime.GOOS == "windows" {
				add(strings.ReplaceAll(value, `\`, `\\`))
			}
		}
	}
	return out
}

func expandUser(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, operationError := os.UserHomeDir(); operationError == nil && home != "" {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func encodeSecondVariant(variant string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(strings.ReplaceAll(variant, "/", `\`), `\`, `\\`)
	}
	return variant
}

// EncodeSessionWorkingDirectory implements Python quote(s, safe="") for Grok's session cwd directory names.
func EncodeSessionWorkingDirectory(workingDirectory string) string {
	var builder strings.Builder
	for itemIndex := 0; itemIndex < len(workingDirectory); itemIndex++ {
		currentByte := workingDirectory[itemIndex]
		if (currentByte >= 'a' && currentByte <= 'z') || (currentByte >= 'A' && currentByte <= 'Z') || (currentByte >= '0' && currentByte <= '9') || currentByte == '-' || currentByte == '_' || currentByte == '.' || currentByte == '~' {
			builder.WriteByte(currentByte)
		} else {
			builder.WriteByte('%')
			builder.WriteString(strings.ToUpper(strconv.FormatInt(int64(currentByte), 16)))
			if currentByte < 16 {
				// strconv wrote a single digit after %, so rewrite the suffix as %0X.
				encodedPath := builder.String()
				builder.Reset()
				builder.WriteString(encodedPath[:len(encodedPath)-1])
				builder.WriteByte('0')
				builder.WriteByte(encodedPath[len(encodedPath)-1])
			}
		}
	}
	return builder.String()
}

func matchWorkingDirectoryHits(hits []string, workingDirectory string) []string {
	cnorm := workingDirectory
	if absolutePath, operationError := filepath.Abs(expandUser(os.ExpandEnv(workingDirectory))); operationError == nil {
		if resolved, operationError := filepath.EvalSymlinks(absolutePath); operationError == nil {
			cnorm = resolved
		} else {
			cnorm = absolutePath
		}
	}
	if runtime.GOOS == "windows" {
		cnorm = strings.ToLower(strings.ReplaceAll(cnorm, "/", `\`))
	}
	var workingDirectoryMatches []string
	for _, directory := range hits {
		parent := percentDecode(filepath.Base(filepath.Dir(directory)))
		parent = strings.ReplaceAll(parent, "/", `\`)
		lowercasePath := parent
		if runtime.GOOS == "windows" {
			lowercasePath = strings.ToLower(parent)
		}
		if cnorm != "" && (lowercasePath == cnorm || strings.HasSuffix(lowercasePath, cnorm) || strings.HasSuffix(cnorm, lowercasePath)) {
			workingDirectoryMatches = append(workingDirectoryMatches, directory)
		}
	}
	return workingDirectoryMatches
}

func percentDecode(encodedText string) string {
	var builder strings.Builder
	for itemIndex := 0; itemIndex < len(encodedText); itemIndex++ {
		if encodedText[itemIndex] == '%' && itemIndex+2 < len(encodedText) {
			if value, operationError := strconv.ParseUint(encodedText[itemIndex+1:itemIndex+3], 16, 8); operationError == nil {
				builder.WriteByte(byte(value))
				itemIndex += 2
				continue
			}
		}
		builder.WriteByte(encodedText[itemIndex])
	}
	return builder.String()
}

func updateMTime(directory string) time.Time {
	fileInfo, operationError := os.Stat(filepath.Join(directory, "updates.jsonl"))
	if operationError != nil {
		return time.Time{}
	}
	return fileInfo.ModTime()
}

// ReadSessionInfo reads summary.json title/cwd using server.py's precedence.
func ReadSessionInfo(sessionDirectory string) SessionInfo {
	out := SessionInfo{}
	if sessionDirectory == "" {
		return out
	}
	data, operationError := os.ReadFile(filepath.Join(sessionDirectory, "summary.json"))
	if operationError != nil {
		return out
	}
	var summ map[string]any
	if json.Unmarshal(data, &summ) != nil {
		return out
	}
	out.Title = firstString(summ, "remote_title", "generated_title", "session_summary")
	if info, valid := asMap(summ["info"]); valid {
		out.WorkingDirectory = strings.TrimSpace(toString(info["cwd"]))
	}
	if out.WorkingDirectory == "" {
		out.WorkingDirectory = strings.TrimSpace(toString(summ["cwd"]))
	}
	return out
}

func firstString(mapping map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(toString(mapping[key])); value != "" {
			return value
		}
	}
	return ""
}

// ReadSessionUpdates reads updates.jsonl with the same filtering, coalescing and byte pagination semantics as grok-remote server.py.
func ReadSessionUpdates(sessionDirectory string, options ReadOptions) ([]Event, Meta) {
	if options.Limit == 0 {
		options.Limit = defaultLimit
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = defaultMaxBytes
	}
	path := filepath.Join(sessionDirectory, "updates.jsonl")
	fileInfo, operationError := os.Stat(path)
	if operationError != nil || fileInfo.IsDir() {
		return []Event{}, Meta{"path": path, "missing": true, "size": int64(0), "has_more": false}
	}
	size := fileInfo.Size()
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
		maxBytes = defaultMaxBytes
	}
	if options.Live && since >= size {
		return []Event{}, Meta{"path": path, "size": size, "returned": 0, "scanned": 0, "since": since, "live": true, "has_more": false, "end": size}
	}

	if options.Live {
		return readLive(path, size, since, limit, maxBytes)
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
		return []Event{}, Meta{"path": path, "size": size, "has_more": false, "window_start": 0, "window_end": 0, "returned": 0, "live": false}
	}
	if options.ChatOnly {
		return readChatOnly(path, size, endCap, limit, maxBytes)
	}
	return readFull(path, size, endCap, limit, maxBytes, options.ChatOnly)
}

func readLive(path string, size, since int64, limit int, maxBytes int64) ([]Event, Meta) {
	start := since
	if start <= 0 {
		capBytes := min64(maxBytes, 512_000)
		start = max64(0, size-capBytes)
		if start > 0 {
			start = advanceAfterLine(path, start, size)
		}
	}
	windowStart := start
	blob, operationError := readRange(path, windowStart, size)
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
		if !containsAnyBytes(line, [][]byte{[]byte("user_message_chunk"), []byte("agent_message_chunk"), []byte("agent_thought_chunk"), []byte("tool_call"), []byte("turn_completed"), []byte("task_completed"), []byte("plan"), []byte("session_recap")}) {
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
	return stripAndTrim(scored), Meta{"path": path, "size": size, "end": endPos, "returned": len(scored), "scanned": len(scored), "since": since, "live": true, "has_more": false, "window_start": windowStart}
}

func readChatOnly(path string, size, endCap int64, limit int, maxBytes int64) ([]Event, Meta) {
	want := maxInt(1, limit)
	maxScan := min64(64_000_000, max64(16_000_000, max64(maxBytes*16, int64(want)*800_000)))
	scan := min64(maxScan, max64(1_200_000, int64(want)*80_000))
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
			windowStart = advanceAfterLine(path, start, readTo)
		}
		if windowStart < readTo {
			blob, operationError := readRange(path, windowStart, readTo)
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
				if len(acc) >= want*4 {
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
		if groups >= want || start <= 0 || scan >= maxScan || len(acc) >= want*4 {
			break
		}
		scan = min64(maxScan, max64(scan*2, scan+4_000_000))
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
	return stripAndTrim(collected), Meta{"path": path, "size": size, "returned": len(collected), "scanned": scannedLines, "live": false, "has_more": hasMore, "window_start": firstOff, "window_end": endPos, "end": endPos, "older_before": firstOff, "chat_only": true}
}

func readFull(path string, size, endCap int64, limit int, maxBytes int64, chatOnly bool) ([]Event, Meta) {
	start := max64(0, endCap-maxBytes)
	windowStart := start
	if start > 0 {
		windowStart = advanceAfterLine(path, start, endCap)
	}
	blob, operationError := readRange(path, windowStart, endCap)
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
		chatBudget := maxInt(limit*3/4, maxInt(1, limit-20))
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
	events := stripAndTrim(kept)
	hasMore := windowStart > 0 || trimmed
	return events, Meta{"path": path, "size": size, "returned": len(events), "scanned": len(scored), "live": false, "has_more": hasMore, "window_start": firstOff, "window_end": endPos, "end": endPos, "older_before": firstOff, "chat_only": chatOnly}
}

func parseUpdateLine(line string, live bool) Event {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	var obj map[string]any
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	if operationError := dec.Decode(&obj); operationError != nil {
		return nil
	}
	params, valid := asMap(obj["params"])
	if !valid {
		return nil
	}
	update, valid := asMap(params["update"])
	if !valid || update == nil {
		return nil
	}
	kind := toString(update["sessionUpdate"])
	if _, valid := historyKeep[kind]; !valid {
		return nil
	}
	content, _ := asMap(update["content"])
	text := ""
	if content != nil {
		text = toString(content["text"])
	}
	if (kind == "user_message_chunk" || kind == "agent_message_chunk" || kind == "agent_thought_chunk") && strings.TrimSpace(text) == "" {
		return nil
	}
	if kind == "tool_call_update" && !live {
		status := strings.ToLower(toString(update["status"]))
		_, terminal := terminalToolStatuses[status]
		_, hasContent := update["content"]
		_, hasRaw := update["rawOutput"]
		if status != "" && !terminal && !hasContent && !hasRaw {
			return nil
		}
	}
	meta := map[string]any{}
	if pmeta, valid := asMap(params["_meta"]); valid {
		for key, value := range pmeta {
			meta[key] = value
		}
	}
	if umeta, valid := asMap(update["_meta"]); valid {
		for key, value := range umeta {
			if _, exists := meta[key]; !exists {
				meta[key] = value
			}
		}
	}
	method := obj["method"]
	if toString(method) == "" {
		method = "session/update"
	}
	eid := ""
	if value, valid := meta["eventId"]; valid {
		eid = toString(value)
	}
	if eid == "" {
		if umeta, valid := asMap(update["_meta"]); valid {
			eid = toString(umeta["eventId"])
		}
	}
	return Event{
		"method": method,
		"params": map[string]any{
			"sessionId": params["sessionId"],
			"update":    update,
			"_meta":     meta,
		},
		"_kind": kind,
		"_eid":  eid,
	}
}

func stripAndTrim(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		clean := Event{}
		for key, value := range event {
			if strings.HasPrefix(key, "_") {
				continue
			}
			clean[key] = value
		}
		out = append(out, trimChatText(clean, chatTextCap))
	}
	return out
}

func trimChatText(event Event, cap int) Event {
	params, valid := asMap(event["params"])
	if !valid {
		return event
	}
	update, valid := asMap(params["update"])
	if !valid {
		return event
	}
	content, valid := asMap(update["content"])
	if !valid || content == nil {
		return event
	}
	text, valid := content["text"].(string)
	if !valid || len(text) <= cap {
		return event
	}
	content2 := cloneMap(content)
	content2["text"] = text[:cap] + "\n…[truncated for load speed]"
	update2 := cloneMap(update)
	update2["content"] = content2
	params2 := cloneMap(params)
	params2["update"] = update2
	clean := cloneMap(event)
	clean["params"] = params2
	return clean
}

func coalesceChat(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		kind := toString(event["_kind"])
		if (kind != "user_message_chunk" && kind != "agent_message_chunk") || len(out) == 0 {
			out = append(out, event)
			continue
		}
		prev := out[len(out)-1]
		if toString(prev["_kind"]) != kind {
			out = append(out, event)
			continue
		}
		pparams, _ := asMap(prev["params"])
		pupdate, _ := asMap(pparams["update"])
		cparams, _ := asMap(event["params"])
		cupdate, _ := asMap(cparams["update"])
		pcontent, valid := asMap(pupdate["content"])
		if !valid || pcontent == nil {
			pcontent = map[string]any{"type": "text", "text": ""}
			pupdate["content"] = pcontent
		}
		ccontent, _ := asMap(cupdate["content"])
		pcontent["text"] = toString(pcontent["text"]) + toString(ccontent["text"])
		pparams["update"] = pupdate
		prev["params"] = pparams
	}
	return out
}

func asMap(value any) (map[string]any, bool) {
	mapping, valid := value.(map[string]any)
	return mapping, valid
}

func cloneMap[ValueType any](mapping map[string]ValueType) map[string]any {
	out := make(map[string]any, len(mapping))
	for key, value := range mapping {
		out[key] = value
	}
	return out
}

func toString(value any) string {
	switch typedValue := value.(type) {
	case nil:
		return ""
	case string:
		return typedValue
	case json.Number:
		return typedValue.String()
	case float64:
		if math.Trunc(typedValue) == typedValue {
			return strconv.FormatInt(int64(typedValue), 10)
		}
		return strconv.FormatFloat(typedValue, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(typedValue, 10)
	case int:
		return strconv.Itoa(typedValue)
	case bool:
		if typedValue {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func isMessageKind(kind string) bool {
	return kind == "user_message_chunk" || kind == "agent_message_chunk"
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

func readRange(path string, start, end int64) ([]byte, error) {
	if end < start {
		return nil, errors.New("invalid byte range")
	}
	file, operationError := os.Open(path)
	if operationError != nil {
		return nil, operationError
	}
	defer file.Close()
	if _, operationError := file.Seek(start, io.SeekStart); operationError != nil {
		return nil, operationError
	}
	buffer := make([]byte, end-start)
	_, operationError = io.ReadFull(file, buffer)
	if operationError != nil && operationError != io.ErrUnexpectedEOF {
		return nil, operationError
	}
	return buffer, nil
}

func advanceAfterLine(path string, start, end int64) int64 {
	if start <= 0 || start >= end {
		return start
	}
	file, operationError := os.Open(path)
	if operationError != nil {
		return start
	}
	defer file.Close()
	if _, operationError := file.Seek(start, io.SeekStart); operationError != nil {
		return start
	}
	remainingBytes := bufio.NewReader(io.LimitReader(file, end-start))
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
