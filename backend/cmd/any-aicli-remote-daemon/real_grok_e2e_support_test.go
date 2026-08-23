package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
)

const realGrokEnabledEnvironment = "ANY_AI_CLI_REMOTE_REAL_GROK_E2E"
const realGrokExecutableEnvironment = "ANY_AI_CLI_REMOTE_REAL_GROK_EXECUTABLE"
const realGrokSessionsEnvironment = "ANY_AI_CLI_REMOTE_REAL_GROK_SESSIONS_DIR"
const realGrokTimeout = 45 * time.Second
const realGrokHTTPTimeout = 10 * time.Second
const realGrokPollInterval = 100 * time.Millisecond
const realGrokHTTPResponseLimit = 16 * 1024 * 1024

type realGrokFixture struct {
	paths                          smokePaths
	daemonPort, agentPort          int
	workspaceA, workspaceB, secret string
	executable                     string
	client                         *http.Client
}

func realGrokExecutable(testingContext *testing.T) string {
	testingContext.Helper()
	path := os.Getenv(realGrokExecutableEnvironment)
	if path == "" {
		path = filepath.Join(mustHome(testingContext), ".grok", "bin", "grok")
	}
	info, errorValue := os.Stat(path)
	if errorValue != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		testingContext.Fatalf("真实 Grok E2E 已启用但 CLI 不可执行: %s", path)
	}
	return path
}
func mustHome(testingContext *testing.T) string {
	testingContext.Helper()
	home, errorValue := os.UserHomeDir()
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	return home
}
func realGrokSessionsDirectory(testingContext *testing.T) string {
	testingContext.Helper()
	path := os.Getenv(realGrokSessionsEnvironment)
	if path == "" {
		path = filepath.Join(mustHome(testingContext), ".grok", "sessions")
	}
	info, errorValue := os.Stat(path)
	if errorValue != nil || !info.IsDir() {
		testingContext.Fatalf("真实 Grok 会话目录不可读（可能尚未登录）: %s", path)
	}
	return path
}
func resetRealGrokEnvironment(testingContext *testing.T) {
	testingContext.Helper()
	for _, name := range []string{"ANY_AI_CLI_REMOTE_CONFIG", "ANY_AI_CLI_REMOTE_BIND", "ANY_AI_CLI_REMOTE_PORT", "ANY_AI_CLI_REMOTE_AGENT_HOST", "ANY_AI_CLI_REMOTE_AGENT_PORT", "ANY_AI_CLI_REMOTE_PAIRING_SECRET", "ANY_AI_CLI_REMOTE_PAIRING_SECRET_FILE", "ANY_AI_CLI_REMOTE_AGENT_SECRET", "ANY_AI_CLI_REMOTE_AGENT_SECRET_FILE", "ANY_AI_CLI_REMOTE_RUNTIME_DIR", "ANY_AI_CLI_REMOTE_PUBLIC_HOST", "ANY_AI_CLI_REMOTE_PROVIDER", "ANY_AI_CLI_REMOTE_PROVIDER_PATH", "ANY_AI_CLI_REMOTE_DATA_DIR", "ANY_AI_CLI_REMOTE_ENSURE_AGENT", "ANY_AI_CLI_REMOTE_STOP_AGENT_ON_EXIT", "ANY_AI_CLI_REMOTE_PROVIDER_SESSIONS_DIR", "ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE", "ANY_AI_CLI_REMOTE_PROVIDER_LEADER", "ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR", "ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE", "ANY_AI_CLI_REMOTE_GROK_LEADER", "ANY_AI_CLI_REMOTE_CWD", "GROK_PLUGIN_DATA", "GROK_REMOTE_CONFIG", "GROK_REMOTE_BIND", "GROK_REMOTE_PORT", "GROK_REMOTE_AGENT_HOST", "GROK_REMOTE_AGENT_PORT", "GROK_REMOTE_SECRET_FILE", "GROK_REMOTE_RUNTIME_DIR", "GROK_REMOTE_PUBLIC_HOST", "GROK_REMOTE_PROVIDER", "GROK_REMOTE_GROK_PATH", "GROK_REMOTE_SESSIONS_DIR", "GROK_REMOTE_ENSURE_AGENT", "GROK_REMOTE_STOP_AGENT_ON_EXIT", "GROK_REMOTE_ALWAYS_APPROVE", "GROK_REMOTE_LEADER", "GROK_REMOTE_CWD"} {
		testingContext.Setenv(name, "")
	}
}
func newRealGrokFixture(testingContext *testing.T, executable, sessions string) realGrokFixture {
	testingContext.Helper()
	resetRealGrokEnvironment(testingContext)
	paths := newSmokePaths(testingContext)
	ports := distinctSmokePorts(testingContext)
	document := config.DefaultDocument(mustHome(testingContext))
	document.Network.Bind = "127.0.0.1"
	document.Network.Port = ports[0]
	document.Agent.Host = "127.0.0.1"
	document.Agent.Port = ports[1]
	document.Agent.Ensure = true
	document.Agent.StopOnExit = true
	document.Storage.DataDirectory = paths.dataDirectory
	document.Storage.RuntimeDirectory = paths.runtimeDirectory
	document.Provider.ID = "grok"
	document.Provider.ExecutablePath = executable
	document.Provider.Options = map[string]string{"sessions-directory": sessions, "always-approve": "false", "leader": "false"}
	applySmokeDocument(testingContext, paths.configurationPath, document)
	materializeSmokeSecret(testingContext, paths.secretPath)
	return realGrokFixture{paths: paths, daemonPort: ports[0], agentPort: ports[1], workspaceA: testingContext.TempDir(), workspaceB: testingContext.TempDir(), secret: smokeSecretValue, executable: executable, client: &http.Client{Timeout: realGrokHTTPTimeout}}
}
func (fixture realGrokFixture) endpoint(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", fixture.daemonPort, path)
}
func (fixture realGrokFixture) request(testingContext *testing.T, method, path string, body any, result any) int {
	testingContext.Helper()
	var reader io.Reader
	if body != nil {
		encoded, errorValue := json.Marshal(body)
		if errorValue != nil {
			testingContext.Fatal(errorValue)
		}
		reader = bytes.NewReader(encoded)
	}
	request, errorValue := http.NewRequest(method, fixture.endpoint(path), reader)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	request.Header.Set("X-Any-AI-CLI-Remote-Key", fixture.secret)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, errorValue := fixture.client.Do(request)
	if errorValue != nil {
		testingContext.Fatalf("HTTP %s %s failed: %v", method, path, errorValue)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, realGrokHTTPResponseLimit+1)
	data, readError := io.ReadAll(limited)
	if readError != nil {
		testingContext.Fatalf("HTTP %s %s read failed", method, path)
	}
	if len(data) > realGrokHTTPResponseLimit {
		testingContext.Fatalf("HTTP %s %s response exceeds %d bytes", method, path, realGrokHTTPResponseLimit)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.StatusCode
	}
	if result != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
		if errorValue = json.Unmarshal(data, result); errorValue != nil {
			testingContext.Fatalf("HTTP %s %s decode failed: %v", method, path, errorValue)
		}
	}
	return response.StatusCode
}
func (fixture realGrokFixture) awaitHub(testingContext *testing.T) {
	testingContext.Helper()
	deadline := time.Now().Add(realGrokTimeout)
	for time.Now().Before(deadline) {
		var status struct {
			AgentListening bool `json:"agent_listening"`
			HubUp          bool `json:"hub_up"`
		}
		if fixture.request(testingContext, http.MethodGet, "/api/stack/status", nil, &status) == http.StatusOK && status.AgentListening && status.HubUp {
			return
		}
		time.Sleep(realGrokPollInterval)
	}
	testingContext.Fatal("agent_listening/hub_up 未在时限内就绪")
}
func (fixture realGrokFixture) websocketURL() string {
	values := url.Values{}
	values.Set("key", fixture.secret)
	return fmt.Sprintf("ws://127.0.0.1:%d/ws?%s", fixture.daemonPort, values.Encode())
}
func (fixture realGrokFixture) deleteSession(testingContext *testing.T, sessionID string) {
	testingContext.Helper()
	if sessionID == "" {
		return
	}
	command := exec.Command(fixture.executable, "sessions", "delete", sessionID)
	if errorValue := command.Run(); errorValue != nil {
		testingContext.Errorf("failed to delete exact test session ID: %v", errorValue)
	}
}
func realGrokSessionIDs(testingContext *testing.T, fixture realGrokFixture) map[string]bool {
	testingContext.Helper()
	var data struct {
		Sessions []struct {
			SessionID string `json:"sessionId"`
		} `json:"sessions"`
	}
	if status := fixture.request(testingContext, http.MethodGet, "/api/sessions?providerId=grok", nil, &data); status != http.StatusOK {
		testingContext.Fatalf("GET /api/sessions status=%d", status)
	}
	ids := map[string]bool{}
	for _, item := range data.Sessions {
		ids[item.SessionID] = true
	}
	return ids
}
func realGrokSameIDs(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for sessionID := range left {
		if !right[sessionID] {
			return false
		}
	}
	return true
}
func (fixture realGrokFixture) daemonReachable() bool {
	request, errorValue := http.NewRequest(http.MethodGet, fixture.endpoint("/health"), nil)
	if errorValue != nil {
		return false
	}
	response, errorValue := fixture.client.Do(request)
	if errorValue != nil {
		return false
	}
	_ = response.Body.Close()
	return true
}

