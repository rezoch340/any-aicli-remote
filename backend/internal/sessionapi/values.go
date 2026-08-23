// Coercion helpers for loosely typed provider payloads.

package sessionapi

import (
	"encoding/json"
	"strconv"
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstTruthy(values ...any) any {
	for _, value := range values {
		if truthy(value) {
			return value
		}
	}
	return nil
}

func numeric(value any) (float64, bool) {
	switch typedValue := value.(type) {
	case json.Number:
		parsedNumber, operationError := typedValue.Float64()
		return parsedNumber, operationError == nil
	case float64:
		return typedValue, true
	case int:
		return float64(typedValue), true
	case int64:
		return float64(typedValue), true
	case string:
		parsedNumber, operationError := strconv.ParseFloat(strings.TrimSpace(typedValue), 64)
		return parsedNumber, operationError == nil
	default:
		return 0, false
	}
}

func truthy(value any) bool {
	switch typedValue := value.(type) {
	case nil:
		return false
	case bool:
		return typedValue
	case string:
		return strings.TrimSpace(typedValue) != ""
	case float64:
		return typedValue != 0
	case int:
		return typedValue != 0
	case int64:
		return typedValue != 0
	case json.Number:
		parsedNumber, operationError := typedValue.Float64()
		return operationError == nil && parsedNumber != 0
	default:
		return true
	}
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
