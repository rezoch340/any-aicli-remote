package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grok-remote/grok-remote-app/backend/internal/config"
)

const (
	routeTestSecret = "server-route-test-secret"
	remotePeer      = "192.0.2.10:4567"
	loopbackPeer    = "127.0.0.1:4567"
)

type routeTestServer struct {
	server  *Server
	handler http.Handler
	root    string
}

func newRouteTestServer(testingContext *testing.T) *routeTestServer {
	return newRouteTestServerWithAgentPort(testingContext, 35419)
}

func newRouteTestServerWithAgentPort(testingContext *testing.T, agentPort int) *routeTestServer {
	testingContext.Helper()
	base := testingContext.TempDir()
	home := filepath.Join(base, "home")
	root := filepath.Join(base, "workspace")
	data := filepath.Join(base, "data")
	sessions := filepath.Join(base, "sessions")
	for _, directory := range []string{home, root, data, sessions} {
		if errorValue := os.MkdirAll(directory, 0o755); errorValue != nil {
			testingContext.Fatal(errorValue)
		}
	}
	testingContext.Setenv("HOME", home)
	testingContext.Setenv("XAI_API_KEY", "")
	testingContext.Setenv("GROK_API_KEY", "")
	testingContext.Setenv("xai_api_key", "")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server, errorValue := New(config.Config{
		Bind:              "127.0.0.1",
		Port:              35421,
		AgentHost:         "127.0.0.1",
		AgentPort:         agentPort,
		Secret:            routeTestSecret,
		WorkingDirectory:  root,
		DataDirectory:     data,
		SessionsDirectory: sessions,
		EnsureAgent:       false,
	}, logger)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	// Route tests must not inspect or start any real process.
	server.process.Operations.ListenProcessIDs = func(int, bool) ([]int, error) { return nil, nil }
	server.lanIP = "192.168.1.4"
	testingContext.Cleanup(server.Close)
	return &routeTestServer{server: server, handler: server.Handler(), root: root}
}

func TestServerHealthIsPublic(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	response := fixture.request(testingContext, http.MethodGet, "/health", nil, remotePeer, false)
	assertStatus(testingContext, response, http.StatusOK)
	body := decodeObject(testingContext, response)
	if body["ok"] != true || body["ui"] != true {
		testingContext.Fatalf("health = %#v", body)
	}
	if body["cwd"] != fixture.root || body["agent_listening"] != false {
		testingContext.Fatalf("health compatibility fields = %#v", body)
	}
	if response.Header().Get("Set-Cookie") != "" {
		testingContext.Fatalf("public health set cookie: %s", response.Header().Get("Set-Cookie"))
	}
}

func TestManagedAgentStopPolicy(testingContext *testing.T) {
	testCases := []struct {
		name            string
		stoppedByAPI    bool
		keepAgent       bool
		stopAgentOnExit bool
		expected        bool
	}{
		{name: "API stop", stoppedByAPI: true, expected: true},
		{name: "API keep", stoppedByAPI: true, keepAgent: true, stopAgentOnExit: true, expected: false},
		{name: "launcher exit", stopAgentOnExit: true, expected: true},
		{name: "ordinary exit", expected: false},
	}
	for _, testCase := range testCases {
		testingContext.Run(testCase.name, func(testingContext *testing.T) {
			actual := shouldStopManagedAgent(testCase.stoppedByAPI, testCase.keepAgent, testCase.stopAgentOnExit)
			if actual != testCase.expected {
				testingContext.Fatalf("stop policy = %t, expected %t", actual, testCase.expected)
			}
		})
	}
}

func TestServerRemoteAuthAndConfig(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)

	unauthorized := fixture.request(testingContext, http.MethodGet, "/config", nil, remotePeer, false)
	assertStatus(testingContext, unauthorized, http.StatusUnauthorized)

	response := fixture.request(testingContext, http.MethodGet, "/config", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	body := decodeObject(testingContext, response)
	if body["secret"] != "(held server-side)" || body["cwd"] != fixture.root {
		testingContext.Fatalf("config = %#v", body)
	}
	if body["auth"] != true || body["hub"] != true || body["ws_path"] != "/ws" {
		testingContext.Fatalf("config feature fields = %#v", body)
	}
	if body["agent_port"] != float64(35419) {
		testingContext.Fatalf("agent port = %#v", body["agent_port"])
	}
	if len(response.Result().Cookies()) != 1 {
		testingContext.Fatalf("authorized config cookies = %#v", response.Result().Cookies())
	}
}

