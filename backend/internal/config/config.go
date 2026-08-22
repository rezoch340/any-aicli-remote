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
	"sort"
	"strconv"
	"strings"
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
	Secret            string
	SecretFile        string
	WorkingDirectory  string
	PublicHost        string
	GrokPath          string
	DataDirectory     string
	SessionsDirectory string
	EnsureAgent       bool
	StopAgentOnExit   bool
	AlwaysApprove     bool
	Leader            bool
}

func Parse(arguments []string) (Config, error) {
	home, operationError := os.UserHomeDir()
	if operationError != nil {
		return Config{}, fmt.Errorf("resolve home: %w", operationError)
	}
	workingDirectory, operationError := os.Getwd()
	if operationError != nil {
		return Config{}, fmt.Errorf("resolve cwd: %w", operationError)
	}

	configuration := Config{
		Bind:              environmentOrDefault("GROK_REMOTE_BIND", "0.0.0.0"),
		Port:              environmentInteger("GROK_REMOTE_PORT", DefaultPort),
		AgentHost:         environmentOrDefault("GROK_REMOTE_AGENT_HOST", "127.0.0.1"),
		AgentPort:         environmentInteger("GROK_REMOTE_AGENT_PORT", DefaultAgentPort),
		Secret:            strings.TrimSpace(os.Getenv("GROK_AGENT_SECRET")),
		SecretFile:        strings.TrimSpace(os.Getenv("GROK_REMOTE_SECRET_FILE")),
		WorkingDirectory:  environmentOrDefault("GROK_REMOTE_CWD", workingDirectory),
		PublicHost:        strings.TrimSpace(os.Getenv("GROK_REMOTE_PUBLIC_HOST")),
		GrokPath:          strings.TrimSpace(os.Getenv("GROK_REMOTE_GROK_PATH")),
		DataDirectory:     environmentOrDefault("GROK_PLUGIN_DATA", filepath.Join(home, ".grok", "plugin-data", "grok-remote")),
		SessionsDirectory: environmentOrDefault("GROK_REMOTE_SESSIONS_DIR", filepath.Join(home, ".grok", "sessions")),
		EnsureAgent:       environmentBoolean("GROK_REMOTE_ENSURE_AGENT", true),
		StopAgentOnExit:   environmentBoolean("GROK_REMOTE_STOP_AGENT_ON_EXIT", false),
		AlwaysApprove:     environmentBoolean("GROK_REMOTE_ALWAYS_APPROVE", true),
		Leader:            environmentBoolean("GROK_REMOTE_LEADER", false),
	}

	flagSet := flag.NewFlagSet("grok-remote-daemon", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	flagSet.StringVar(&configuration.Bind, "bind", configuration.Bind, "HTTP bind address")
	flagSet.IntVar(&configuration.Port, "port", configuration.Port, "HTTP/WebSocket port")
	flagSet.StringVar(&configuration.AgentHost, "agent-host", configuration.AgentHost, "Grok agent host")
	flagSet.IntVar(&configuration.AgentPort, "agent-port", configuration.AgentPort, "Grok agent port")
	flagSet.StringVar(&configuration.Secret, "secret", configuration.Secret, "pairing secret (prefer --secret-file or env)")
	flagSet.StringVar(&configuration.SecretFile, "secret-file", configuration.SecretFile, "pairing secret file")
	flagSet.StringVar(&configuration.WorkingDirectory, "cwd", configuration.WorkingDirectory, "workspace root")
	flagSet.StringVar(&configuration.PublicHost, "public-host", configuration.PublicHost, "public pairing host")
	flagSet.StringVar(&configuration.GrokPath, "grok", configuration.GrokPath, "path to Grok CLI")
	flagSet.StringVar(&configuration.DataDirectory, "data-dir", configuration.DataDirectory, "daemon data directory")
	flagSet.StringVar(&configuration.SessionsDirectory, "sessions-dir", configuration.SessionsDirectory, "Grok sessions directory")
	flagSet.BoolVar(&configuration.EnsureAgent, "ensure-agent", configuration.EnsureAgent, "start grok agent serve when needed")
	flagSet.BoolVar(&configuration.StopAgentOnExit, "stop-agent-on-exit", configuration.StopAgentOnExit, "stop the owned Grok agent when the daemon exits")
	flagSet.BoolVar(&configuration.AlwaysApprove, "always-approve", configuration.AlwaysApprove, "launch agent with --always-approve")
	flagSet.BoolVar(&configuration.Leader, "leader", configuration.Leader, "launch agent in leader mode")
	if operationError := flagSet.Parse(arguments); operationError != nil {
		return Config{}, operationError
	}
	if configuration.Port < 1 || configuration.Port > 65535 || configuration.AgentPort < 1 || configuration.AgentPort > 65535 {
		return Config{}, errors.New("ports must be between 1 and 65535")
	}
	if configuration.Port == configuration.AgentPort {
		return Config{}, errors.New("HTTP and agent ports must differ")
	}

	configuration.WorkingDirectory, operationError = filepath.Abs(expandHome(configuration.WorkingDirectory, home))
	if operationError != nil {
		return Config{}, fmt.Errorf("resolve workspace: %w", operationError)
	}
	info, operationError := os.Stat(configuration.WorkingDirectory)
	if operationError != nil || !info.IsDir() {
		return Config{}, fmt.Errorf("workspace is not a directory: %s", configuration.WorkingDirectory)
	}
	configuration.DataDirectory = expandHome(configuration.DataDirectory, home)
	configuration.SessionsDirectory = expandHome(configuration.SessionsDirectory, home)
	if configuration.GrokPath == "" {
		configuration.GrokPath = discoverGrok(home)
	}
	if configuration.EnsureAgent && configuration.GrokPath == "" {
		return Config{}, errors.New("grok CLI not found (expected ~/.grok/bin/grok or PATH)")
	}
	if configuration.SecretFile == "" {
		configuration.SecretFile = discoverSecretFile(home, configuration.DataDirectory)
	} else {
		configuration.SecretFile = expandHome(configuration.SecretFile, home)
	}
	if configuration.Secret == "" {
		configuration.Secret, operationError = loadOrCreateSecret(configuration.SecretFile)
		if operationError != nil {
			return Config{}, operationError
		}
	}
	if len(configuration.Secret) < 16 {
		return Config{}, errors.New("pairing secret must contain at least 16 characters")
	}
	return configuration, nil
}

func (configuration Config) AgentWebSocketURL() string {
	return "ws://" + net.JoinHostPort(configuration.AgentHost, strconv.Itoa(configuration.AgentPort)) + "/ws?server-key=" + configuration.Secret
}

func (configuration Config) LocalURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/?key=%s&auto=1", configuration.Port, configuration.Secret)
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
	query.Set("key", configuration.Secret)
	query.Set("auto", "1")
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

// PairingDeepLink opens the native mobile client directly when it is installed.
func (configuration Config) PairingDeepLink(lanIP, workingDirectory string) string {
	pairingURL := configuration.PairingURL(lanIP)
	if pairingURL == "" {
		return ""
	}
	base, operationError := url.Parse(pairingURL)
	if operationError != nil {
		return ""
	}
	base.RawQuery = ""
	query := url.Values{"url": {base.String()}, "key": {configuration.Secret}}
	if strings.TrimSpace(workingDirectory) != "" {
		query.Set("cwd", workingDirectory)
	}
	return "grokremote://pair?" + query.Encode()
}

func defaultPortForScheme(scheme string) int {
	if strings.EqualFold(scheme, "https") {
		return 443
	}
	return 80
}

func environmentOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func environmentInteger(key string, fallback int) int {
	value, operationError := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if operationError != nil || value == 0 {
		return fallback
	}
	return value
}

func environmentBoolean(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
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

func discoverGrok(home string) string {
	candidates := []string{
		filepath.Join(home, ".grok", "bin", "grok"),
		filepath.Join(home, ".grok", "bin", "grok.exe"),
	}
	if path, operationError := os.Executable(); operationError == nil {
		_ = path
	}
	for _, candidate := range candidates {
		if info, operationError := os.Stat(candidate); operationError == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate
		}
	}
	if path, operationError := execLookPath("grok"); operationError == nil {
		return path
	}
	return ""
}

var execLookPath = func(name string) (string, error) {
	path := os.Getenv("PATH")
	for _, directory := range filepath.SplitList(path) {
		candidate := filepath.Join(directory, name)
		if info, operationError := os.Stat(candidate); operationError == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func discoverSecretFile(home, dataDirectory string) string {
	patterns := []string{
		filepath.Join(home, ".grok", "plugins", "grok-remote", ".ui-secret"),
		filepath.Join(home, ".grok", "installed-plugins", "grok-remote-*", ".ui-secret"),
	}
	var matches []string
	for _, pattern := range patterns {
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	sort.Slice(matches, func(leftIndex, rightIndex int) bool {
		left, _ := os.Stat(matches[leftIndex])
		right, _ := os.Stat(matches[rightIndex])
		if left == nil || right == nil {
			return matches[leftIndex] > matches[rightIndex]
		}
		return left.ModTime().After(right.ModTime())
	})
	for _, path := range matches {
		if value, operationError := os.ReadFile(path); operationError == nil && len(strings.TrimSpace(string(value))) >= 16 {
			return path
		}
	}
	return filepath.Join(dataDirectory, ".ui-secret")
}

func loadOrCreateSecret(path string) (string, error) {
	if data, operationError := os.ReadFile(path); operationError == nil {
		if value := strings.TrimSpace(string(data)); len(value) >= 16 {
			return value, nil
		}
	}
	if operationError := os.MkdirAll(filepath.Dir(path), 0700); operationError != nil {
		return "", fmt.Errorf("create secret directory: %w", operationError)
	}
	bytes := make([]byte, 16)
	if _, operationError := rand.Read(bytes); operationError != nil {
		return "", fmt.Errorf("generate secret: %w", operationError)
	}
	value := hex.EncodeToString(bytes)
	if operationError := os.WriteFile(path, []byte(value+"\n"), 0600); operationError != nil {
		return "", fmt.Errorf("write secret: %w", operationError)
	}
	if operationError := os.Chmod(path, 0600); operationError != nil {
		return "", fmt.Errorf("protect secret: %w", operationError)
	}
	return value, nil
}
