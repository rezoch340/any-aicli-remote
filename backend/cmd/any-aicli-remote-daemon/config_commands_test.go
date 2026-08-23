package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
)

func TestConfigCommandsShowValidateApply(testingContext *testing.T) {
	home := testingContext.TempDir()
	testingContext.Setenv("HOME", home)
	configurationPath := filepath.Join(home, "config.json")
	fileDocument := config.DefaultDocument(home)
	fileDocument.Network.Bind = "file-bind"
	if errorValue := config.SaveDocument(configurationPath, fileDocument); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	testingContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
	testingContext.Setenv("ANY_AI_CLI_REMOTE_BIND", "environment-bind")
	testingContext.Setenv("ANY_AI_CLI_REMOTE_PAIRING_SECRET", "must-not-appear")
	var output, diagnostics bytes.Buffer
	if code := run([]string{"config", "show", "--bind", "flag-bind"}, strings.NewReader(""), &output, &diagnostics); code != exitSuccess {
		testingContext.Fatalf("show=%d %s", code, diagnostics.String())
	}
	var document config.Document
	if errorValue := json.Unmarshal(output.Bytes(), &document); errorValue != nil || document.Network.Bind != "flag-bind" {
		testingContext.Fatalf("show=%q err=%v", output.String(), errorValue)
	}
	if strings.Contains(output.String(), "must-not-appear") || strings.Contains(output.String(), "secret") {
		testingContext.Fatal("show leaked secret")
	}
	if _, errorValue := os.Stat(filepath.Join(home, ".any-aicli-remote")); !os.IsNotExist(errorValue) {
		testingContext.Fatalf("side effect: %v", errorValue)
	}
	output.Reset()
	diagnostics.Reset()
	if code := run([]string{"config", "validate"}, strings.NewReader(""), &output, &diagnostics); code != exitSuccess || output.String() != "valid\n" {
		testingContext.Fatalf("validate=%d %q %q", code, output.String(), diagnostics.String())
	}
	candidate, errorValue := json.Marshal(config.DefaultDocument(home))
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	target := filepath.Join(home, "nested", "applied.json")
	output.Reset()
	if code := run([]string{"config", "apply", "-config=" + target, "--input", "-"}, bytes.NewReader(candidate), &output, &diagnostics); code != exitSuccess || strings.TrimSpace(output.String()) != target {
		testingContext.Fatalf("apply=%d %q %q", code, output.String(), diagnostics.String())
	}
	information, errorValue := os.Stat(target)
	if errorValue != nil || information.Mode().Perm() != 0600 {
		testingContext.Fatalf("mode=%v err=%v", information, errorValue)
	}
	if _, errorValue = config.LoadDocument(target, home); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	dataDirectory := filepath.Join(home, ".any-aicli-remote")
	for _, statePath := range []string{"pairing-secret", "agent-transport-secret", "agent-state.json", "runtime-config.json", "loops.json", "run", "logs"} {
		if _, stateError := os.Stat(filepath.Join(dataDirectory, statePath)); !os.IsNotExist(stateError) {
			testingContext.Fatalf("unexpected state %s: %v", statePath, stateError)
		}
	}
	if _, stateError := os.Stat(filepath.Join(home, ".grok")); !os.IsNotExist(stateError) {
		testingContext.Fatalf("unexpected legacy state: %v", stateError)
	}
}
func TestConfigCommandFailuresAndHelp(testingContext *testing.T) {
	var output, diagnostics bytes.Buffer
	for _, arguments := range [][]string{{"config"}, {"config", "unknown"}} {
		output.Reset()
		diagnostics.Reset()
		if code := run(arguments, strings.NewReader(""), &output, &diagnostics); code != exitUsage {
			testingContext.Fatalf("%v=%d", arguments, code)
		}
	}
	for _, arguments := range [][]string{{"config", "show", "--help"}, {"config", "validate", "--help"}, {"config", "apply", "--help"}} {
		output.Reset()
		if code := run(arguments, strings.NewReader(""), &output, &diagnostics); code != exitSuccess {
			testingContext.Fatalf("%v=%d", arguments, code)
		}
	}
	if code := run([]string{"config", "validate", "--input", filepath.Join(testingContext.TempDir(), "missing")}, strings.NewReader(""), &output, &diagnostics); code != exitInternal {
		testingContext.Fatalf("missing=%d", code)
	}
}
func TestConfigApplyBadCandidatePreservesFile(testingContext *testing.T) {
	home := testingContext.TempDir()
	target := filepath.Join(home, "config.json")
	original := []byte("original")
	if errorValue := os.WriteFile(target, original, 0600); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	before, _ := os.Stat(target)
	time.Sleep(time.Millisecond)
	var output, diagnostics bytes.Buffer
	if code := run([]string{"config", "apply", "--config", target, "--input", "-"}, strings.NewReader("{"), &output, &diagnostics); code != exitUsage {
		testingContext.Fatalf("code=%d", code)
	}
	after, _ := os.Stat(target)
	got, _ := os.ReadFile(target)
	if string(got) != string(original) || !after.ModTime().Equal(before.ModTime()) {
		testingContext.Fatal("candidate modified target")
	}
}

func TestConfigApplyRequiresExplicitConfigAndResolveIO(testingContext *testing.T) {
	home := testingContext.TempDir()
	testingContext.Setenv("HOME", home)
	candidate, errorValue := json.Marshal(config.DefaultDocument(home))
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	var output, diagnostics bytes.Buffer
	if code := run([]string{"config", "apply", "--input", "-"}, bytes.NewReader(candidate), &output, &diagnostics); code != exitUsage {
		testingContext.Fatalf("missing config=%d", code)
	}
	if _, errorValue = os.Stat(filepath.Join(home, ".any-aicli-remote", "config.json")); !os.IsNotExist(errorValue) {
		testingContext.Fatalf("default written: %v", errorValue)
	}
	directory := filepath.Join(home, "directory")
	if errorValue = os.Mkdir(directory, 0700); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	for _, command := range [][]string{{"config", "show", "--config=" + directory}, {"config", "validate", "--config=" + directory}} {
		output.Reset()
		diagnostics.Reset()
		if code := run(command, strings.NewReader(""), &output, &diagnostics); code != exitInternal {
			testingContext.Fatalf("%v = %d: %s", command, code, diagnostics.String())
		}
	}
}
