package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
)

type smokePaths struct {
	homeDirectory     string
	rootDirectory     string
	configurationPath string
	secretPath        string
	dataDirectory     string
	runtimeDirectory  string
}

func newSmokePaths(testingContext *testing.T) smokePaths {
	testingContext.Helper()
	homeDirectory := testingContext.TempDir()
	rootDirectory := testingContext.TempDir()
	return smokePaths{
		homeDirectory:     homeDirectory,
		rootDirectory:     rootDirectory,
		configurationPath: filepath.Join(rootDirectory, "config.json"),
		secretPath:        filepath.Join(rootDirectory, "pairing-secret"),
		dataDirectory:     filepath.Join(rootDirectory, "data"),
		runtimeDirectory:  filepath.Join(rootDirectory, "runtime"),
	}
}

func resetSmokeEnvironment(testingContext *testing.T, homeDirectory string) {
	testingContext.Helper()
	for _, environmentName := range []string{
		"ANY_AI_CLI_REMOTE_CONFIG",
		"ANY_AI_CLI_REMOTE_BIND",
		"ANY_AI_CLI_REMOTE_PORT",
		"ANY_AI_CLI_REMOTE_AGENT_HOST",
		"ANY_AI_CLI_REMOTE_AGENT_PORT",
		"ANY_AI_CLI_REMOTE_PAIRING_SECRET",
		"ANY_AI_CLI_REMOTE_PAIRING_SECRET_FILE",
		"ANY_AI_CLI_REMOTE_AGENT_SECRET",
		"ANY_AI_CLI_REMOTE_AGENT_SECRET_FILE",
		"ANY_AI_CLI_REMOTE_RUNTIME_DIR",
		"ANY_AI_CLI_REMOTE_PUBLIC_HOST",
		"ANY_AI_CLI_REMOTE_PROVIDER",
		"ANY_AI_CLI_REMOTE_PROVIDER_PATH",
		"ANY_AI_CLI_REMOTE_DATA_DIR",
		"ANY_AI_CLI_REMOTE_ENSURE_AGENT",
		"ANY_AI_CLI_REMOTE_STOP_AGENT_ON_EXIT",
		"ANY_AI_CLI_REMOTE_PROVIDER_SESSIONS_DIR",
		"ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE",
		"ANY_AI_CLI_REMOTE_PROVIDER_LEADER",
		"ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR",
		"ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE",
		"ANY_AI_CLI_REMOTE_GROK_LEADER",
		"ANY_AI_CLI_REMOTE_CWD",
		"GROK_PLUGIN_DATA",
		"GROK_REMOTE_CONFIG",
		"GROK_REMOTE_BIND",
		"GROK_REMOTE_PORT",
		"GROK_REMOTE_AGENT_HOST",
		"GROK_REMOTE_AGENT_PORT",
		"GROK_REMOTE_SECRET_FILE",
		"GROK_REMOTE_RUNTIME_DIR",
		"GROK_REMOTE_PUBLIC_HOST",
		"GROK_REMOTE_PROVIDER",
		"GROK_REMOTE_GROK_PATH",
		"GROK_REMOTE_SESSIONS_DIR",
		"GROK_REMOTE_ENSURE_AGENT",
		"GROK_REMOTE_STOP_AGENT_ON_EXIT",
		"GROK_REMOTE_ALWAYS_APPROVE",
		"GROK_REMOTE_LEADER",
		"GROK_REMOTE_CWD",
	} {
		testingContext.Setenv(environmentName, "")
	}
	testingContext.Setenv("HOME", homeDirectory)
}

func distinctSmokePorts(testingContext *testing.T) [2]int {
	testingContext.Helper()
	firstPort := reserveSmokePort(testingContext)
	secondPort := reserveSmokePort(testingContext)
	for secondPort == firstPort {
		secondPort = reserveSmokePort(testingContext)
	}
	return [2]int{firstPort, secondPort}
}

func reserveSmokePort(testingContext *testing.T) int {
	testingContext.Helper()
	listener, operationError := net.Listen("tcp", "127.0.0.1:0")
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	defer func() {
		if closeError := listener.Close(); closeError != nil {
			testingContext.Error(closeError)
		}
	}()
	tcpAddress, validAddress := listener.Addr().(*net.TCPAddr)
	if !validAddress {
		testingContext.Fatalf("unexpected listener address %T", listener.Addr())
	}
	return tcpAddress.Port
}

