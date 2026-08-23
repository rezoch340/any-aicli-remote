package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
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
	DefaultPort                = 2421
	DefaultAgentPort           = 2419
	minimumSecretCharacters    = 16
	generatedSecretRandomBytes = 16
	standardHTTPPort           = 80
	standardHTTPSPort          = 443
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
	ConfigurationPath string
	Canonical         Document
}

func Resolve(arguments []string) (Config, error) { return resolve(arguments, true, os.Stderr) }

// ResolveNonSecret resolves effective non-secret settings without inspecting secret environment variables or files.
func ResolveNonSecret(arguments []string) (Config, error) {
	return ResolveNonSecretWithOutput(arguments, io.Discard)
}

// ResolveNonSecretWithOutput resolves non-secret settings and directs flag diagnostics to output.
func ResolveNonSecretWithOutput(arguments []string, output io.Writer) (Config, error) {
	return resolve(arguments, false, output)
}

func resolve(arguments []string, includeSecrets bool, output io.Writer) (Config, error) {
	home, operationError := os.UserHomeDir()
	if operationError != nil {
		return Config{}, fmt.Errorf("resolve home: %w", operationError)
	}
	configurationPath, operationError := ResolveConfigurationPath(arguments)
	if operationError != nil {
		return Config{}, operationError
	}
	document, operationError := LoadDocument(configurationPath, home)
	if operationError != nil {
		return Config{}, fmt.Errorf("load config: %w", operationError)
	}
	if operationError := applyEnvironment(&document); operationError != nil {
		return Config{}, operationError
	}
	providerOptions := providerfactory.NewOptionParserWithValues(document.Provider.Options)
	if operationError := providerOptions.ApplyEnvironment(); operationError != nil {
		return Config{}, operationError
	}
	pairingSecretFile, agentSecretFile := "", ""
	if includeSecrets {
		pairingSecretFile, agentSecretFile = compat.Environment("ANY_AI_CLI_REMOTE_PAIRING_SECRET_FILE", ""), compat.Environment("ANY_AI_CLI_REMOTE_AGENT_SECRET_FILE", "")
	}
	flagSet := flag.NewFlagSet("any-aicli-remote-daemon", flag.ContinueOnError)
	flagSet.SetOutput(output)
	bindFlags(flagSet, &document, providerOptions, &pairingSecretFile, &agentSecretFile, &configurationPath)
	for index, argument := range arguments {
		if isPlaintextSecretArgument(argument) {
			return Config{}, errors.New("plaintext secrets are not accepted")
		}
		if (argument == "--config" || argument == "-config") && (index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "") {
			return Config{}, errors.New("config path cannot be empty")
		}
		if (strings.HasPrefix(argument, "--config=") || strings.HasPrefix(argument, "-config=")) && strings.TrimSpace(argument[strings.IndexByte(argument, '=')+1:]) == "" {
			return Config{}, errors.New("config path cannot be empty")
		}
	}
	if operationError := flagSet.Parse(arguments); operationError != nil {
		return Config{}, operationError
	}
	configurationPath, operationError = ResolveConfigurationPath(arguments)
	if operationError != nil {
		return Config{}, operationError
	}
	document.Provider.Options = providerOptions.Values()
	document = NormalizeDocument(document, home)
	if operationError := ValidateDocument(document); operationError != nil {
		return Config{}, operationError
	}
	configuration := ApplyCanonical(Config{ConfigurationPath: configurationPath, Canonical: document}, document)
	if !includeSecrets {
		return configuration, nil
	}
	configuration.PairingSecret = strings.TrimSpace(os.Getenv("ANY_AI_CLI_REMOTE_PAIRING_SECRET"))
	configuration.AgentSecret = strings.TrimSpace(os.Getenv("ANY_AI_CLI_REMOTE_AGENT_SECRET"))
	configuration.PairingSecretFile, configuration.AgentSecretFile = pairingSecretFile, agentSecretFile
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
	return configuration, nil
}

