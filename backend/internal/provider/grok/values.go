// Value coercion for loosely typed Grok payloads and summary documents.

package grok

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

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

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func anyInt64(value any) int64 {
	switch typedValue := value.(type) {
	case int64:
		return typedValue
	case int:
		return int64(typedValue)
	case float64:
		return int64(typedValue)
	case json.Number:
		result, _ := typedValue.Int64()
		return result
	case string:
		result, parseError := strconv.ParseInt(strings.TrimSpace(typedValue), 10, 64)
		if parseError == nil {
			return result
		}
	}
	return 0
}

func anyInt(value any) int {
	return int(anyInt64(value))
}

func anyFloat64(value any) float64 {
	switch typedValue := value.(type) {
	case float64:
		return typedValue
	case int64:
		return float64(typedValue)
	case int:
		return float64(typedValue)
	case json.Number:
		result, _ := typedValue.Float64()
		return result
	case string:
		result, parseError := strconv.ParseFloat(strings.TrimSpace(typedValue), 64)
		if parseError == nil {
			return result
		}
	}
	return 0
}

func stringSliceValue(value any) []string {
	values, valid := value.([]any)
	if !valid {
		if stringsValue, valid := value.([]string); valid {
			return append([]string{}, stringsValue...)
		}
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		text := strings.TrimSpace(stringValue(item))
		if text != "" {
			result = append(result, text)
		}
	}
	return result
}

func eventSequence(eventID string) (uint64, bool) {
	dashIndex := strings.LastIndex(strings.TrimSpace(eventID), "-")
	if dashIndex < 0 {
		return 0, false
	}
	sequence, parseError := strconv.ParseUint(strings.TrimSpace(eventID[dashIndex+1:]), 10, 64)
	if parseError != nil {
		return 0, false
	}
	return sequence, true
}

func clampPercent(numberValue float64) float64 {
	if math.IsNaN(numberValue) || math.IsInf(numberValue, 0) {
		return 0
	}
	if numberValue < 0 {
		return 0
	}
	if numberValue > 100 {
		return 100
	}
	return numberValue
}

func nonnegativeInt64(numberValue int64) int64 {
	if numberValue < 0 {
		return 0
	}
	return numberValue
}

func nonnegativeInt(numberValue int) int {
	if numberValue < 0 {
		return 0
	}
	return numberValue
}
