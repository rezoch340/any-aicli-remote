package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func resetResolverEnvironment(testContext *testing.T, homeDirectory string) {
	testContext.Helper()
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
		"GROK_PLUGIN_DATA",
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
		testContext.Setenv(environmentName, "")
	}
	testContext.Setenv("HOME", homeDirectory)
}

func saveResolverDocument(testContext *testing.T, homeDirectory string, mutate func(*Document)) string {
	testContext.Helper()
	document := DefaultDocument(homeDirectory)
	mutate(&document)
	configurationPath := filepath.Join(homeDirectory, "configuration", "config.json")
	if operationError := SaveDocument(configurationPath, document); operationError != nil {
		testContext.Fatal(operationError)
	}
	return configurationPath
}

func TestResolveSourcePrecedence(testContext *testing.T) {
	testCases := []struct {
		name       string
		configure  func(*testing.T, string)
		wantBind   string
		wantPort   int
		wantEnsure bool
	}{
		{
			name: "file overrides defaults",
			configure: func(testContext *testing.T, configurationPath string) {
				testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
			},
			wantBind: "file-bind", wantPort: 2601, wantEnsure: false,
		},
		{
			name: "legacy overrides file",
			configure: func(testContext *testing.T, configurationPath string) {
				testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
				testContext.Setenv("GROK_REMOTE_BIND", "legacy-bind")
				testContext.Setenv("GROK_REMOTE_PORT", "2602")
				testContext.Setenv("GROK_REMOTE_ENSURE_AGENT", "true")
			},
			wantBind: "legacy-bind", wantPort: 2602, wantEnsure: true,
		},
		{
			name: "current overrides legacy",
			configure: func(testContext *testing.T, configurationPath string) {
				testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
				testContext.Setenv("GROK_REMOTE_BIND", "legacy-bind")
				testContext.Setenv("GROK_REMOTE_PORT", "2602")
				testContext.Setenv("GROK_REMOTE_ENSURE_AGENT", "true")
				testContext.Setenv("ANY_AI_CLI_REMOTE_BIND", "current-bind")
				testContext.Setenv("ANY_AI_CLI_REMOTE_PORT", "2603")
				testContext.Setenv("ANY_AI_CLI_REMOTE_ENSURE_AGENT", "false")
			},
			wantBind: "current-bind", wantPort: 2603, wantEnsure: false,
		},
		{
			name: "flags override current",
			configure: func(testContext *testing.T, configurationPath string) {
				testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
				testContext.Setenv("ANY_AI_CLI_REMOTE_BIND", "current-bind")
				testContext.Setenv("ANY_AI_CLI_REMOTE_PORT", "2603")
				testContext.Setenv("ANY_AI_CLI_REMOTE_ENSURE_AGENT", "false")
			},
			wantBind: "flag-bind", wantPort: 2604, wantEnsure: true,
		},
	}
	for _, testCase := range testCases {
		testContext.Run(testCase.name, func(testContext *testing.T) {
			homeDirectory := testContext.TempDir()
			resetResolverEnvironment(testContext, homeDirectory)
			configurationPath := saveResolverDocument(testContext, homeDirectory, func(document *Document) {
				document.Network.Bind = "file-bind"
				document.Network.Port = 2601
				document.Agent.Ensure = false
			})
			testCase.configure(testContext, configurationPath)
			arguments := []string(nil)
			if testCase.name == "flags override current" {
				arguments = []string{"--bind", "flag-bind", "--port", "2604", "--ensure-agent=true"}
			}
			configuration, operationError := Resolve(arguments)
			if operationError != nil {
				testContext.Fatal(operationError)
			}
			if configuration.Bind != testCase.wantBind || configuration.Port != testCase.wantPort || configuration.EnsureAgent != testCase.wantEnsure {
				testContext.Fatalf("configuration = %#v", configuration)
			}
		})
	}
}

func TestResolveConfigurationPathPrecedence(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	resetResolverEnvironment(testContext, homeDirectory)
	configurationPaths := []string{
		filepath.Join(homeDirectory, "environment", "config.json"),
		filepath.Join(homeDirectory, "first", "config.json"),
		filepath.Join(homeDirectory, "second", "config.json"),
	}
	for index, configurationPath := range configurationPaths {
		document := DefaultDocument(homeDirectory)
		document.Network.Bind = "configuration-" + string(rune('a'+index))
		if operationError := SaveDocument(configurationPath, document); operationError != nil {
			testContext.Fatal(operationError)
		}
	}
	testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", "~/environment/config.json")
	configuration, operationError := Resolve([]string{"--config", "~/first/config.json", "--config=~/second/config.json"})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	wantPath := filepath.Join(homeDirectory, "second", "config.json")
	if configuration.ConfigurationPath != wantPath || configuration.Bind != "configuration-c" {
		testContext.Fatalf("path = %q, bind = %q", configuration.ConfigurationPath, configuration.Bind)
	}
}

