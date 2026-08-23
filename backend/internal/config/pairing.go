// Pairing addresses. A pairing URL or deep link never carries a workspace:
// existing sessions restore their own directory and new sessions supply one.

package config

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

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
