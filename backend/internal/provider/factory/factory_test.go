package factory

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
)

func TestFactoryBuildsCompleteGrokComponents(testContext *testing.T) {
	sessionsDirectory := filepath.Join(testContext.TempDir(), "sessions")
	components, operationError := New(Configuration{
		ProviderID: DefaultProviderID,
		Options: map[string]string{
			SessionsDirectoryOption: sessionsDirectory,
			AlwaysApproveOption:     "false",
			LeaderOption:            "true",
		},
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	roots := components.SkillRoots.SkillRoots("/workspace")
	if len(roots.Roots) == 0 || components.Catalog.ID() != DefaultProviderID {
		testContext.Fatalf("components = %#v roots = %#v", components, roots)
	}
}

func TestFactoryRequiresExplicitAutomaticApproval(testContext *testing.T) {
	executablePath := filepath.Join(testContext.TempDir(), "grok")
	if operationError := os.WriteFile(executablePath, []byte("#!/bin/sh\n"), 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	testCases := []struct {
		name           string
		options        map[string]string
		expectsApprove bool
	}{
		{name: "default disabled"},
		{name: "explicit enabled", options: map[string]string{AlwaysApproveOption: "true"}, expectsApprove: true},
	}
	for _, testCase := range testCases {
		testContext.Run(testCase.name, func(testContext *testing.T) {
			components, operationError := New(Configuration{
				ProviderID: DefaultProviderID, ExecutablePath: executablePath, Options: testCase.options,
			})
			if operationError != nil {
				testContext.Fatal(operationError)
			}
			command, operationError := components.Protocol.AgentCommand(providerapi.AgentLaunchConfiguration{
				Host: "127.0.0.1", Port: 2419, Secret: "provider-transport-secret",
			})
			if operationError != nil {
				testContext.Fatal(operationError)
			}
			if slices.Contains(command.Arguments, "--always-approve") != testCase.expectsApprove {
				testContext.Fatalf("arguments = %#v", command.Arguments)
			}
		})
	}
}

func TestOptionParserOwnsProviderFlagsAndCompatibilityAliases(testContext *testing.T) {
	testContext.Setenv("ANY_AI_CLI_REMOTE_PROVIDER_SESSIONS_DIR", "")
	testContext.Setenv("ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR", "")
	testContext.Setenv("ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE", "")
	testContext.Setenv("ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE", "")
	testContext.Setenv("ANY_AI_CLI_REMOTE_PROVIDER_LEADER", "")
	testContext.Setenv("ANY_AI_CLI_REMOTE_GROK_LEADER", "")
	testContext.Setenv("GROK_REMOTE_SESSIONS_DIR", "")
	testContext.Setenv("GROK_REMOTE_ALWAYS_APPROVE", "")
	testContext.Setenv("GROK_REMOTE_LEADER", "")
	parser := NewOptionParser()
	executablePath := ""
	flagSet := flag.NewFlagSet("provider-options", flag.ContinueOnError)
	parser.BindFlags(flagSet, &executablePath)
	if operationError := flagSet.Parse([]string{
		"--grok", "/provider-cli",
		"--grok-sessions-dir", "/provider-sessions",
		"--provider-always-approve=false",
		"--provider-leader=true",
	}); operationError != nil {
		testContext.Fatal(operationError)
	}
	options := parser.Values()
	if executablePath != "/provider-cli" || options[SessionsDirectoryOption] != "/provider-sessions" || options[AlwaysApproveOption] != "false" || options[LeaderOption] != "true" {
		testContext.Fatalf("path=%q options=%#v", executablePath, options)
	}
}

func TestFactoryRejectsInvalidProviderConfiguration(testContext *testing.T) {
	if _, operationError := New(Configuration{ProviderID: "missing"}); operationError == nil {
		testContext.Fatal("unsupported provider was accepted")
	}
	if _, operationError := New(Configuration{Options: map[string]string{LeaderOption: "perhaps"}}); operationError == nil {
		testContext.Fatal("invalid boolean option was accepted")
	}
}
