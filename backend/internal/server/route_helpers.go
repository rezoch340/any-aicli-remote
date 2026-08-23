package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
)

func writeJSON(responseWriter http.ResponseWriter, status int, value any) {
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	responseWriter.WriteHeader(status)
	encoder := json.NewEncoder(responseWriter)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeText(responseWriter http.ResponseWriter, status int, value string) {
	responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.WriteHeader(status)
	_, _ = io.WriteString(responseWriter, value)
}

func writeAPIError(responseWriter http.ResponseWriter, status int, errorValue error) {
	writeJSON(responseWriter, status, map[string]any{"ok": false, "error": errorValue.Error()})
}

func decodeJSON(responseWriter http.ResponseWriter, request *http.Request, target any, allowEmpty bool) error {
	decoder := json.NewDecoder(http.MaxBytesReader(responseWriter, request.Body, maxRequestBody))
	decoder.UseNumber()
	errorValue := decoder.Decode(target)
	if errors.Is(errorValue, io.EOF) && allowEmpty {
		return nil
	}
	if errorValue != nil {
		writeText(responseWriter, http.StatusBadRequest, "json required")
		return errorValue
	}
	return nil
}

func decodeLooseJSON(request *http.Request, target any) {
	if request.Body == nil {
		return
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRequestBody))
	decoder.UseNumber()
	_ = decoder.Decode(target)
}

func writeFSError(responseWriter http.ResponseWriter, errorValue error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(errorValue, fsapi.OutsideWorkspaceError):
		status = http.StatusForbidden
	case errors.Is(errorValue, fsapi.NotFileError), errors.Is(errorValue, os.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(errorValue, fsapi.FileTooLargeError), errors.Is(errorValue, fsapi.ContentTooLargeError):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(errorValue, fsapi.PathRequiredError), errors.Is(errorValue, fsapi.NotDirectoryError), errors.Is(errorValue, fsapi.ContentRequiredError):
		status = http.StatusBadRequest
	}
	writeText(responseWriter, status, errorValue.Error())
}

func requestLoopback(request *http.Request) bool {
	host, _, errorValue := net.SplitHostPort(request.RemoteAddr)
	if errorValue != nil {
		host = request.RemoteAddr
	}
	ipAddress := net.ParseIP(strings.Trim(host, "[]"))
	return ipAddress != nil && ipAddress.IsLoopback()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return parseBool(typed)
	case float64:
		return typed != 0
	case json.Number:
		count, _ := typed.Int64()
		return count != 0
	default:
		return false
	}
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

func intQuery(value string, fallback int) int {
	parsed, errorValue := strconv.Atoi(strings.TrimSpace(value))
	if errorValue != nil {
		return fallback
	}
	return parsed
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func redactSecret(value, secret string, limit int) string {
	if secret != "" {
		value = strings.ReplaceAll(value, secret, "***")
	}
	return truncate(value, limit)
}
