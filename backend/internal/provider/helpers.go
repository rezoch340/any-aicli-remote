package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	millisecondsTimestampThreshold = int64(1_000_000_000_000)
	millisecondsPerSecond          = int64(1000)
)

func CanonicalDirectory(rawPath string) (string, error) {
	path := strings.TrimSpace(os.ExpandEnv(rawPath))
	if path == "" {
		return "", WorkspaceRequiredError
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		homeDirectory, operationError := os.UserHomeDir()
		if operationError != nil {
			return "", fmt.Errorf("resolve home directory: %w", operationError)
		}
		if path == "~" {
			path = homeDirectory
		} else {
			path = filepath.Join(homeDirectory, strings.TrimPrefix(path, "~/"))
		}
	}
	absolutePath, operationError := filepath.Abs(filepath.Clean(path))
	if operationError != nil {
		return "", operationError
	}
	canonicalPath, operationError := filepath.EvalSymlinks(absolutePath)
	if operationError != nil {
		return "", operationError
	}
	fileInfo, operationError := os.Stat(canonicalPath)
	if operationError != nil || !fileInfo.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", canonicalPath)
	}
	return canonicalPath, nil
}

func CanonicalPathWithinRoots(rawPath string, roots []string) (string, string, error) {
	canonicalPath, operationError := canonicalExistingPath(rawPath)
	if operationError != nil {
		return "", "", operationError
	}
	bestRoot := ""
	for _, rawRoot := range roots {
		canonicalRoot, rootError := canonicalExistingPath(rawRoot)
		if rootError != nil {
			continue
		}
		if PathContainedBy(canonicalRoot, canonicalPath) && len(canonicalRoot) > len(bestRoot) {
			bestRoot = canonicalRoot
		}
	}
	if bestRoot == "" {
		return "", "", errors.New("path is outside provider roots")
	}
	return canonicalPath, bestRoot, nil
}

func canonicalExistingPath(rawPath string) (string, error) {
	absolutePath, operationError := filepath.Abs(filepath.Clean(rawPath))
	if operationError != nil {
		return "", operationError
	}
	return filepath.EvalSymlinks(absolutePath)
}

func PathContainedBy(root string, candidate string) bool {
	relativePath, operationError := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if operationError != nil {
		return false
	}
	return relativePath == "." || (relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)))
}

func ParseTimestampMilliseconds(value any) int64 {
	switch typedValue := value.(type) {
	case json.Number:
		if integerValue, operationError := typedValue.Int64(); operationError == nil {
			return normalizeTimestamp(integerValue)
		}
		if decimalValue, operationError := typedValue.Float64(); operationError == nil {
			return normalizeTimestamp(int64(decimalValue))
		}
	case float64:
		return normalizeTimestamp(int64(typedValue))
	case int64:
		return normalizeTimestamp(typedValue)
	case int:
		return normalizeTimestamp(int64(typedValue))
	case string:
		if parsedTime, operationError := time.Parse(time.RFC3339Nano, typedValue); operationError == nil {
			return parsedTime.UnixMilli()
		}
	}
	return 0
}

func normalizeTimestamp(value int64) int64 {
	if value > millisecondsTimestampThreshold {
		return value
	}
	return value * millisecondsPerSecond
}

func ExtractText(content any) string {
	switch typedContent := content.(type) {
	case string:
		return typedContent
	case []any:
		parts := make([]string, 0, len(typedContent))
		for _, item := range typedContent {
			text := extractTextItem(item)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		return stringField(typedContent, "text")
	default:
		return ""
	}
}

func extractTextItem(item any) string {
	mapping, valid := item.(map[string]any)
	if !valid {
		return ExtractText(item)
	}
	itemType := stringField(mapping, "type")
	if itemType == "tool_use" || itemType == "toolCall" {
		name := stringField(mapping, "name")
		if name == "" {
			name = "unknown"
		}
		return "[Tool: " + name + "]"
	}
	if itemType == "tool_result" {
		return ExtractText(mapping["content"])
	}
	for _, key := range []string{"text", "input_text", "output_text"} {
		if text := stringField(mapping, key); text != "" {
			return text
		}
	}
	return ExtractText(mapping["content"])
}

func stringField(mapping map[string]any, key string) string {
	value, _ := mapping[key].(string)
	return value
}
