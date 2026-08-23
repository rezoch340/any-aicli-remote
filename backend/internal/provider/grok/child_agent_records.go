// Persisted child-agent records. Invalid, oversized, or unsafe entries below
// the subagents directory are excluded rather than partially reported.

package grok

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

// ListChildAgents returns valid structured child-agent metadata stored directly
// below subagents. Invalid, oversized, or unsafe child entries are excluded.
func (providerInstance *GrokProvider) ListChildAgents(requestContext context.Context, sessionID string) ([]providerapi.ChildAgentRecord, error) {
	if requestError := requestContext.Err(); requestError != nil {
		return nil, requestError
	}

	sessionMetadata, resolveError := providerInstance.ResolveSession(requestContext, sessionID)
	if resolveError != nil {
		return nil, resolveError
	}

	sessionRoot, rootError := providerInstance.openSessionSource(sessionMetadata.SessionID, sessionMetadata.SourcePath)
	if rootError != nil {
		return nil, rootError
	}
	defer sessionRoot.Close()

	subagentsRoot, subrootError := openStableSubroot(sessionRoot.root, childAgentsDirectoryName)
	if errors.Is(subrootError, os.ErrNotExist) {
		return []providerapi.ChildAgentRecord{}, nil
	}
	if subrootError != nil {
		return nil, subrootError
	}
	defer subagentsRoot.Close()

	directoryHandle, directoryError := subagentsRoot.Open(".")
	if directoryError != nil {
		return nil, directoryError
	}
	defer directoryHandle.Close()
	directoryEntries, directoryError := directoryHandle.ReadDir(-1)
	if directoryError != nil {
		return nil, directoryError
	}

	childAgents := make([]providerapi.ChildAgentRecord, 0, len(directoryEntries))
	for _, directoryEntry := range directoryEntries {
		if requestError := requestContext.Err(); requestError != nil {
			return nil, requestError
		}
		childAgent, validEntry := providerInstance.readChildAgentRecord(subagentsRoot, directoryEntry.Name(), sessionMetadata.SessionID)
		if validEntry {
			childAgents = append(childAgents, childAgent)
		}
	}

	sort.Slice(childAgents, func(leftIndex, rightIndex int) bool {
		if childAgents[leftIndex].StartedAt == childAgents[rightIndex].StartedAt {
			return childAgents[leftIndex].ProviderChildID < childAgents[rightIndex].ProviderChildID
		}
		return childAgents[leftIndex].StartedAt < childAgents[rightIndex].StartedAt
	})
	return childAgents, nil
}

func (providerInstance *GrokProvider) readChildAgentRecord(subagentsRoot *os.Root, directoryName string, parentSessionID string) (providerapi.ChildAgentRecord, bool) {
	directoryInfo, directoryError := subagentsRoot.Lstat(directoryName)
	if directoryError != nil || directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return providerapi.ChildAgentRecord{}, false
	}
	childRoot, rootError := openStableSubroot(subagentsRoot, directoryName)
	if rootError != nil {
		return providerapi.ChildAgentRecord{}, false
	}
	defer childRoot.Close()
	metadataBytes, metadataError := providerInstance.readMetadataFile(childRoot, childAgentMetadataName)
	if metadataError != nil {
		return providerapi.ChildAgentRecord{}, false
	}
	var metadata grokChildAgentMetadata
	if jsonError := json.Unmarshal(metadataBytes, &metadata); jsonError != nil {
		return providerapi.ChildAgentRecord{}, false
	}
	if metadata.SubagentID != directoryName || metadata.ParentSessionID != parentSessionID || strings.TrimSpace(metadata.ChildSessionID) == "" {
		return providerapi.ChildAgentRecord{}, false
	}
	return providerapi.ChildAgentRecord{
		ProviderChildID:     directoryName,
		ParentSessionID:     parentSessionID,
		ParentPromptID:      strings.TrimSpace(metadata.ParentPromptID),
		ChildSessionID:      strings.TrimSpace(metadata.ChildSessionID),
		AgentType:           strings.TrimSpace(metadata.SubagentType),
		Description:         truncate(strings.TrimSpace(metadata.Description), providerInstance.historyPolicy.MetadataSummaryMaxRunes),
		Status:              normalizeChildAgentStatus(metadata.Status),
		StartedAt:           parseChildAgentTimestamp(metadata.StartedAt),
		CompletedAt:         parseChildAgentTimestamp(metadata.CompletedAt),
		DurationMS:          nonnegativeInt64(metadata.DurationMS),
		ToolCallCount:       nonnegativeInt(metadata.ToolCalls),
		TurnCount:           nonnegativeInt(metadata.Turns),
		ModelID:             strings.TrimSpace(metadata.ModelID),
		ContextSource:       strings.TrimSpace(metadata.EffectiveContextSource),
		ContextNormalized:   metadata.ContextNormalized,
		CapabilityMode:      strings.TrimSpace(metadata.CapabilityMode),
		Persona:             strings.TrimSpace(metadata.Persona),
		Role:                strings.TrimSpace(metadata.Role),
		ResumedFrom:         strings.TrimSpace(metadata.ResumedFrom),
		TokensUsed:          nonnegativeInt64(metadata.TokensUsed),
		ContextWindowTokens: nonnegativeInt64(metadata.ContextWindowTokens),
		ContextUsagePercent: clampPercent(metadata.ContextUsagePercent),
		ToolsUsed:           append([]string{}, metadata.ToolsUsed...),
		ErrorCount:          nonnegativeInt(metadata.ErrorCount),
		WorkingDirectory:    strings.TrimSpace(metadata.ChildDirectory),
	}, true
}

func normalizeChildAgentStatus(statusValue string) providerapi.ChildAgentStatus {
	switch strings.ToLower(strings.TrimSpace(statusValue)) {
	case "running":
		return providerapi.ChildAgentStatusRunning
	case "completed":
		return providerapi.ChildAgentStatusCompleted
	case "failed":
		return providerapi.ChildAgentStatusFailed
	case "cancelled", "canceled":
		return providerapi.ChildAgentStatusCancelled
	default:
		return providerapi.ChildAgentStatusUnknown
	}
}

func parseChildAgentTimestamp(timestampValue string) int64 {
	parsedTime, parseError := time.Parse(time.RFC3339Nano, strings.TrimSpace(timestampValue))
	if parseError != nil {
		return 0
	}
	return parsedTime.UnixMilli()
}
