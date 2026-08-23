// Package grok implements Any AI CLI Remote's Grok provider adapter.
package grok

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	historydata "github.com/rezoch340/any-aicli-remote/backend/internal/provider/grok/historydata"
)

const (
	ProviderID                    = "grok"
	temporaryFileCreationAttempts = 16
	randomSuffixBytes             = 16
)

func (providerInstance *GrokProvider) ID() string { return ProviderID }

func (providerInstance *GrokProvider) ScanSessions(operationContext context.Context) ([]providerapi.SessionMetadata, error) {
	selected := make(map[string]scannedSession)
	for rootIndex, root := range providerInstance.sessionRoots() {
		paths, operationError := collectSummaryPaths(operationContext, root)
		if operationError != nil {
			return nil, operationError
		}
		for _, path := range paths {
			metadata, operationError := providerInstance.parseSummary(path)
			if operationError != nil {
				continue
			}
			candidate := scannedSession{metadata: metadata, active: rootIndex == 0}
			previous, present := selected[metadata.SessionID]
			if !present || preferSession(candidate, previous) {
				selected[metadata.SessionID] = candidate
			}
		}
	}
	sessions := make([]providerapi.SessionMetadata, 0, len(selected))
	for _, session := range selected {
		sessions = append(sessions, session.metadata)
	}
	providerapi.SortSessions(sessions)
	return sessions, nil
}

func preferSession(candidate, previous scannedSession) bool {
	if candidate.active != previous.active {
		return candidate.active
	}
	candidateTimestamp := sessionTimestamp(candidate.metadata)
	previousTimestamp := sessionTimestamp(previous.metadata)
	if candidateTimestamp != previousTimestamp {
		return candidateTimestamp > previousTimestamp
	}
	return candidate.metadata.SourcePath < previous.metadata.SourcePath
}

func sessionTimestamp(metadata providerapi.SessionMetadata) int64 {
	if metadata.LastActiveAt != 0 {
		return metadata.LastActiveAt
	}
	return metadata.CreatedAt
}

func (providerInstance *GrokProvider) ResolveSession(operationContext context.Context, sessionID string) (providerapi.SessionMetadata, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return providerapi.SessionMetadata{}, providerapi.SessionRequiredError
	}
	sessions, operationError := providerInstance.ScanSessions(operationContext)
	if operationError != nil {
		return providerapi.SessionMetadata{}, operationError
	}
	for _, metadata := range sessions {
		if metadata.SessionID != sessionID {
			continue
		}
		validated, validationError := providerInstance.validateSource(sessionID, metadata.SourcePath)
		if validationError != nil {
			return providerapi.SessionMetadata{}, validationError
		}
		metadata.SourcePath = validated
		return metadata, nil
	}
	return providerapi.SessionMetadata{}, providerapi.SessionNotFoundError
}

func (providerInstance *GrokProvider) LoadMessages(operationContext context.Context, sessionID string) ([]providerapi.Message, error) {
	metadata, operationError := providerInstance.ResolveSession(operationContext, sessionID)
	if operationError != nil {
		return nil, operationError
	}
	access, operationError := providerInstance.openSessionSource(sessionID, metadata.SourcePath)
	if operationError != nil {
		return nil, operationError
	}
	defer access.Close()
	file, operationError := openRegularFile(access.root, "chat_history.jsonl")
	if errors.Is(operationError, os.ErrNotExist) {
		return []providerapi.Message{}, nil
	}
	if operationError != nil {
		return nil, operationError
	}
	defer file.Close()
	messages := make([]providerapi.Message, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, providerInstance.historyPolicy.MessageScanInitialBytes), providerInstance.historyPolicy.MessageScanMaxBytes)
	for scanner.Scan() {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		var record map[string]any
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.UseNumber()
		if operationError := decoder.Decode(&record); operationError != nil {
			continue
		}
		role, _ := record["type"].(string)
		switch role {
		case "system", "user", "assistant", "tool":
		default:
			continue
		}
		content := providerapi.ExtractText(record["content"])
		if strings.TrimSpace(content) == "" {
			continue
		}
		timestamp := providerapi.ParseTimestampMilliseconds(record["timestamp"])
		if timestamp == 0 {
			timestamp = providerapi.ParseTimestampMilliseconds(record["ts"])
		}
		messages = append(messages, providerapi.Message{Role: role, Content: content, Timestamp: timestamp})
	}
	if operationError := scanner.Err(); operationError != nil {
		return nil, operationError
	}
	return messages, nil
}