// ResolveConfigurationPath returns the canonical path selected by environment and config flags.
func ResolveConfigurationPath(arguments []string) (string, error) {
	home, operationError := os.UserHomeDir()
	if operationError != nil {
		return "", fmt.Errorf("resolve home: %w", operationError)
	}
	configurationPath := strings.TrimSpace(os.Getenv("ANY_AI_CLI_REMOTE_CONFIG"))
	if configurationPath == "" {
		configurationPath = filepath.Join(home, ".any-aicli-remote", "config.json")
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--config" || argument == "-config" {
			if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
				return "", errors.New("config path cannot be empty")
			}
			configurationPath = arguments[index+1]
			index++
			continue
		}
		if strings.HasPrefix(argument, "--config=") || strings.HasPrefix(argument, "-config=") {
			configurationPath = argument[strings.IndexByte(argument, '=')+1:]
			if strings.TrimSpace(configurationPath) == "" {
				return "", errors.New("config path cannot be empty")
			}
		}
	}
	return canonicalConfigurationPath(configurationPath, home)
}

// ApplyCanonical overlays every non-secret runtime field from a validated canonical document.
func ApplyCanonical(configuration Config, document Document) Config {
	configuration.Bind = document.Network.Bind
	configuration.Port = document.Network.Port
	configuration.AgentHost = document.Agent.Host
	configuration.AgentPort = document.Agent.Port
	configuration.RuntimeDirectory = document.Storage.RuntimeDirectory
	configuration.PublicHost = document.Network.PublicHost
	configuration.ProviderID = document.Provider.ID
	configuration.ProviderPath = document.Provider.ExecutablePath
	configuration.DataDirectory = document.Storage.DataDirectory
	configuration.EnsureAgent = document.Agent.Ensure
	configuration.StopAgentOnExit = document.Agent.StopOnExit
	configuration.ProviderOptions = cloneOptions(document.Provider.Options)
	configuration.Canonical = document
	configuration.Canonical.Provider.Options = cloneOptions(document.Provider.Options)
	return configuration
}

func cloneOptions(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for optionName, optionValue := range source {
		result[optionName] = strings.TrimSpace(optionValue)
	}
	return result
}

func isPlaintextSecretArgument(argument string) bool {
	name := argument
	if separator := strings.IndexByte(name, '='); separator >= 0 {
		name = name[:separator]
	}
	return name == "--secret" || name == "-secret" || name == "--pairing-secret" || name == "-pairing-secret" || name == "--agent-secret" || name == "-agent-secret"
}

