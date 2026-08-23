package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBooleanEnvironmentUsesPrimaryAndFallbackValues(testContext *testing.T) {
	testContext.Setenv("CURRENT_BOOLEAN", "false")
	if BooleanEnvironment("CURRENT_BOOLEAN", true) {
		testContext.Fatal("explicit false was ignored")
	}
	testContext.Setenv("CURRENT_BOOLEAN", "yes")
	if !BooleanEnvironment("CURRENT_BOOLEAN", false) {
		testContext.Fatal("yes was not parsed")
	}
	testContext.Setenv("CURRENT_BOOLEAN", "")
	if !BooleanEnvironment("CURRENT_BOOLEAN", true) {
		testContext.Fatal("fallback was ignored")
	}
}

func TestParseArchivedSessionIDsAcceptsCurrentAndLegacyFormats(testContext *testing.T) {
	testCases := []struct {
		name       string
		contents   string
		wantLegacy bool
	}{
		{name: "current", contents: `["session-a"]`},
		{name: "legacy", contents: `{"ids":["session-a"],"updatedAt":123}`, wantLegacy: true},
	}
	for _, testCase := range testCases {
		testContext.Run(testCase.name, func(testContext *testing.T) {
			sessionIDs, legacy, operationError := ParseArchivedSessionIDs([]byte(testCase.contents))
			if operationError != nil || legacy != testCase.wantLegacy || len(sessionIDs) != 1 || sessionIDs[0] != "session-a" {
				testContext.Fatalf("ids=%q legacy=%t error=%v", sessionIDs, legacy, operationError)
			}
		})
	}
}

func TestMigrateDataFilesNormalizesLegacyArchivedSessions(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	legacyDirectory := filepath.Join(homeDirectory, ".grok", "plugin-data", "grok-remote")
	if operationError := os.MkdirAll(legacyDirectory, 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(filepath.Join(legacyDirectory, "archived_sessions.json"), []byte(`{"ids":["session-a"],"updatedAt":123}`), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	dataDirectory := filepath.Join(homeDirectory, ".any-aicli-remote")
	if operationError := MigrateDataFiles(dataDirectory, homeDirectory, false); operationError != nil {
		testContext.Fatal(operationError)
	}
	assertCurrentArchivedFile(testContext, filepath.Join(dataDirectory, "archived_sessions.json"), []string{"session-a"})
}

func TestMigrateDataFilesNormalizesExistingLegacyArchivedSessions(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	dataDirectory := filepath.Join(homeDirectory, ".any-aicli-remote")
	if operationError := os.MkdirAll(dataDirectory, 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	archivePath := filepath.Join(dataDirectory, "archived_sessions.json")
	if operationError := os.WriteFile(archivePath, []byte(`{"ids":["session-b"],"updatedAt":456}`), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := MigrateDataFiles(dataDirectory, homeDirectory, false); operationError != nil {
		testContext.Fatal(operationError)
	}
	assertCurrentArchivedFile(testContext, archivePath, []string{"session-b"})
	fileInfo, operationError := os.Stat(archivePath)
	if operationError != nil || fileInfo.Mode().Perm() != 0o600 {
		testContext.Fatalf("archive mode = %v, error = %v", fileInfo.Mode(), operationError)
	}
}

func TestParseArchivedSessionIDsRejectsMalformedData(testContext *testing.T) {
	for _, contents := range []string{"null", `{}`, `{"ids":null}`, `{"ids":"session-a"}`, `[1]`, "{"} {
		if _, _, operationError := ParseArchivedSessionIDs([]byte(contents)); operationError == nil {
			testContext.Fatalf("accepted malformed archive %q", contents)
		}
	}
}

func assertCurrentArchivedFile(testContext *testing.T, archivePath string, wantIDs []string) {
	testContext.Helper()
	contents, operationError := os.ReadFile(archivePath)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	var sessionIDs []string
	if operationError := json.Unmarshal(contents, &sessionIDs); operationError != nil {
		testContext.Fatalf("archive contents %q are not an array: %v", contents, operationError)
	}
	if len(sessionIDs) != len(wantIDs) || sessionIDs[0] != wantIDs[0] {
		testContext.Fatalf("archive IDs = %q, want %q", sessionIDs, wantIDs)
	}
}

func TestMigrateDataFilesCanExcludeLegacyPairingSecret(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	legacyDirectory := filepath.Join(homeDirectory, ".grok", "plugin-data", "grok-remote")
	if operationError := os.MkdirAll(legacyDirectory, 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(filepath.Join(legacyDirectory, ".ui-secret"), []byte("legacy-pairing-secret"), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(filepath.Join(legacyDirectory, "loops.json"), []byte("[]"), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	dataDirectory := filepath.Join(homeDirectory, ".any-aicli-remote")
	if operationError := MigrateDataFiles(dataDirectory, homeDirectory, false); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := os.Stat(filepath.Join(dataDirectory, "pairing-secret")); !os.IsNotExist(operationError) {
		testContext.Fatalf("legacy pairing secret was persisted: %v", operationError)
	}
	if contents, operationError := os.ReadFile(filepath.Join(dataDirectory, "loops.json")); operationError != nil || string(contents) != "[]" {
		testContext.Fatalf("non-secret migration contents = %q, error = %v", contents, operationError)
	}
}