func (providerInstance *GrokProvider) ReadHistory(operationContext context.Context, query providerapi.HistoryQuery) (providerapi.HistoryPage, error) {
	metadata, operationError := providerInstance.ResolveSession(operationContext, query.SessionID)
	if operationError != nil {
		return providerapi.HistoryPage{}, operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return providerapi.HistoryPage{}, operationError
	}
	access, operationError := providerInstance.openSessionSource(metadata.SessionID, metadata.SourcePath)
	if operationError != nil {
		return providerapi.HistoryPage{}, operationError
	}
	defer access.Close()
	events, historyMetadata, operationError := historydata.ReadSessionUpdatesFromRoot(access.root, filepath.Join(filepath.Dir(access.sourcePath), "updates.jsonl"), providerInstance.historyPolicy, historydata.ReadOptions{
		Limit: query.Limit, MaxBytes: query.MaxBytes, SinceBytes: query.SinceBytes,
		Live: query.Live, BeforeBytes: query.BeforeBytes, ChatOnly: query.ChatOnly,
	})
	if operationError != nil {
		return providerapi.HistoryPage{}, operationError
	}
	if historyError, valid := historyMetadata["error"].(string); valid && strings.TrimSpace(historyError) != "" {
		return providerapi.HistoryPage{}, errors.New(historyError)
	}
	convertedEvents := make([]providerapi.Event, 0, len(events))
	for _, event := range events {
		convertedEvents = append(convertedEvents, providerapi.Event(event))
	}
	convertedMetadata := providerapi.HistoryMetadata(historyMetadata)
	convertedMetadata["resolvedSid"] = metadata.SessionID
	convertedMetadata["resolvedDir"] = filepath.Dir(access.sourcePath)
	convertedMetadata["resolvedCwd"] = metadata.ProjectDirectory
	return providerapi.HistoryPage{Session: metadata, Events: convertedEvents, Metadata: convertedMetadata}, nil
}