func TestResolveProviderOptionPrecedenceAndCopy(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	resetResolverEnvironment(testContext, homeDirectory)
	configurationPath := saveResolverDocument(testContext, homeDirectory, func(document *Document) {
		document.Provider.Options = map[string]string{
			"always-approve":     "false",
			"sessions-directory": "/file/sessions",
			"extension-setting":  "file-value",
		}
	})
	testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
	testContext.Setenv("GROK_REMOTE_ALWAYS_APPROVE", "false")
	testContext.Setenv("ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE", "true")
	configuration, operationError := Resolve([]string{
		"--provider-always-approve=false",
		"--provider-sessions-dir", "/flag/sessions",
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if configuration.ProviderOptions["always-approve"] != "false" || configuration.ProviderOptions["sessions-directory"] != "/flag/sessions" || configuration.ProviderOptions["extension-setting"] != "file-value" {
		testContext.Fatalf("provider options = %#v", configuration.ProviderOptions)
	}
	configuration.ProviderOptions["extension-setting"] = "runtime-value"
	if configuration.Canonical.Provider.Options["extension-setting"] != "file-value" {
		testContext.Fatalf("canonical options mutated: %#v", configuration.Canonical.Provider.Options)
	}
}

func TestResolveRejectsInvalidProviderBooleanInFile(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	resetResolverEnvironment(testContext, homeDirectory)
	configurationPath := saveResolverDocument(testContext, homeDirectory, func(document *Document) {
		document.Provider.Options = map[string]string{"always-approve": "invalid"}
	})
	testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
	if _, operationError := Resolve(nil); operationError == nil {
		testContext.Fatal("accepted invalid provider boolean from configuration file")
	}
}

func TestResolveCanonicalizesRelativeConfigurationPath(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	resetResolverEnvironment(testContext, homeDirectory)
	configurationDirectory := filepath.Join(homeDirectory, "relative")
	configurationPath := filepath.Join(configurationDirectory, "config.json")
	if operationError := SaveDocument(configurationPath, DefaultDocument(homeDirectory)); operationError != nil {
		testContext.Fatal(operationError)
	}
	workingDirectory := testContext.TempDir()
	relativePath, operationError := filepath.Rel(workingDirectory, configurationPath)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Chdir(workingDirectory)
	configuration, operationError := Resolve([]string{"--config", relativePath})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if configuration.ConfigurationPath != filepath.Clean(configurationPath) {
		testContext.Fatalf("configuration path = %q, want %q", configuration.ConfigurationPath, configurationPath)
	}
}

func TestResolveHasNoFilesystemSideEffects(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	resetResolverEnvironment(testContext, homeDirectory)
	configuration, operationError := Resolve(nil)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if configuration.DataDirectory != filepath.Join(homeDirectory, ".any-aicli-remote") {
		testContext.Fatalf("data directory = %q", configuration.DataDirectory)
	}
	dataDirectory := filepath.Join(homeDirectory, ".any-aicli-remote")
	if _, operationError := os.Stat(dataDirectory); !os.IsNotExist(operationError) {
		testContext.Fatalf("resolve created data, runtime, or secret material: %v", operationError)
	}
}

func TestResolveRejectsInvalidEnvironmentAndPlaintextSecrets(testContext *testing.T) {
	testContext.Run("invalid port", func(testContext *testing.T) {
		homeDirectory := testContext.TempDir()
		resetResolverEnvironment(testContext, homeDirectory)
		testContext.Setenv("ANY_AI_CLI_REMOTE_PORT", "invalid")
		if _, operationError := Resolve(nil); operationError == nil {
			testContext.Fatal("accepted invalid port")
		}
	})
	testContext.Run("invalid daemon boolean", func(testContext *testing.T) {
		homeDirectory := testContext.TempDir()
		resetResolverEnvironment(testContext, homeDirectory)
		testContext.Setenv("ANY_AI_CLI_REMOTE_ENSURE_AGENT", "invalid")
		if _, operationError := Resolve(nil); operationError == nil {
			testContext.Fatal("accepted invalid daemon boolean")
		}
	})
	testContext.Run("invalid provider boolean", func(testContext *testing.T) {
		homeDirectory := testContext.TempDir()
		resetResolverEnvironment(testContext, homeDirectory)
		testContext.Setenv("ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE", "invalid")
		if _, operationError := Resolve(nil); operationError == nil {
			testContext.Fatal("accepted invalid provider boolean")
		}
	})
	for _, secretFlag := range []string{"--pairing-secret", "-pairing-secret", "--secret", "-secret", "--agent-secret", "-agent-secret"} {
		for _, arguments := range [][]string{{secretFlag, "plaintext-value"}, {secretFlag + "=plaintext-value"}} {
			testContext.Run(secretFlag+"/"+arguments[len(arguments)-1], func(testContext *testing.T) {
				homeDirectory := testContext.TempDir()
				resetResolverEnvironment(testContext, homeDirectory)
				if _, operationError := Resolve(arguments); operationError == nil {
					testContext.Fatalf("accepted plaintext secret arguments %#v", arguments)
				}
			})
		}
	}
}

func TestResolveCarriesCanonicalTuning(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	resetResolverEnvironment(testContext, homeDirectory)
	configurationPath := saveResolverDocument(testContext, homeDirectory, func(document *Document) {
		document.Tuning.HTTP.ReadHeaderTimeout = duration("11s")
		document.Tuning.Hub.DialAttempts = 4
		document.Tuning.Hub.PendingTimeout = duration("31m")
		document.Tuning.History.LiveLimit = 350
		document.Tuning.History.LiveMaxBytes = 510000
		document.Tuning.Lifecycle.StackWait = duration("25s")
		document.Tuning.Loops.MaxJobs = 42
	})
	expected, operationError := LoadDocument(configurationPath, homeDirectory)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Setenv("ANY_AI_CLI_REMOTE_CONFIG", configurationPath)
	configuration, operationError := Resolve(nil)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if !reflect.DeepEqual(configuration.Canonical.Tuning, expected.Tuning) {
		testContext.Fatalf("canonical tuning = %#v, expected %#v", configuration.Canonical.Tuning, expected.Tuning)
	}
}