func cleanupRealGrok(testingContext *testing.T, fixture realGrokFixture, client **realGrokRPCClient, cancel context.CancelFunc, completion <-chan error, sessionIDs ...string) {
	testingContext.Helper()
	if *client != nil {
		(*client).close()
		*client = nil
	}
	if fixture.daemonReachable() {
		fixture.request(testingContext, http.MethodPost, "/api/stack/stop", map[string]bool{"keep_agent": false}, nil)
	}
	cancel()
	select {
	case errorValue := <-completion:
		if errorValue != nil {
			testingContext.Errorf("daemon cleanup returned: %v", errorValue)
		}
	case <-time.After(realGrokTimeout):
		testingContext.Error("daemon cleanup timed out")
	}
	assertHealthUnavailable(testingContext, fixture.client, fixture.daemonPort)
	assertPortAvailable(testingContext, fixture.daemonPort)
	assertPortAvailable(testingContext, fixture.agentPort)
	for _, sessionID := range sessionIDs {
		fixture.deleteSession(testingContext, sessionID)
	}
}
func realGrokRunDaemon(fixture realGrokFixture) (context.CancelFunc, <-chan error) {
	executionContext, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runDaemonWithContext(executionContext, daemonLaunchArguments(fixture.paths.configurationPath, fixture.paths.secretPath), io.Discard)
	}()
	return cancel, done
}
