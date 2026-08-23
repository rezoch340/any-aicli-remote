// Package compat centralizes reads and migrations for identifiers used before
// the Any AI CLI Remote rename. Current code must not duplicate legacy names.
package compat

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	AuthenticationCookieName = "any_aicli_remote_key"
	AuthenticationHeaderName = "X-Any-AI-CLI-Remote-Key"

	legacyAuthenticationCookieName = "grok_remote_key"
	legacyAuthenticationHeaderName = "X-Grok-Remote-Key"
)

var legacyEnvironmentKeys = map[string]string{
	"ANY_AI_CLI_REMOTE_DATA_DIR":            "GROK_PLUGIN_DATA",
	"ANY_AI_CLI_REMOTE_BIND":                "GROK_REMOTE_BIND",
	"ANY_AI_CLI_REMOTE_PORT":                "GROK_REMOTE_PORT",
	"ANY_AI_CLI_REMOTE_AGENT_HOST":          "GROK_REMOTE_AGENT_HOST",
	"ANY_AI_CLI_REMOTE_AGENT_PORT":          "GROK_REMOTE_AGENT_PORT",
	"ANY_AI_CLI_REMOTE_PAIRING_SECRET_FILE": "GROK_REMOTE_SECRET_FILE",
	"ANY_AI_CLI_REMOTE_RUNTIME_DIR":         "GROK_REMOTE_RUNTIME_DIR",
	"ANY_AI_CLI_REMOTE_PUBLIC_HOST":         "GROK_REMOTE_PUBLIC_HOST",
	"ANY_AI_CLI_REMOTE_PROVIDER":            "GROK_REMOTE_PROVIDER",
	"ANY_AI_CLI_REMOTE_PROVIDER_PATH":       "GROK_REMOTE_GROK_PATH",
	"ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR":   "GROK_REMOTE_SESSIONS_DIR",
	"ANY_AI_CLI_REMOTE_ENSURE_AGENT":        "GROK_REMOTE_ENSURE_AGENT",
	"ANY_AI_CLI_REMOTE_STOP_AGENT_ON_EXIT":  "GROK_REMOTE_STOP_AGENT_ON_EXIT",
	"ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE": "GROK_REMOTE_ALWAYS_APPROVE",
	"ANY_AI_CLI_REMOTE_GROK_LEADER":         "GROK_REMOTE_LEADER",
	"ANY_AI_CLI_REMOTE_CWD":                 "GROK_REMOTE_CWD",
}

func Environment(primaryKey string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(primaryKey)); value != "" {
		return value
	}
	if legacyKey := legacyEnvironmentKeys[primaryKey]; legacyKey != "" {
		if value := strings.TrimSpace(os.Getenv(legacyKey)); value != "" {
			return value
		}
	}
	return fallback
}

func BooleanEnvironment(primaryKey string, fallback bool) bool {
	value := strings.ToLower(Environment(primaryKey, ""))
	if value == "" {
		return fallback
	}
	parsed, operationError := strconv.ParseBool(value)
	if operationError == nil {
		return parsed
	}
	return value == "yes" || value == "on"
}

func AuthenticationCookie(request *http.Request) string {
	return cookie(request, AuthenticationCookieName, legacyAuthenticationCookieName)
}

func AuthenticationHeader(request *http.Request) string {
	return header(request, AuthenticationHeaderName, legacyAuthenticationHeaderName)
}

func header(request *http.Request, primaryName string, legacyName string) string {
	if request == nil {
		return ""
	}
	if value := strings.TrimSpace(request.Header.Get(primaryName)); value != "" {
		return value
	}
	return strings.TrimSpace(request.Header.Get(legacyName))
}

func cookie(request *http.Request, primaryName string, legacyName string) string {
	if request == nil {
		return ""
	}
	if currentCookie, operationError := request.Cookie(primaryName); operationError == nil {
		if value := strings.TrimSpace(currentCookie.Value); value != "" {
			return value
		}
	}
	if legacyCookie, operationError := request.Cookie(legacyName); operationError == nil {
		return strings.TrimSpace(legacyCookie.Value)
	}
	return ""
}

func MigrateDataFiles(dataDirectory, homeDirectory string, migratePairingSecret bool) error {
	legacyDataDirectory := filepath.Join(homeDirectory, ".grok", "plugin-data", "grok-remote")
	if filepath.Clean(dataDirectory) == filepath.Clean(legacyDataDirectory) {
		return nil
	}
	if operationError := os.MkdirAll(dataDirectory, 0o700); operationError != nil {
		return fmt.Errorf("create data directory: %w", operationError)
	}
	fileMappings := [][2]string{
		{"archived_sessions.json", "archived_sessions.json"},
		{"loops.json", "loops.json"},
	}
	if migratePairingSecret {
		fileMappings = append(fileMappings, [2]string{"pairing-secret", ".ui-secret"})
	}
	for _, fileMapping := range fileMappings {
		destinationPath := filepath.Join(dataDirectory, fileMapping[0])
		if _, operationError := os.Stat(destinationPath); operationError == nil {
			continue
		}
		sourcePath := filepath.Join(legacyDataDirectory, fileMapping[1])
		data, operationError := os.ReadFile(sourcePath)
		if operationError != nil {
			continue
		}
		if operationError := os.WriteFile(destinationPath, data, 0o600); operationError != nil {
			return fmt.Errorf("migrate legacy data file: %w", operationError)
		}
	}
	return nil
}
