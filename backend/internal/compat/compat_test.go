package compat

import (
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
