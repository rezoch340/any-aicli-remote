// Session catalog for the Grok adapter: discovery, summary parsing, identity
// resolution, and title mutation over the on-disk session roots.

package grok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

type scannedSession struct {
	metadata providerapi.SessionMetadata
	active   bool
}

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
