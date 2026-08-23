// Package config owns the daemon's one configuration schema, defaults,
// normalization, and validation path.
//
// This file holds the effective runtime settings and the canonical-document
// overlay. Schema types live in schema.go, defaults in defaults.go, validation
// in validation.go, the document codec in document.go, source resolution in
// resolver.go, and pairing addresses in pairing.go.
package config

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