func prepareSmokeDocument(testingContext *testing.T, paths smokePaths, listenerPorts [2]int) config.Document {
	testingContext.Helper()
	showOutput := new(bytes.Buffer)
	showError := new(bytes.Buffer)
	if exitCode := run(
		[]string{"config", "show", "--config", paths.configurationPath},
		strings.NewReader(""), showOutput, showError,
	); exitCode != exitSuccess {
		testingContext.Fatalf("config show failed (%d): %s", exitCode, showError.String())
	}
	assertNoBootstrapState(testingContext, paths)
	var document config.Document
	if operationError := json.Unmarshal(bytes.TrimSpace(showOutput.Bytes()), &document); operationError != nil {
		testingContext.Fatal(operationError)
	}
	document.Network.Bind = "127.0.0.1"
	document.Network.Port = listenerPorts[0]
	document.Network.PublicHost = ""
	document.Agent.Host = "127.0.0.1"
	document.Agent.Port = listenerPorts[1]
	document.Agent.Ensure = false
	document.Agent.StopOnExit = true
	document.Storage.DataDirectory = paths.dataDirectory
	document.Storage.RuntimeDirectory = paths.runtimeDirectory
	return document
}

func assertNoBootstrapState(testingContext *testing.T, paths smokePaths) {
	testingContext.Helper()
	for _, targetPath := range []string{
		paths.configurationPath,
		paths.dataDirectory,
		paths.runtimeDirectory,
		paths.secretPath,
		filepath.Join(paths.dataDirectory, "pairing-secret"),
		filepath.Join(paths.dataDirectory, "agent-transport-secret"),
		filepath.Join(paths.homeDirectory, ".grok"),
	} {
		if _, operationError := os.Lstat(targetPath); !os.IsNotExist(operationError) {
			testingContext.Fatalf("bootstrap unexpectedly created %s: %v", targetPath, operationError)
		}
	}
}

func applySmokeDocument(testingContext *testing.T, configurationPath string, document config.Document) {
	testingContext.Helper()
	encodedDocument, operationError := json.Marshal(document)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	validateOutput, validateError := new(bytes.Buffer), new(bytes.Buffer)
	if exitCode := run(
		[]string{"config", "validate", "--config", configurationPath, "--input", "-"},
		bytes.NewReader(encodedDocument), validateOutput, validateError,
	); exitCode != exitSuccess {
		testingContext.Fatalf("config validate failed (%d): %s", exitCode, validateError.String())
	}
	if outputText := strings.TrimSpace(validateOutput.String()); outputText != "valid" {
		testingContext.Fatalf("unexpected validate output %q", outputText)
	}
	applyOutput, applyError := new(bytes.Buffer), new(bytes.Buffer)
	if exitCode := run(
		[]string{"config", "apply", "--config", configurationPath, "--input", "-"},
		bytes.NewReader(encodedDocument), applyOutput, applyError,
	); exitCode != exitSuccess {
		testingContext.Fatalf("config apply failed (%d): %s", exitCode, applyError.String())
	}
	if outputPath := strings.TrimSpace(applyOutput.String()); outputPath != configurationPath {
		testingContext.Fatalf("unexpected applied path %q", outputPath)
	}
}

func assertPrivateFileMode(testingContext *testing.T, targetPath string) {
	testingContext.Helper()
	information, operationError := os.Stat(targetPath)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if mode := information.Mode().Perm(); mode != smokeFileMode {
		testingContext.Fatalf("unexpected permissions for %s: %o", targetPath, mode)
	}
}

func assertSmokeDocumentPreserved(testingContext *testing.T, configurationPath string, expected config.Document) {
	testingContext.Helper()
	showOutput, showError := new(bytes.Buffer), new(bytes.Buffer)
	if exitCode := run(
		[]string{"config", "show", "--config", configurationPath},
		strings.NewReader(""), showOutput, showError,
	); exitCode != exitSuccess {
		testingContext.Fatalf("post-apply config show failed (%d): %s", exitCode, showError.String())
	}
	var actual config.Document
	if operationError := json.Unmarshal(bytes.TrimSpace(showOutput.Bytes()), &actual); operationError != nil {
		testingContext.Fatal(operationError)
	}
	validateOutput, validateError := new(bytes.Buffer), new(bytes.Buffer)
	if exitCode := run(
		[]string{"config", "validate", "--config", configurationPath},
		strings.NewReader(""), validateOutput, validateError,
	); exitCode != exitSuccess {
		testingContext.Fatalf("stored config validate failed (%d): %s", exitCode, validateError.String())
	}
	if outputText := strings.TrimSpace(validateOutput.String()); outputText != "valid" {
		testingContext.Fatalf("unexpected stored validate output %q", outputText)
	}
	if !reflect.DeepEqual(actual.Provider, expected.Provider) {
		testingContext.Fatalf("provider changed after apply: %#v != %#v", actual.Provider, expected.Provider)
	}
	if !reflect.DeepEqual(actual.Tuning, expected.Tuning) {
		testingContext.Fatal("tuning changed after apply")
	}
	if !reflect.DeepEqual(actual.Network, expected.Network) ||
		!reflect.DeepEqual(actual.Agent, expected.Agent) ||
		!reflect.DeepEqual(actual.Storage, expected.Storage) {
		testingContext.Fatal("canonical fields changed after apply")
	}
}