func Parse(arguments []string) (Config, error) {
	configuration, operationError := Resolve(arguments)
	if operationError != nil {
		return Config{}, operationError
	}
	home, operationError := os.UserHomeDir()
	if operationError != nil {
		return Config{}, operationError
	}
	if configuration.PairingSecret == "" && configuration.PairingSecretFile == "" {
		configuration.PairingSecretFile = filepath.Join(configuration.DataDirectory, "pairing-secret")
	}
	if operationError := compat.MigrateDataFiles(configuration.DataDirectory, home, configuration.PairingSecret == "" && filepath.Clean(configuration.PairingSecretFile) == filepath.Join(filepath.Clean(configuration.DataDirectory), "pairing-secret")); operationError != nil {
		return Config{}, operationError
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
	if len(configuration.PairingSecret) < minimumSecretCharacters || len(configuration.AgentSecret) < minimumSecretCharacters {
		return Config{}, errors.New("secrets must contain at least 16 characters")
	}
	return configuration, nil
}

func bindFlags(flagSet *flag.FlagSet, document *Document, options *providerfactory.OptionParser, pairingSecretFile *string, agentSecretFile *string, configurationPath *string) {
	flagSet.StringVar(configurationPath, "config", *configurationPath, "configuration file")
	flagSet.StringVar(&document.Network.Bind, "bind", document.Network.Bind, "HTTP bind address")
	flagSet.IntVar(&document.Network.Port, "port", document.Network.Port, "HTTP/WebSocket port")
	flagSet.StringVar(&document.Agent.Host, "agent-host", document.Agent.Host, "provider agent host")
	flagSet.IntVar(&document.Agent.Port, "agent-port", document.Agent.Port, "provider agent port")
	var legacyWorkingDirectory string
	flagSet.StringVar(pairingSecretFile, "pairing-secret-file", *pairingSecretFile, "device pairing secret file")
	flagSet.StringVar(pairingSecretFile, "secret-file", *pairingSecretFile, "deprecated alias")
	flagSet.StringVar(agentSecretFile, "agent-secret-file", *agentSecretFile, "local secret file")
	flagSet.StringVar(&legacyWorkingDirectory, "cwd", "", "deprecated compatibility option")
	flagSet.StringVar(&document.Storage.RuntimeDirectory, "runtime-dir", document.Storage.RuntimeDirectory, "runtime directory")
	flagSet.StringVar(&document.Network.PublicHost, "public-host", document.Network.PublicHost, "public host")
	flagSet.StringVar(&document.Provider.ID, "provider", document.Provider.ID, "provider identifier")
	flagSet.StringVar(&document.Provider.ExecutablePath, "provider-path", document.Provider.ExecutablePath, "provider path")
	flagSet.StringVar(&document.Storage.DataDirectory, "data-dir", document.Storage.DataDirectory, "data directory")
	flagSet.BoolVar(&document.Agent.Ensure, "ensure-agent", document.Agent.Ensure, "ensure provider")
	flagSet.BoolVar(&document.Agent.StopOnExit, "stop-agent-on-exit", document.Agent.StopOnExit, "stop provider")
	options.BindFlags(flagSet, &document.Provider.ExecutablePath)
}

func applyEnvironment(document *Document) error {
	setString := func(key string, target *string) {
		if value := compat.Environment(key, ""); value != "" {
			*target = value
		}
	}
	setInteger := func(key string, target *int) error {
		if value := compat.Environment(key, ""); value != "" {
			parsed, parseError := strconv.Atoi(value)
			if parseError != nil {
				return fmt.Errorf("%s must be an integer", key)
			}
			*target = parsed
		}
		return nil
	}
	setString("ANY_AI_CLI_REMOTE_BIND", &document.Network.Bind)
	setString("ANY_AI_CLI_REMOTE_AGENT_HOST", &document.Agent.Host)
	setString("ANY_AI_CLI_REMOTE_PUBLIC_HOST", &document.Network.PublicHost)
	setString("ANY_AI_CLI_REMOTE_PROVIDER", &document.Provider.ID)
	setString("ANY_AI_CLI_REMOTE_PROVIDER_PATH", &document.Provider.ExecutablePath)
	setString("ANY_AI_CLI_REMOTE_DATA_DIR", &document.Storage.DataDirectory)
	setString("ANY_AI_CLI_REMOTE_RUNTIME_DIR", &document.Storage.RuntimeDirectory)
	if portError := setInteger("ANY_AI_CLI_REMOTE_PORT", &document.Network.Port); portError != nil {
		return portError
	}
	if agentPortError := setInteger("ANY_AI_CLI_REMOTE_AGENT_PORT", &document.Agent.Port); agentPortError != nil {
		return agentPortError
	}
	setBoolean := func(key string, target *bool) error {
		value := compat.Environment(key, "")
		if value == "" {
			return nil
		}
		parsed, parseError := strconv.ParseBool(value)
		if parseError != nil {
			switch strings.ToLower(value) {
			case "yes", "on":
				parsed = true
			case "no", "off":
				parsed = false
			default:
				return fmt.Errorf("%s must be a boolean", key)
			}
		}
		*target = parsed
		return nil
	}
	if parseError := setBoolean("ANY_AI_CLI_REMOTE_ENSURE_AGENT", &document.Agent.Ensure); parseError != nil {
		return parseError
	}
	if parseError := setBoolean("ANY_AI_CLI_REMOTE_STOP_AGENT_ON_EXIT", &document.Agent.StopOnExit); parseError != nil {
		return parseError
	}
	return nil
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
		return standardHTTPSPort
	}
	return standardHTTPPort
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

func canonicalConfigurationPath(path string, home string) (string, error) {
	cleanPath := filepath.Clean(expandHome(strings.TrimSpace(path), home))
	if !filepath.IsAbs(cleanPath) {
		absolutePath, operationError := filepath.Abs(cleanPath)
		if operationError != nil {
			return "", fmt.Errorf("resolve configuration path: %w", operationError)
		}
		cleanPath = absolutePath
	}
	return filepath.Clean(cleanPath), nil
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
		if len(value) < minimumSecretCharacters {
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
	bytes := make([]byte, generatedSecretRandomBytes)
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
