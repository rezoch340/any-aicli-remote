// Session history reads for the Grok adapter: chat messages, paginated update
// events, and signal metadata resolved through the session store.

package grok

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	historydata "github.com/rezoch340/any-aicli-remote/backend/internal/provider/grok/historydata"
)

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
		convertedEvent := providerapi.Event(event)
		method, _ := convertedEvent["method"].(string)
		params, _ := convertedEvent["params"].(map[string]any)
		if normalizedMethod, normalizedParams, handled := providerInstance.normalizeChildAgentNotification(method, params); handled {
			if normalizedMethod == "" {
				continue
			}
			normalizedSessionID, _ := normalizedParams["sessionId"].(string)
			if strings.TrimSpace(normalizedSessionID) != metadata.SessionID {
				continue
			}
			convertedEvent["method"] = normalizedMethod
			convertedEvent["params"] = normalizedParams
		}
		convertedEvents = append(convertedEvents, convertedEvent)
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
