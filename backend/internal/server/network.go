// Host network probes: existing-daemon health and LAN address discovery.

package server

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func healthyRemote(port int, timeout time.Duration, maxBytes int64) bool {
	client := &http.Client{Timeout: timeout}
	response, errorValue := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if errorValue != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	data, _ := io.ReadAll(io.LimitReader(response.Body, maxBytes))
	return bytes.Contains(data, []byte(`"ok"`))
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ipAddress := net.ParseIP(host)
	return ipAddress != nil && ipAddress.IsLoopback()
}

func discoverLANIP() string {
	interfaces, _ := net.Interfaces()
	first := ""
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := networkInterface.Addrs()
		for _, address := range addresses {
			ipAddress, _, errorValue := net.ParseCIDR(address.String())
			if errorValue != nil || ipAddress == nil || ipAddress.IsLoopback() || ipAddress.To4() == nil {
				continue
			}
			if first == "" {
				first = ipAddress.String()
			}
			if ipAddress.IsPrivate() {
				return ipAddress.String()
			}
		}
	}
	if first != "" {
		return first
	}
	return "127.0.0.1"
}