func TestServerFSRoutes(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	if errorValue := os.WriteFile(filepath.Join(fixture.root, "seed.txt"), []byte("seed content"), 0o644); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if errorValue := os.Mkdir(filepath.Join(fixture.root, "docs"), 0o755); errorValue != nil {
		testingContext.Fatal(errorValue)
	}

	response := fixture.request(testingContext, http.MethodGet, "/api/fs/root", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	root := decodeObject(testingContext, response)
	if root["root"] != fixture.root || root["exists"] != true {
		testingContext.Fatalf("root = %#v", root)
	}

	response = fixture.request(testingContext, http.MethodGet, "/api/fs/list?path=.", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	listing := decodeObject(testingContext, response)
	if !containsNamedItem(listing["files"], "seed.txt") || !containsNamedItem(listing["dirs"], "docs") {
		testingContext.Fatalf("listing = %#v", listing)
	}

	response = fixture.request(testingContext, http.MethodGet, "/api/fs/read?path=seed.txt", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	read := decodeObject(testingContext, response)
	if read["content"] != "seed content" || read["text"] != true {
		testingContext.Fatalf("read = %#v", read)
	}

	response = fixture.request(testingContext, http.MethodPost, "/api/fs/mkdir", map[string]any{"path": "notes"}, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	if info, errorValue := os.Stat(filepath.Join(fixture.root, "notes")); errorValue != nil || !info.IsDir() {
		testingContext.Fatalf("mkdir result: info=%v err=%v", info, errorValue)
	}

	response = fixture.request(testingContext, http.MethodPost, "/api/fs/write", map[string]any{
		"path": "notes/new.md", "content": "hello\nworld",
	}, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	written := decodeObject(testingContext, response)
	if written["ok"] != true || written["rel"] != "notes/new.md" {
		testingContext.Fatalf("write = %#v", written)
	}
	data, errorValue := os.ReadFile(filepath.Join(fixture.root, "notes", "new.md"))
	if errorValue != nil || string(data) != "hello\nworld" {
		testingContext.Fatalf("written file = %q err=%v", data, errorValue)
	}

	response = fixture.request(testingContext, http.MethodGet, "/api/fs/read?path=notes%2Fnew.md", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	if got := decodeObject(testingContext, response)["content"]; got != "hello\nworld" {
		testingContext.Fatalf("written content = %#v", got)
	}

	alternate := filepath.Join(filepath.Dir(fixture.root), "alternate")
	if errorValue := os.Mkdir(alternate, 0o755); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	response = fixture.request(testingContext, http.MethodPost, "/api/fs/root", map[string]any{"path": alternate}, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	if got := decodeObject(testingContext, response)["root"]; got != alternate {
		testingContext.Fatalf("updated root = %#v", got)
	}
}

func TestServerRoomRoutes(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	response := fixture.request(testingContext, http.MethodPost, "/api/room/say", map[string]any{
		"who": "phone", "text": "hello room", "kind": "say",
	}, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	say := decodeObject(testingContext, response)
	message, valid := say["message"].(map[string]any)
	if say["ok"] != true || !valid || message["text"] != "hello room" || message["who"] != "phone" {
		testingContext.Fatalf("say = %#v", say)
	}

	response = fixture.request(testingContext, http.MethodGet, "/api/room/feed?since=0&limit=20", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	feed := decodeObject(testingContext, response)
	if !containsTextItem(feed["messages"], "hello room") || feed["last"] != float64(1) {
		testingContext.Fatalf("feed = %#v", feed)
	}

	response = fixture.request(testingContext, http.MethodGet, "/api/room/members", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	if !containsNamedMember(decodeObject(testingContext, response)["members"], "phone") {
		testingContext.Fatalf("members = %s", response.Body.String())
	}

	response = fixture.request(testingContext, http.MethodPost, "/api/room/clear", map[string]any{}, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	response = fixture.request(testingContext, http.MethodGet, "/api/room/feed", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	messages, _ := decodeObject(testingContext, response)["messages"].([]any)
	if len(messages) != 0 {
		testingContext.Fatalf("room was not cleared: %#v", messages)
	}
}

func TestServerLoopRoutesWithoutScheduler(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	response := fixture.request(testingContext, http.MethodGet, "/api/loops", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	jobs, _ := decodeObject(testingContext, response)["jobs"].([]any)
	if len(jobs) != 0 {
		testingContext.Fatalf("initial jobs = %#v", jobs)
	}

	response = fixture.request(testingContext, http.MethodPost, "/api/loops", map[string]any{
		"sessionId": "session-1", "prompt": "check status", "interval": "5m",
	}, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	created := decodeObject(testingContext, response)
	job, valid := created["job"].(map[string]any)
	identifier, _ := job["id"].(string)
	if !valid || identifier == "" || job["sessionId"] != "session-1" || job["interval_sec"] != float64(300) {
		testingContext.Fatalf("created loop = %#v", created)
	}

	response = fixture.request(testingContext, http.MethodGet, "/api/loops?sessionId=session-1", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	jobs, _ = decodeObject(testingContext, response)["jobs"].([]any)
	if len(jobs) != 1 {
		testingContext.Fatalf("listed jobs = %#v", jobs)
	}

	response = fixture.request(testingContext, http.MethodPost, "/api/loops/stop", map[string]any{"id": identifier}, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	stopped := decodeObject(testingContext, response)
	if stopped["count"] != float64(1) {
		testingContext.Fatalf("stopped loop = %#v", stopped)
	}
	if remaining := fixture.server.loops.List(""); len(remaining) != 0 {
		testingContext.Fatalf("remaining loops = %#v", remaining)
	}
}

func TestServerVoiceStatus(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	response := fixture.request(testingContext, http.MethodGet, "/api/voice/status", nil, remotePeer, true)
	assertStatus(testingContext, response, http.StatusOK)
	status := decodeObject(testingContext, response)
	voices, _ := status["voices"].([]any)
	if status["ok"] != true || status["tts"] != false || status["stt"] != "browser" || status["provider"] != "browser-fallback" || len(voices) == 0 {
		testingContext.Fatalf("voice status = %#v", status)
	}
}

func TestServerPairLoopbackRestrictionAndUnknownRoute(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)

	remote := fixture.requestWithHeaders(testingContext, http.MethodGet, "/pair", nil, remotePeer, http.Header{
		"X-Grok-Remote-Key": {routeTestSecret},
		"X-Forwarded-For":   {"127.0.0.1"},
	})
	assertStatus(testingContext, remote, http.StatusForbidden)
	if strings.TrimSpace(remote.Body.String()) != "pair is loopback-only" {
		testingContext.Fatalf("remote pair body = %q", remote.Body.String())
	}

	local := fixture.request(testingContext, http.MethodGet, "/pair", nil, loopbackPeer, false)
	assertStatus(testingContext, local, http.StatusOK)
	if !strings.Contains(local.Body.String(), "Pair Grok Remote") || !strings.Contains(local.Body.String(), "grokremote://pair?") {
		testingContext.Fatalf("local pair page = %s", local.Body.String())
	}

	unknown := fixture.request(testingContext, http.MethodGet, "/not-a-route", nil, remotePeer, true)
	assertStatus(testingContext, unknown, http.StatusNotFound)
}

func (fixture *routeTestServer) request(testingContext *testing.T, method, target string, payload any, remote string, authenticated bool) *httptest.ResponseRecorder {
	testingContext.Helper()
	headers := make(http.Header)
	if authenticated {
		headers.Set("X-Grok-Remote-Key", routeTestSecret)
	}
	return fixture.requestWithHeaders(testingContext, method, target, payload, remote, headers)
}

func (fixture *routeTestServer) requestWithHeaders(testingContext *testing.T, method, target string, payload any, remote string, headers http.Header) *httptest.ResponseRecorder {
	testingContext.Helper()
	var body io.Reader
	if payload != nil {
		encoded, errorValue := json.Marshal(payload)
		if errorValue != nil {
			testingContext.Fatal(errorValue)
		}
		body = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, target, body)
	request.RemoteAddr = remote
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

func assertStatus(testingContext *testing.T, response *httptest.ResponseRecorder, expected int) {
	testingContext.Helper()
	if response.Code != expected {
		testingContext.Fatalf("status = %d, want %d; body=%s", response.Code, expected, response.Body.String())
	}
}

func decodeObject(testingContext *testing.T, response *httptest.ResponseRecorder) map[string]any {
	testingContext.Helper()
	var result map[string]any
	if errorValue := json.Unmarshal(response.Body.Bytes(), &result); errorValue != nil {
		testingContext.Fatalf("decode JSON: %v; body=%s", errorValue, response.Body.String())
	}
	return result
}

func containsNamedItem(raw any, name string) bool {
	items, _ := raw.([]any)
	for _, rawItem := range items {
		if item, valid := rawItem.(map[string]any); valid && item["name"] == name {
			return true
		}
	}
	return false
}

func containsTextItem(raw any, text string) bool {
	items, _ := raw.([]any)
	for _, rawItem := range items {
		if item, valid := rawItem.(map[string]any); valid && item["text"] == text {
			return true
		}
	}
	return false
}

func containsNamedMember(raw any, who string) bool {
	members, _ := raw.([]any)
	for _, rawMember := range members {
		if member, valid := rawMember.(map[string]any); valid && member["who"] == who {
			return true
		}
	}
	return false
}
