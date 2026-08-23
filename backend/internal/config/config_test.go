package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPairingSecret = "0123456789abcdef"

func TestLoadOrCreateSecret(testContext *testing.T) {
	path := filepath.Join(testContext.TempDir(), "nested", ".secret")
	first, operationError := loadOrCreateSecret(path)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(first) != 32 {
		testContext.Fatalf("secret length = %d", len(first))
	}
	second, operationError := loadOrCreateSecret(path)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if first != second {
		testContext.Fatal("secret was not stable")
	}
	info, operationError := os.Stat(path)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if info.Mode().Perm() != 0600 {
		testContext.Fatalf("secret mode = %o", info.Mode().Perm())
	}
}

func TestLoadOrCreateSecretProtectsExistingFileAndRejectsUnsafePaths(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, "existing-secret")
	if operationError := os.WriteFile(path, []byte(testPairingSecret), 0o644); operationError != nil {
		testContext.Fatal(operationError)
	}
	value, operationError := loadOrCreateSecret(path)
	if operationError != nil || value != testPairingSecret {
		testContext.Fatalf("existing secret = %q, error = %v", value, operationError)
	}
	information, operationError := os.Stat(path)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if information.Mode().Perm() != 0o600 {
		testContext.Fatalf("existing secret mode = %o", information.Mode().Perm())
	}

	symlinkPath := filepath.Join(directory, "secret-link")
	if operationError := os.Symlink(path, symlinkPath); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := loadOrCreateSecret(symlinkPath); operationError == nil {
		testContext.Fatal("accepted symlink secret path")
	}
	invalidPath := filepath.Join(directory, "invalid-secret")
	if operationError := os.WriteFile(invalidPath, []byte("short"), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := loadOrCreateSecret(invalidPath); operationError == nil {
		testContext.Fatal("accepted short secret file")
	}
}

func TestPairingURL(testContext *testing.T) {
	configuration := Config{Port: 2421, PairingSecret: "0123456789abcdef"}
	if got := configuration.PairingURL("192.0.2.44"); got != "http://192.0.2.44:2421/?auto=1&key=0123456789abcdef" {
		testContext.Fatal(got)
	}
	configuration.PublicHost = "https://remote.example:24443"
	if got := configuration.PairingURL("ignored"); !strings.HasPrefix(got, "https://remote.example:24443/") {
		testContext.Fatal(got)
	}
	configuration.PublicHost = "https://remote.example"
	if got := configuration.PairingURL("ignored"); !strings.HasPrefix(got, "https://remote.example/") {
		testContext.Fatal(got)
	}
	configuration.PublicHost = "remote.example"
	if got := configuration.PairingURL("ignored"); !strings.HasPrefix(got, "http://remote.example:2421/") {
		testContext.Fatal(got)
	}
}

func TestPairingDeepLink(testContext *testing.T) {
	configuration := Config{Port: 2421, PairingSecret: "0123456789abcdef"}
	got := configuration.PairingDeepLink("192.0.2.44")
	if !strings.HasPrefix(got, "anyaicliremote://pair?") || strings.Contains(got, "cwd=") {
		testContext.Fatal(got)
	}
}

func TestParseGeneratesIndependentSecretsAndIgnoresGrokAgentSecretForPairing(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	dataDirectory := filepath.Join(homeDirectory, "daemon-data")
	testContext.Setenv("HOME", homeDirectory)
	testContext.Setenv("ANY_AI_CLI_REMOTE_DATA_DIR", dataDirectory)
	testContext.Setenv("ANY_AI_CLI_REMOTE_PAIRING_SECRET", "")
	testContext.Setenv("ANY_AI_CLI_REMOTE_AGENT_SECRET", "")
	testContext.Setenv("GROK_AGENT_SECRET", "provider-owned-secret-value")

	configuration, operationError := Parse(nil)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if configuration.PairingSecret == "provider-owned-secret-value" {
		testContext.Fatal("GROK_AGENT_SECRET was exposed as the device pairing secret")
	}
	if configuration.AgentSecret == "provider-owned-secret-value" {
		testContext.Fatal("GROK_AGENT_SECRET was reused as the daemon transport secret")
	}
	if configuration.PairingSecret == configuration.AgentSecret {
		testContext.Fatal("pairing and provider-agent transport secrets are identical")
	}
	if configuration.PairingSecretFile != filepath.Join(dataDirectory, "pairing-secret") || configuration.AgentSecretFile != filepath.Join(dataDirectory, "agent-transport-secret") {
		testContext.Fatalf("secret files = %q and %q", configuration.PairingSecretFile, configuration.AgentSecretFile)
	}
}

