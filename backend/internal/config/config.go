package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/compat"
	providerfactory "github.com/rezoch340/any-aicli-remote/backend/internal/provider/factory"
)

const (
	DefaultPort      = 2421
	DefaultAgentPort = 2419
)

type Config struct {
	Bind              string
	Port              int
	AgentHost         string
	AgentPort         int
	PairingSecret     string
	PairingSecretFile string
	AgentSecret       string
	AgentSecretFile   string
	RuntimeDirectory  string
	PublicHost        string
	ProviderID        string
	ProviderPath      string
	DataDirectory     string
	EnsureAgent       bool
	StopAgentOnExit   bool
	ProviderOptions   map[string]string
}

func Parse(arguments []string) (Config, error) {
	home, operationError := os.UserHomeDir()
	if operationError != nil {
		return Config{}, fmt.Errorf("resolve home: %w", operationError)
	}
	dataDirectory := compat.Environment("ANY_AI_CLI_REMOTE_DATA_DIR", filepath.Join(home, ".any-aicli-remote"))

	configuration := Config{
		Bind:              compat.Environment("ANY_AI_CLI_REMOTE_BIND", "0.0.0.0"),
		Port:              environmentInteger("ANY_AI_CLI_REMOTE_PORT", DefaultPort),
		AgentHost:         compat.Environment("ANY_AI_CLI_REMOTE_AGENT_HOST", "127.0.0.1"),
		AgentPort:         environmentInteger("ANY_AI_CLI_REMOTE_AGENT_PORT", DefaultAgentPort),
		PairingSecret:     compat.Environment("ANY_AI_CLI_REMOTE_PAIRING_SECRET", ""),
		PairingSecretFile: compat.Environment("ANY_AI_CLI_REMOTE_PAIRING_SECRET_FILE", ""),
		AgentSecret:       compat.Environment("ANY_AI_CLI_REMOTE_AGENT_SECRET", ""),
		AgentSecretFile:   compat.Environment("ANY_AI_CLI_REMOTE_AGENT_SECRET_FILE", ""),
		RuntimeDirectory:  compat.Environment("ANY_AI_CLI_REMOTE_RUNTIME_DIR", filepath.Join(dataDirectory, "run")),
		PublicHost:        compat.Environment("ANY_AI_CLI_REMOTE_PUBLIC_HOST", ""),
		ProviderID:        compat.Environment("ANY_AI_CLI_REMOTE_PROVIDER", providerfactory.DefaultProviderID),
		ProviderPath:      compat.Environment("ANY_AI_CLI_REMOTE_PROVIDER_PATH", ""),
		DataDirectory:     dataDirectory,
		EnsureAgent:       compat.BooleanEnvironment("ANY_AI_CLI_REMOTE_ENSURE_AGENT", true),
		StopAgentOnExit:   compat.BooleanEnvironment("ANY_AI_CLI_REMOTE_STOP_AGENT_ON_EXIT", false),
	}
	providerOptions := providerfactory.NewOptionParser()

	flagSet := flag.NewFlagSet("any-aicli-remote-daemon", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	flagSet.StringVar(&configuration.Bind, "bind", configuration.Bind, "HTTP bind address")
	flagSet.IntVar(&configuration.Port, "port", configuration.Port, "HTTP/WebSocket port")
	flagSet.StringVar(&configuration.AgentHost, "agent-host", configuration.AgentHost, "provider agent host")
	flagSet.IntVar(&configuration.AgentPort, "agent-port", configuration.AgentPort, "provider agent port")
	flagSet.StringVar(&configuration.PairingSecretFile, "pairing-secret-file", configuration.PairingSecretFile, "device pairing secret file")
	flagSet.StringVar(&configuration.PairingSecretFile, "secret-file", configuration.PairingSecretFile, "deprecated alias for --pairing-secret-file")
	flagSet.StringVar(&configuration.AgentSecretFile, "agent-secret-file", configuration.AgentSecretFile, "local provider-agent transport secret file")
	flagSet.StringVar(&configuration.RuntimeDirectory, "runtime-dir", configuration.RuntimeDirectory, "neutral daemon runtime directory")
	legacyWorkingDirectory := compat.Environment("ANY_AI_CLI_REMOTE_CWD", "")
	flagSet.StringVar(&legacyWorkingDirectory, "cwd", legacyWorkingDirectory, "deprecated compatibility option; a workspace is selected only when creating a session")
	flagSet.StringVar(&configuration.PublicHost, "public-host", configuration.PublicHost, "public pairing host")
	flagSet.StringVar(&configuration.ProviderID, "provider", configuration.ProviderID, "CLI provider identifier")
	flagSet.StringVar(&configuration.ProviderPath, "provider-path", configuration.ProviderPath, "path to provider CLI")
	flagSet.StringVar(&configuration.DataDirectory, "data-dir", configuration.DataDirectory, "daemon data directory")
	flagSet.BoolVar(&configuration.EnsureAgent, "ensure-agent", configuration.EnsureAgent, "start the provider agent when needed")
	flagSet.BoolVar(&configuration.StopAgentOnExit, "stop-agent-on-exit", configuration.StopAgentOnExit, "stop the owned provider agent when the daemon exits")
	providerOptions.BindFlags(flagSet, &configuration.ProviderPath)
	if operationError := flagSet.Parse(arguments); operationError != nil {
		return Config{}, operationError
	}
	if configuration.Port < 1 || configuration.Port > 65535 || configuration.AgentPort < 1 || configuration.AgentPort > 65535 {
		return Config{}, errors.New("ports must be between 1 and 65535")
	}
	if configuration.Port == configuration.AgentPort {
		return Config{}, errors.New("HTTP and agent ports must differ")
	}

	configuration.DataDirectory = expandHome(configuration.DataDirectory, home)
	configuration.RuntimeDirectory, operationError = filepath.Abs(expandHome(configuration.RuntimeDirectory, home))
	if operationError != nil {
		return Config{}, fmt.Errorf("resolve runtime directory: %w", operationError)
	}
	configuration.ProviderPath = expandHome(configuration.ProviderPath, home)
	configuration.ProviderOptions = providerOptions.Values()
	for optionName, optionValue := range configuration.ProviderOptions {
		configuration.ProviderOptions[optionName] = expandHome(optionValue, home)
	}
	migratePairingSecret := configuration.PairingSecret == "" && configuration.PairingSecretFile == ""
	if operationError := compat.MigrateDataFiles(configuration.DataDirectory, home, migratePairingSecret); operationError != nil {
		return Config{}, operationError
	}
	if configuration.PairingSecretFile == "" {
		configuration.PairingSecretFile = filepath.Join(configuration.DataDirectory, "pairing-secret")
	} else {
		configuration.PairingSecretFile = expandHome(configuration.PairingSecretFile, home)
	}
	if configuration.AgentSecretFile == "" {
		configuration.AgentSecretFile = filepath.Join(configuration.DataDirectory, "agent-transport-secret")
	} else {
		configuration.AgentSecretFile = expandHome(configuration.AgentSecretFile, home)
	}
	if configuration.PairingSecret == "" {
		configuration.PairingSecret, operationError = loadOrCreateSecret(configuration.PairingSecretFile)
		if operationError != nil {
			return Config{}, operationError
		}
	}
	if configuration.AgentSecret == "" {
		configuration.AgentSecret, operationError = loadOrCreateSecret(configuration.AgentSecretFile)
		if operationError != nil {
			return Config{}, operationError
		}
	}
	if len(configuration.PairingSecret) < 16 {
		return Config{}, errors.New("pairing secret must contain at least 16 characters")
	}
	if len(configuration.AgentSecret) < 16 {
		return Config{}, errors.New("agent transport secret must contain at least 16 characters")
	}
	return configuration, nil
}

func (configuration Config) PairingURL(lanIP string) string {
	base := strings.TrimSpace(configuration.PublicHost)
	publicURL := strings.Contains(base, "://")
	if base == "" {
		base = "http://" + net.JoinHostPort(lanIP, strconv.Itoa(configuration.Port))
	} else if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	parsed, operationError := url.Parse(base)
	if operationError != nil || parsed.Hostname() == "" {
		return ""
	}
	if parsed.Port() == "" && !publicURL && configuration.Port != defaultPortForScheme(parsed.Scheme) {
		parsed.Host = net.JoinHostPort(parsed.Hostname(), strconv.Itoa(configuration.Port))
	}
	parsed.Path = "/"
	query := parsed.Query()
	query.Set("key", configuration.PairingSecret)
	query.Set("auto", "1")
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

// PairingDeepLink opens the native mobile client directly when it is installed.
// Device pairing intentionally contains no project workspace. Existing sessions
// recover their directory and new sessions supply one explicitly.
func (configuration Config) PairingDeepLink(lanIP string) string {
	pairingURL := configuration.PairingURL(lanIP)
	if pairingURL == "" {
		return ""
	}
	base, operationError := url.Parse(pairingURL)
	if operationError != nil {
		return ""
	}
	base.RawQuery = ""
	query := url.Values{"url": {base.String()}, "key": {configuration.PairingSecret}}
	return "anyaicliremote://pair?" + query.Encode()
}

func defaultPortForScheme(scheme string) int {
	if strings.EqualFold(scheme, "https") {
		return 443
	}
	return 80
}

func environmentInteger(primaryKey string, fallback int) int {
	value, operationError := strconv.Atoi(compat.Environment(primaryKey, ""))
	if operationError != nil || value == 0 {
		return fallback
	}
	return value
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func loadOrCreateSecret(path string) (string, error) {
	information, operationError := os.Lstat(path)
	if operationError == nil {
		if !information.Mode().IsRegular() {
			return "", errors.New("secret path must be a regular file")
		}
		data, readError := os.ReadFile(path)
		if readError != nil {
			return "", fmt.Errorf("read secret: %w", readError)
		}
		value := strings.TrimSpace(string(data))
		if len(value) < 16 {
			return "", errors.New("secret file must contain at least 16 characters")
		}
		if permissionError := os.Chmod(path, 0o600); permissionError != nil {
			return "", fmt.Errorf("protect secret: %w", permissionError)
		}
		return value, nil
	}
	if !errors.Is(operationError, os.ErrNotExist) {
		return "", fmt.Errorf("inspect secret: %w", operationError)
	}
	if operationError := os.MkdirAll(filepath.Dir(path), 0700); operationError != nil {
		return "", fmt.Errorf("create secret directory: %w", operationError)
	}
	bytes := make([]byte, 16)
	if _, operationError := rand.Read(bytes); operationError != nil {
		return "", fmt.Errorf("generate secret: %w", operationError)
	}
	value := hex.EncodeToString(bytes)
	secretFile, operationError := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if operationError != nil {
		return "", fmt.Errorf("create secret: %w", operationError)
	}
	if _, operationError := secretFile.WriteString(value + "\n"); operationError != nil {
		_ = secretFile.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write secret: %w", operationError)
	}
	if operationError := secretFile.Close(); operationError != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close secret: %w", operationError)
	}
	return value, nil
}