func (providerInstance *GrokProvider) ReadSignals(operationContext context.Context, sessionID string) (map[string]any, error) {
	metadata, operationError := providerInstance.ResolveSession(operationContext, sessionID)
	if operationError != nil {
		return nil, operationError
	}
	access, operationError := providerInstance.openSessionSource(metadata.SessionID, metadata.SourcePath)
	if operationError != nil {
		return nil, operationError
	}
	defer access.Close()
	data, operationError := providerInstance.readMetadataFile(access.root, "signals.json")
	if errors.Is(operationError, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if operationError != nil {
		return nil, operationError
	}
	var signals map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if operationError := decoder.Decode(&signals); operationError != nil {
		return nil, operationError
	}
	if signals == nil {
		signals = map[string]any{}
	}
	return signals, nil
}

func (providerInstance *GrokProvider) RenameSession(operationContext context.Context, sessionID, title string) (providerapi.RenameResult, error) {
	metadata, operationError := providerInstance.ResolveSession(operationContext, sessionID)
	if operationError != nil {
		return providerapi.RenameResult{}, operationError
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return providerapi.RenameResult{}, errors.New("title required")
	}
	providerInstance.renameMutex.Lock()
	defer providerInstance.renameMutex.Unlock()
	access, operationError := providerInstance.openSessionSource(metadata.SessionID, metadata.SourcePath)
	if operationError != nil {
		return providerapi.RenameResult{}, operationError
	}
	defer access.Close()
	summary := access.summaryData
	previousTitle := firstString(summary, "remote_title", "generated_title", "session_summary")
	summary["remote_title"] = title
	summary["generated_title"] = title
	summary["session_summary"] = title
	summary["updated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, operationError := json.MarshalIndent(summary, "", "  ")
	if operationError != nil {
		return providerapi.RenameResult{}, operationError
	}
	temporaryName, temporaryFile, operationError := createTemporarySummary(access.root)
	if operationError != nil {
		return providerapi.RenameResult{}, operationError
	}
	defer access.root.Remove(temporaryName)
	payload := append(data, '\n')
	writtenBytes, operationError := temporaryFile.Write(payload)
	if operationError == nil && writtenBytes != len(payload) {
		operationError = io.ErrShortWrite
	}
	if operationError == nil {
		operationError = temporaryFile.Sync()
	}
	if closeError := temporaryFile.Close(); operationError == nil {
		operationError = closeError
	}
	if operationError != nil {
		return providerapi.RenameResult{}, operationError
	}
	if operationError := access.root.Rename(temporaryName, "summary.json"); operationError != nil {
		return providerapi.RenameResult{}, operationError
	}
	return providerapi.RenameResult{SessionID: sessionID, Title: title, PreviousTitle: previousTitle, SourcePath: access.sourcePath}, nil
}

func (providerInstance *GrokProvider) parseSummary(path string) (providerapi.SessionMetadata, error) {
	access, operationError := providerInstance.openSessionSource("", path)
	if operationError != nil {
		return providerapi.SessionMetadata{}, operationError
	}
	defer access.Close()
	summary := access.summaryData
	info, _ := summary["info"].(map[string]any)
	sessionID := strings.TrimSpace(stringValue(info["id"]))
	if sessionID == "" {
		return providerapi.SessionMetadata{}, errors.New("summary info.id required")
	}
	title := truncate(firstString(summary, "remote_title", "generated_title", "session_summary"), providerInstance.historyPolicy.MetadataTitleMaxRunes)
	sessionSummary := truncate(firstString(summary, "session_summary"), providerInstance.historyPolicy.MetadataSummaryMaxRunes)
	projectDirectory := strings.TrimSpace(stringValue(info["cwd"]))
	if projectDirectory == "" {
		projectDirectory = strings.TrimSpace(stringValue(summary["cwd"]))
	}
	createdAt := providerapi.ParseTimestampMilliseconds(summary["created_at"])
	lastActiveAt := providerapi.ParseTimestampMilliseconds(summary["last_active_at"])
	if lastActiveAt == 0 {
		lastActiveAt = providerapi.ParseTimestampMilliseconds(summary["updated_at"])
	}
	return providerapi.SessionMetadata{
		ProviderID: ProviderID, SessionID: sessionID, Title: title, Summary: sessionSummary,
		ProjectDirectory: projectDirectory, CreatedAt: createdAt, LastActiveAt: lastActiveAt,
		SourcePath: access.sourcePath, ResumeCommand: "grok --resume " + sessionID,
	}, nil
}

func (providerInstance *GrokProvider) validateSource(sessionID, sourcePath string) (string, error) {
	access, operationError := providerInstance.openSessionSource(sessionID, sourcePath)
	if operationError != nil {
		return "", operationError
	}
	defer access.Close()
	return access.sourcePath, nil
}

func (providerInstance *GrokProvider) openSessionSource(expectedSessionID, sourcePath string) (*sessionAccess, error) {
	sourceAbsolute, operationError := filepath.Abs(filepath.Clean(sourcePath))
	if operationError != nil {
		return nil, operationError
	}
	selectedRoot := ""
	selectedMatch := ""
	selectedRelative := ""
	for _, configuredRoot := range providerInstance.sessionRoots() {
		rootAbsolute, absoluteError := filepath.Abs(filepath.Clean(configuredRoot))
		if absoluteError != nil {
			continue
		}
		canonicalRoot, canonicalError := filepath.EvalSymlinks(rootAbsolute)
		if canonicalError != nil {
			continue
		}
		for _, matchRoot := range []string{rootAbsolute, canonicalRoot} {
			relativeSource, relativeError := filepath.Rel(matchRoot, sourceAbsolute)
			if relativeError != nil || relativeSource == ".." || strings.HasPrefix(relativeSource, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeSource) {
				continue
			}
			if selectedRoot == "" || len(matchRoot) > len(selectedMatch) {
				selectedRoot = canonicalRoot
				selectedMatch = matchRoot
				selectedRelative = relativeSource
			}
		}
	}
	if selectedRoot == "" {
		return nil, errors.New("path is outside provider roots")
	}
	if filepath.Base(selectedRelative) != "summary.json" {
		return nil, errors.New("unexpected Grok session source")
	}
	sessionRelative := filepath.Dir(selectedRelative)
	if sessionRelative == "." {
		return nil, errors.New("unexpected Grok session directory")
	}

	providerRootIdentity, operationError := fsapi.PinRoot(selectedRoot)
	if operationError != nil {
		return nil, operationError
	}
	providerRoot, operationError := providerRootIdentity.OpenRoot()
	if operationError != nil {
		return nil, operationError
	}
	defer providerRoot.Close()
	if operationError := validateDirectoryComponents(providerRoot, sessionRelative); operationError != nil {
		return nil, operationError
	}
	sessionRoot, operationError := openStableSubroot(providerRoot, sessionRelative)
	if operationError != nil {
		return nil, operationError
	}
	if operationError := validateDirectoryComponents(providerRoot, sessionRelative); operationError != nil {
		_ = sessionRoot.Close()
		return nil, operationError
	}
	summaryData, operationError := providerInstance.readMetadataFile(sessionRoot, "summary.json")
	if operationError != nil {
		_ = sessionRoot.Close()
		return nil, operationError
	}
	summary := map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(string(summaryData)))
	decoder.UseNumber()
	operationError = decoder.Decode(&summary)
	if operationError != nil {
		_ = sessionRoot.Close()
		return nil, operationError
	}
	if summary == nil {
		_ = sessionRoot.Close()
		return nil, errors.New("JSON object required")
	}
	info, _ := summary["info"].(map[string]any)
	summarySessionID := strings.TrimSpace(stringValue(info["id"]))
	if summarySessionID == "" {
		_ = sessionRoot.Close()
		return nil, errors.New("summary info.id required")
	}
	if filepath.Base(sessionRelative) != summarySessionID {
		_ = sessionRoot.Close()
		return nil, errors.New("session directory does not match summary info.id")
	}
	if expectedSessionID != "" && summarySessionID != expectedSessionID {
		_ = sessionRoot.Close()
		return nil, errors.New("summary sessionId mismatch")
	}
	return &sessionAccess{
		root: sessionRoot, sourcePath: filepath.Join(selectedRoot, selectedRelative), summaryData: summary,
	}, nil
}

func openRegularFile(root *os.Root, name string) (*os.File, error) {
	fileInfo, operationError := root.Lstat(name)
	if operationError != nil {
		return nil, operationError
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
		return nil, errors.New("regular file required")
	}
	file, operationError := root.Open(name)
	if operationError != nil {
		return nil, operationError
	}
	openedInfo, operationError := file.Stat()
	if operationError != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(fileInfo, openedInfo) {
		_ = file.Close()
		return nil, errors.New("file changed while opening")
	}
	return file, nil
}

func openStableSubroot(parentRoot *os.Root, relativePath string) (*os.Root, error) {
	beforeInfo, operationError := parentRoot.Lstat(relativePath)
	if operationError != nil {
		return nil, operationError
	}
	if beforeInfo.Mode()&os.ModeSymlink != 0 || !beforeInfo.IsDir() {
		return nil, errors.New("session root must be a directory")
	}
	root, operationError := parentRoot.OpenRoot(relativePath)
	if operationError != nil {
		return nil, operationError
	}
	openedInfo, operationError := root.Stat(".")
	if operationError != nil {
		_ = root.Close()
		return nil, operationError
	}
	afterInfo, operationError := parentRoot.Lstat(relativePath)
	if operationError != nil || afterInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(beforeInfo, openedInfo) || !os.SameFile(openedInfo, afterInfo) {
		_ = root.Close()
		return nil, errors.New("session root changed while opening")
	}
	return root, nil
}

func validateDirectoryComponents(root *os.Root, relativePath string) error {
	currentRelative := "."
	for _, component := range strings.Split(filepath.Clean(relativePath), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		currentRelative = filepath.Join(currentRelative, component)
		fileInfo, operationError := root.Lstat(currentRelative)
		if operationError != nil {
			return operationError
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.IsDir() {
			return errors.New("Grok session path contains a symbolic link")
		}
	}
	return nil
}

func createTemporarySummary(root *os.Root) (string, *os.File, error) {
	for attempt := 0; attempt < temporaryFileCreationAttempts; attempt++ {
		randomBytes := make([]byte, randomSuffixBytes)
		if _, operationError := rand.Read(randomBytes); operationError != nil {
			return "", nil, operationError
		}
		name := ".summary.json.any-aicli-remote-" + hex.EncodeToString(randomBytes) + ".tmp"
		file, operationError := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(operationError, os.ErrExist) {
			continue
		}
		if operationError != nil {
			return "", nil, operationError
		}
		return name, file, nil
	}
	return "", nil, errors.New("could not create unique summary file")
}

func (providerInstance *GrokProvider) readMetadataFile(root *os.Root, name string) ([]byte, error) {
	file, operationError := openRegularFile(root, name)
	if operationError != nil {
		return nil, operationError
	}
	defer file.Close()
	limit := providerInstance.historyPolicy.AdapterReadBytes
	if limit < 1 || limit >= int64(math.MaxInt64) {
		return nil, MetadataFileTooLargeError
	}
	data, operationError := io.ReadAll(io.LimitReader(file, limit+1))
	if operationError != nil {
		return nil, operationError
	}
	if int64(len(data)) > limit {
		return nil, MetadataFileTooLargeError
	}
	return data, nil
}

func collectSummaryPaths(operationContext context.Context, root string) ([]string, error) {
	paths := make([]string, 0)
	operationError := filepath.WalkDir(root, func(path string, entry os.DirEntry, traversalError error) error {
		if traversalError != nil {
			if errors.Is(traversalError, os.ErrNotExist) {
				return nil
			}
			return traversalError
		}
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		if entry.Type()&os.ModeSymlink != 0 && entry.IsDir() {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "summary.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if errors.Is(operationError, os.ErrNotExist) {
		return paths, nil
	}
	return paths, operationError
}

func firstString(mapping map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringValue(mapping[key])); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, valid := value.(string); valid {
		return text
	}
	return fmt.Sprint(value)
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

var MetadataFileTooLargeError = errors.New("metadata file too large")

var _ providerapi.Provider = (*GrokProvider)(nil)