func TestParseUsesCurrentPairingAndAgentSecretEnvironmentIndependently(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	testContext.Setenv("HOME", homeDirectory)
	testContext.Setenv("ANY_AI_CLI_REMOTE_DATA_DIR", filepath.Join(homeDirectory, "daemon-data"))
	testContext.Setenv("ANY_AI_CLI_REMOTE_PAIRING_SECRET", "current-pairing-secret")
	testContext.Setenv("ANY_AI_CLI_REMOTE_AGENT_SECRET", "current-transport-secret")
	testContext.Setenv("GROK_AGENT_SECRET", "provider-owned-secret-value")

	configuration, operationError := Parse(nil)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if configuration.PairingSecret != "current-pairing-secret" || configuration.AgentSecret != "current-transport-secret" {
		testContext.Fatalf("secrets = pairing %q, agent %q", configuration.PairingSecret, configuration.AgentSecret)
	}
}

func TestParseStoresProviderConfigurationInGenericOptions(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	testContext.Setenv("HOME", homeDirectory)
	testContext.Setenv("ANY_AI_CLI_REMOTE_DATA_DIR", filepath.Join(homeDirectory, "daemon-data"))
	testContext.Setenv("ANY_AI_CLI_REMOTE_PAIRING_SECRET", testPairingSecret)
	testContext.Setenv("ANY_AI_CLI_REMOTE_AGENT_SECRET", "0123456789abcdef-agent")

	configuration, operationError := Parse([]string{
		"--provider-sessions-dir", "~/provider-sessions",
		"--provider-always-approve=false",
		"--provider-leader=true",
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if configuration.ProviderOptions["sessions-directory"] != filepath.Join(homeDirectory, "provider-sessions") || configuration.ProviderOptions["always-approve"] != "false" || configuration.ProviderOptions["leader"] != "true" {
		testContext.Fatalf("provider options = %#v", configuration.ProviderOptions)
	}
}

func TestParseRejectsSecretsInProcessArguments(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	testContext.Setenv("HOME", homeDirectory)
	testContext.Setenv("ANY_AI_CLI_REMOTE_DATA_DIR", filepath.Join(homeDirectory, "daemon-data"))
	for _, flagName := range []string{"--pairing-secret", "--secret", "--agent-secret"} {
		testContext.Run(flagName, func(testContext *testing.T) {
			if _, operationError := Parse([]string{flagName, testPairingSecret}); operationError == nil {
				testContext.Fatalf("accepted plaintext secret flag %s", flagName)
			}
		})
	}
}

func TestParseExplicitSecretFileDoesNotPersistLegacyPairingSecret(testContext *testing.T) {
	homeDirectory := testContext.TempDir()
	dataDirectory := filepath.Join(homeDirectory, ".any-aicli-remote")
	legacyDirectory := filepath.Join(homeDirectory, ".grok", "plugin-data", "grok-remote")
	if operationError := os.MkdirAll(legacyDirectory, 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(filepath.Join(legacyDirectory, ".ui-secret"), []byte("legacy-pairing-secret"), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	explicitSecretFile := filepath.Join(homeDirectory, "launcher-secret")
	if operationError := os.WriteFile(explicitSecretFile, []byte(testPairingSecret), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Setenv("HOME", homeDirectory)
	testContext.Setenv("ANY_AI_CLI_REMOTE_DATA_DIR", dataDirectory)
	testContext.Setenv("ANY_AI_CLI_REMOTE_AGENT_SECRET", "0123456789abcdef-agent")

	configuration, operationError := Parse([]string{"--pairing-secret-file", explicitSecretFile})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if configuration.PairingSecret != testPairingSecret {
		testContext.Fatalf("pairing secret = %q", configuration.PairingSecret)
	}
	if _, operationError := os.Stat(filepath.Join(dataDirectory, "pairing-secret")); !os.IsNotExist(operationError) {
		testContext.Fatalf("legacy pairing secret was persisted: %v", operationError)
	}
}