func daemonLaunchArguments(configurationPath, secretPath string) []string {
	return []string{"--config", configurationPath, "--pairing-secret-file", secretPath}
}

func assertLaunchArguments(testingContext *testing.T, configurationPath, secretPath string) {
	testingContext.Helper()
	arguments := daemonLaunchArguments(configurationPath, secretPath)
	expected := []string{"--config", configurationPath, "--pairing-secret-file", secretPath}
	if !reflect.DeepEqual(arguments, expected) {
		testingContext.Fatalf("unexpected launch arguments: %#v", arguments)
	}
	for _, forbiddenArgument := range []string{"--bind", "--port", "--agent-port", "--public-host", "--stop-agent-on-exit"} {
		for _, argument := range arguments {
			if argument == forbiddenArgument {
				testingContext.Fatalf("launch arguments contain %s", forbiddenArgument)
			}
		}
	}
}

func materializeSmokeSecret(testingContext *testing.T, secretPath string) {
	testingContext.Helper()
	secretFile, operationError := os.OpenFile(secretPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, smokeFileMode)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if _, operationError = secretFile.WriteString(smokeSecretValue); operationError != nil {
		closeSmokeFile(testingContext, secretFile)
		testingContext.Fatal(operationError)
	}
	if operationError = secretFile.Chmod(smokeFileMode); operationError != nil {
		closeSmokeFile(testingContext, secretFile)
		testingContext.Fatal(operationError)
	}
	closeSmokeFile(testingContext, secretFile)
	if len(smokeSecretValue) < 16 {
		testingContext.Fatal("smoke secret is too short")
	}
	assertPrivateFileMode(testingContext, secretPath)
}

func closeSmokeFile(testingContext *testing.T, secretFile *os.File) {
	testingContext.Helper()
	if closeError := secretFile.Close(); closeError != nil {
		testingContext.Error(closeError)
	}
}

func assertRuntimeConfigDoesNotPersistSecrets(testingContext *testing.T, dataDirectory string) {
	testingContext.Helper()
	runtimeData, operationError := os.ReadFile(filepath.Join(dataDirectory, "runtime-config.json"))
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if strings.Contains(string(runtimeData), smokeSecretValue) ||
		strings.Contains(string(runtimeData), "pairing_url") ||
		strings.Contains(string(runtimeData), "pairing_deep_link") {
		testingContext.Fatalf("runtime config persisted pairing material: %s", runtimeData)
	}
}

func removeSmokeSecret(testingContext *testing.T, secretPath string) {
	testingContext.Helper()
	if operationError := os.Remove(secretPath); operationError != nil {
		testingContext.Fatal(operationError)
	}
	if _, operationError := os.Stat(secretPath); !os.IsNotExist(operationError) {
		testingContext.Fatalf("pairing secret still exists: %v", operationError)
	}
}

func assertPortAvailable(testingContext *testing.T, port int) {
	testingContext.Helper()
	listener, operationError := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if operationError != nil {
		testingContext.Fatalf("agent port %d remained bound: %v", port, operationError)
	}
	if closeError := listener.Close(); closeError != nil {
		testingContext.Fatal(closeError)
	}
}

func assertNoSessionOrWorkspaceState(testingContext *testing.T, paths smokePaths) {
	testingContext.Helper()
	for _, rootPath := range []string{paths.dataDirectory, paths.runtimeDirectory} {
		walkError := filepath.WalkDir(rootPath, func(targetPath string, directoryEntry os.DirEntry, operationError error) error {
			if operationError != nil {
				return operationError
			}
			components := strings.FieldsFunc(
				strings.ToLower(filepath.Clean(targetPath)),
				func(character rune) bool { return character == filepath.Separator },
			)
			for _, component := range components {
				if strings.Contains(component, "session") || strings.Contains(component, "workspace") {
					return fmt.Errorf("unexpected state path %s", targetPath)
				}
			}
			return nil
		})
		if walkError != nil && !os.IsNotExist(walkError) {
			testingContext.Fatal(walkError)
		}
	}
}

func assertNoLegacyHomeState(testingContext *testing.T, homeDirectory string) {
	testingContext.Helper()
	if _, operationError := os.Stat(filepath.Join(homeDirectory, ".grok")); !os.IsNotExist(operationError) {
		testingContext.Fatalf("legacy home state exists: %v", operationError)
	}
}
