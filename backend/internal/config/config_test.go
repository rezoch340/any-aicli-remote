package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestPairingURL(testContext *testing.T) {
	configuration := Config{Port: 2421, Secret: "0123456789abcdef"}
	if got := configuration.PairingURL("192.168.1.4"); got != "http://192.168.1.4:2421/?auto=1&key=0123456789abcdef" {
		testContext.Fatal(got)
	}
	configuration.PublicHost = "https://happy.example:20997"
	if got := configuration.PairingURL("ignored"); !strings.HasPrefix(got, "https://happy.example:20997/") {
		testContext.Fatal(got)
	}
	configuration.PublicHost = "https://happy.example"
	if got := configuration.PairingURL("ignored"); !strings.HasPrefix(got, "https://happy.example/") {
		testContext.Fatal(got)
	}
	configuration.PublicHost = "happy.example"
	if got := configuration.PairingURL("ignored"); !strings.HasPrefix(got, "http://happy.example:2421/") {
		testContext.Fatal(got)
	}
}

func TestPairingDeepLink(testContext *testing.T) {
	configuration := Config{Port: 2421, Secret: "0123456789abcdef"}
	got := configuration.PairingDeepLink("192.168.1.4", "/tmp/work space")
	if !strings.HasPrefix(got, "grokremote://pair?") || !strings.Contains(got, "cwd=%2Ftmp%2Fwork+space") {
		testContext.Fatal(got)
	}
}
