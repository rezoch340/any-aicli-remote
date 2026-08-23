package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

const realGrokPromptTimeout = 180 * time.Second

var realGrokSessionMeta = map[string]any{"yoloMode": false, "autoMode": false}

type realGrokObservation struct {
	methods             map[string]bool
	updateKinds         map[string]bool
	agentMessageChunk   bool
	permissionLifecycle bool
}

type realGrokCallResult struct {
	envelope   realGrokRPCEnvelope
	errorValue error
}

type realGrokStackStatus struct {
	AgentPIDs      []int `json:"agent_pids"`
	AgentListening bool  `json:"agent_listening"`
	HubUp          bool  `json:"hub_up"`
}

type realGrokStackStartResponse struct {
	AgentPIDs []int `json:"agent_pids"`
	Started   bool  `json:"started"`
	HubUp     bool  `json:"hub_up"`
}

func TestRealGrokLifecycle(testingContext *testing.T) {
	if os.Getenv(realGrokEnabledEnvironment) != "1" {
		testingContext.Skip("设置 ANY_AI_CLI_REMOTE_REAL_GROK_E2E=1 后运行真实 Grok E2E")
	}
	executable := realGrokExecutable(testingContext)
	fixture := newRealGrokFixture(testingContext, executable, realGrokSessionsDirectory(testingContext))
	cancelDaemon, daemonDone := realGrokRunDaemon(fixture)
	var client *realGrokRPCClient
	var sessionA, sessionB string
	var cleanupOnce sync.Once
	defer func() {
		cleanupOnce.Do(func() {
			cleanupRealGrok(testingContext, fixture, &client, cancelDaemon, daemonDone, sessionA, sessionB)
		})
	}()

	awaitRealGrokAgentHealth(testingContext, fixture)
	fixture.awaitHub(testingContext)
	beforeIDs := realGrokSessionIDs(testingContext, fixture)
	time.Sleep(500 * time.Millisecond)
	if stableIDs := realGrokSessionIDs(testingContext, fixture); !realGrokSameIDs(beforeIDs, stableIDs) {
		testingContext.Fatal("daemon 启动/idle 意外创建、加载或恢复了会话")
	}

	client = dialInitializedRealGrok(testingContext, fixture)
	sessionA = newRealGrokSession(testingContext, client, fixture.workspaceA)
	sessionB = newRealGrokSession(testingContext, client, fixture.workspaceB)
	assertRealGrokSessionWorkspaces(testingContext, fixture, map[string]string{sessionA: fixture.workspaceA, sessionB: fixture.workspaceB})
	assertRealGrokFilesystemIsolation(testingContext, fixture, sessionA, sessionB)
	writeRealGrokFile(testingContext, fixture, sessionA, "input.txt", "REAL_GROK_INPUT")

	promptText := "Use the file tool to read input.txt. Use the terminal tool to execute /usr/bin/printf with argument REAL_GROK_TERMINAL and observe its output. Then use the file write tool to write exactly REAL_GROK_OUTPUT to grok-output.txt and exactly REAL_GROK_TERMINAL to terminal-output.txt. Reply with REAL_GROK_REPLY."
	promptResponse, observation := promptRealGrok(testingContext, client, sessionA, promptText, nil)
	if promptResponse.StopReason != acp.StopReasonEndTurn {
		testingContext.Fatalf("prompt stop reason=%s", promptResponse.StopReason)
	}
	assertRealGrokPromptEvidence(testingContext, client, observation)
	assertRealGrokFile(testingContext, fixture, sessionA, "grok-output.txt", "REAL_GROK_OUTPUT")
	assertRealGrokFile(testingContext, fixture, sessionA, "terminal-output.txt", "REAL_GROK_TERMINAL")
	cancelRealGrokPrompt(testingContext, client, sessionA)

	var previousStatus realGrokStackStatus
	if status := fixture.request(testingContext, http.MethodGet, "/api/stack/status", nil, &previousStatus); status != http.StatusOK || !previousStatus.AgentListening || len(previousStatus.AgentPIDs) == 0 {
		testingContext.Fatalf("agent status before force restart invalid: status=%d pids=%v listening=%t", status, previousStatus.AgentPIDs, previousStatus.AgentListening)
	}
	var restartResponse realGrokStackStartResponse
	if status := fixture.request(testingContext, http.MethodPost, "/api/stack/start", map[string]bool{"force": true}, &restartResponse); status != http.StatusOK || !restartResponse.Started || !restartResponse.HubUp || !realGrokDifferentPIDs(previousStatus.AgentPIDs, restartResponse.AgentPIDs) {
		testingContext.Fatalf("force restart did not replace agent: status=%d started=%t hub_up=%t old_pids=%v new_pids=%v", status, restartResponse.Started, restartResponse.HubUp, previousStatus.AgentPIDs, restartResponse.AgentPIDs)
	}
	select {
	case <-client.done:
		testingContext.Fatal("force restart 意外关闭了远程 ACP WebSocket")
	default:
	}
	initializeRealGrok(testingContext, client)
	loadContext, loadCancel := context.WithTimeout(context.Background(), realGrokTimeout)
	defer loadCancel()
	loadRequest := acp.LoadSessionRequest{SessionId: acp.SessionId(sessionA), Cwd: fixture.workspaceB, McpServers: []acp.McpServer{}}
	if _, errorValue := client.call(loadContext, acp.AgentMethodSessionLoad, loadRequest); errorValue != nil {
		testingContext.Fatalf("session/load failed: %v", errorValue)
	}
	assertRealGrokSessionWorkspaces(testingContext, fixture, map[string]string{sessionA: fixture.workspaceA})
	assertRealGrokRoot(testingContext, fixture, sessionA, fixture.workspaceA)
	writeRealGrokFile(testingContext, fixture, sessionA, "resume-check.txt", "RESUMED_A")
	assertRealGrokFile(testingContext, fixture, sessionA, "resume-check.txt", "RESUMED_A")
	resumeResponse, resumeObservation := promptRealGrok(testingContext, client, sessionA, "Reply exactly REAL_GROK_RESUMED.", nil)
	if resumeResponse.StopReason != acp.StopReasonEndTurn || !resumeObservation.agentMessageChunk {
		testingContext.Fatalf("resume prompt missing end_turn/stream; kinds=%v", sortedRealGrokKeys(resumeObservation.updateKinds))
	}
	awaitRealGrokNonEmpty(testingContext, fixture, "/api/session/history?providerId=grok&sessionId="+url.QueryEscape(sessionA)+"&chat_only=1")
	awaitRealGrokNonEmpty(testingContext, fixture, "/api/sessions/"+url.PathEscape(sessionA)+"/messages?providerId=grok")
	assertRealGrokArchive(testingContext, fixture, sessionA, true)
	assertRealGrokArchive(testingContext, fixture, sessionA, false)
}

func realGrokDifferentPIDs(previous, current []int) bool {
	if len(previous) == 0 || len(current) == 0 || len(previous) != len(current) {
		return false
	}
	previousSet := make(map[int]bool, len(previous))
	for _, processID := range previous {
		previousSet[processID] = true
	}
	for _, processID := range current {
		if previousSet[processID] {
			return false
		}
	}
	return true
}

func awaitRealGrokAgentHealth(testingContext *testing.T, fixture realGrokFixture) {
	testingContext.Helper()
	deadline := time.Now().Add(realGrokTimeout)
	for time.Now().Before(deadline) {
		request, _ := http.NewRequest(http.MethodGet, fixture.endpoint("/health"), nil)
		response, errorValue := fixture.client.Do(request)
		if errorValue == nil {
			var health healthResponse
			decodeError := json.NewDecoder(response.Body).Decode(&health)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeError == nil && health.OK && health.AgentListening {
				return
			}
		}
		time.Sleep(realGrokPollInterval)
	}
	testingContext.Fatal("health agent_listening 未在时限内就绪")
}

func dialInitializedRealGrok(testingContext *testing.T, fixture realGrokFixture) *realGrokRPCClient {
	testingContext.Helper()
	client, _, errorValue := dialRealGrokWebSocket(fixture.websocketURL(), fixture.secret)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	initializeRealGrok(testingContext, client)
	return client
}

func initializeRealGrok(testingContext *testing.T, client *realGrokRPCClient) {
	testingContext.Helper()
	requestContext, cancel := context.WithTimeout(context.Background(), realGrokTimeout)
	defer cancel()
	request := acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber, ClientInfo: &acp.Implementation{Name: "real-grok-e2e", Version: "1"}, ClientCapabilities: acp.ClientCapabilities{Fs: acp.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}, Terminal: true}}
	envelope, errorValue := client.call(requestContext, acp.AgentMethodInitialize, request)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	var response acp.InitializeResponse
	if errorValue = decodeRealGrokRPCResult(envelope, &response); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if response.ProtocolVersion == 0 {
		testingContext.Fatal("initialize response missing protocol metadata")
	}
}

func newRealGrokSession(testingContext *testing.T, client *realGrokRPCClient, directory string) string {
	testingContext.Helper()
	requestContext, cancel := context.WithTimeout(context.Background(), realGrokTimeout)
	defer cancel()
	request := acp.NewSessionRequest{Meta: realGrokSessionMeta, Cwd: directory, McpServers: []acp.McpServer{}}
	envelope, errorValue := client.call(requestContext, acp.AgentMethodSessionNew, request)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	var response acp.NewSessionResponse
	if errorValue = decodeRealGrokRPCResult(envelope, &response); errorValue != nil || response.SessionId == "" {
		testingContext.Fatal("session/new did not return sessionId")
	}
	return string(response.SessionId)
}

func promptRealGrok(testingContext *testing.T, client *realGrokRPCClient, sessionID, promptText string, ready func(realGrokRPCEnvelope) bool) (acp.PromptResponse, realGrokObservation) {
	testingContext.Helper()
	drainRealGrokNotifications(client)
	requestContext, cancel := context.WithTimeout(context.Background(), realGrokPromptTimeout)
	defer cancel()
	resultChannel := make(chan realGrokCallResult, 1)
	go func() {
		request := acp.PromptRequest{SessionId: acp.SessionId(sessionID), Prompt: []acp.ContentBlock{acp.TextBlock(promptText)}}
		envelope, errorValue := client.call(requestContext, acp.AgentMethodSessionPrompt, request)
		resultChannel <- realGrokCallResult{envelope: envelope, errorValue: errorValue}
	}()
	observation := realGrokObservation{methods: map[string]bool{}, updateKinds: map[string]bool{}}
	for {
		select {
		case result := <-resultChannel:
			if result.errorValue != nil {
				testingContext.Fatal(result.errorValue)
			}
			var response acp.PromptResponse
			if errorValue := decodeRealGrokRPCResult(result.envelope, &response); errorValue != nil {
				testingContext.Fatal(errorValue)
			}
			return response, observation
		case notification := <-client.notifications:
			observeRealGrokNotification(notification, sessionID, &observation)
			if ready != nil && ready(notification) {
				if errorValue := client.notify(acp.AgentMethodSessionCancel, acp.CancelNotification{SessionId: acp.SessionId(sessionID)}); errorValue != nil {
					testingContext.Fatal(errorValue)
				}
				ready = nil
			}
		case <-requestContext.Done():
			testingContext.Fatal(requestContext.Err())
		case <-client.done:
			testingContext.Fatal(client.terminalError())
		}
	}
}

func drainRealGrokNotifications(client *realGrokRPCClient) {
	for {
		select {
		case <-client.notifications:
			continue
		default:
			return
		}
	}
}

func realGrokNotificationDetails(notification realGrokRPCEnvelope) (string, string, string) {
	var parameters map[string]json.RawMessage
	if json.Unmarshal(notification.Params, &parameters) != nil {
		return "", "", ""
	}
	var sessionID string
	_ = json.Unmarshal(parameters["sessionId"], &sessionID)
	if notification.Method == "_x.ai/remote/client_rpc" {
		var method string
		_ = json.Unmarshal(parameters["method"], &method)
		return sessionID, method, ""
	}
	if notification.Method != "session/update" {
		return sessionID, "", ""
	}
	var update struct {
		Kind string `json:"sessionUpdate"`
	}
	_ = json.Unmarshal(parameters["update"], &update)
	return sessionID, "", update.Kind
}

func observeRealGrokNotification(notification realGrokRPCEnvelope, expectedSessionID string, observation *realGrokObservation) {
	sessionID, method, kind := realGrokNotificationDetails(notification)
	if sessionID != expectedSessionID {
		return
	}
	if notification.Method == "_x.ai/remote/client_rpc" {
		observation.methods[method] = true
	}
	if notification.Method != "session/update" {
		return
	}
	observation.updateKinds[kind] = true
	if kind == "agent_message_chunk" {
		observation.agentMessageChunk = true
	}
	lowerKind := strings.ToLower(kind)
	if strings.Contains(lowerKind, "permission") && (strings.Contains(lowerKind, "pending") || strings.Contains(lowerKind, "resolved")) {
		observation.permissionLifecycle = true
	}
}

func assertRealGrokPromptEvidence(testingContext *testing.T, client *realGrokRPCClient, observation realGrokObservation) {
	testingContext.Helper()
	for _, category := range []string{"fs/read_text_file", "fs/write_text_file", "terminal/create"} {
		found := false
		for method := range observation.methods {
			canonical := strings.ReplaceAll(strings.ToLower(method), "readtextfile", "read_text_file")
			canonical = strings.ReplaceAll(canonical, "writetextfile", "write_text_file")
			if canonical == category {
				found = true
			}
		}
		if !found {
			testingContext.Fatalf("未观察到 %s；methods=%v kinds=%v", category, sortedRealGrokKeys(observation.methods), sortedRealGrokKeys(observation.updateKinds))
		}
	}
	if !observation.agentMessageChunk {
		testingContext.Fatalf("响应前未观察到 agent_message_chunk；kinds=%v", sortedRealGrokKeys(observation.updateKinds))
	}
	if !client.permissionWasObserved() && !observation.permissionLifecycle {
		testingContext.Fatal("未观察到 permission 生命周期")
	}
}

func cancelRealGrokPrompt(testingContext *testing.T, client *realGrokRPCClient, sessionID string) {
	testingContext.Helper()
	ready := func(notification realGrokRPCEnvelope) bool {
		notificationSessionID, method, kind := realGrokNotificationDetails(notification)
		return notificationSessionID == sessionID && (method == "terminal/create" || kind == "tool_call")
	}
	response, _ := promptRealGrok(testingContext, client, sessionID, "Use the terminal tool to execute /bin/sleep with argument 30, and wait for it to finish.", ready)
	if response.StopReason != acp.StopReasonCancelled {
		testingContext.Fatalf("cancel stop reason=%s", response.StopReason)
	}
}

func writeRealGrokFile(testingContext *testing.T, fixture realGrokFixture, sessionID, path, content string) {
	testingContext.Helper()
	status := fixture.request(testingContext, http.MethodPost, "/api/fs/write", map[string]any{"providerId": "grok", "sessionId": sessionID, "path": path, "content": content}, nil)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		testingContext.Fatalf("fs write status=%d", status)
	}
}

func assertRealGrokFile(testingContext *testing.T, fixture realGrokFixture, sessionID, path, expected string) {
	testingContext.Helper()
	var result struct {
		Content string `json:"content"`
	}
	status := fixture.request(testingContext, http.MethodGet, "/api/fs/read?providerId=grok&sessionId="+url.QueryEscape(sessionID)+"&path="+url.QueryEscape(path), nil, &result)
	if status != http.StatusOK || strings.TrimSpace(result.Content) != expected {
		testingContext.Fatalf("unexpected file result for %s (status=%d)", path, status)
	}
}

func assertRealGrokFilesystemIsolation(testingContext *testing.T, fixture realGrokFixture, sessionA, sessionB string) {
	writeRealGrokFile(testingContext, fixture, sessionA, "isolation.txt", "A")
	writeRealGrokFile(testingContext, fixture, sessionB, "isolation.txt", "B")
	assertRealGrokFile(testingContext, fixture, sessionA, "isolation.txt", "A")
	assertRealGrokFile(testingContext, fixture, sessionB, "isolation.txt", "B")
	traversal := "../" + filepath.Base(fixture.workspaceB) + "/isolation.txt"
	status := fixture.request(testingContext, http.MethodGet, "/api/fs/read?providerId=grok&sessionId="+url.QueryEscape(sessionA)+"&path="+url.QueryEscape(traversal), nil, nil)
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		testingContext.Fatal("A can traverse into B")
	}
}

func assertRealGrokSessionWorkspaces(testingContext *testing.T, fixture realGrokFixture, expected map[string]string) {
	testingContext.Helper()
	canonical := make(map[string]string, len(expected))
	for sessionID, directory := range expected {
		resolved, errorValue := filepath.EvalSymlinks(directory)
		if errorValue != nil {
			testingContext.Fatal(errorValue)
		}
		canonical[sessionID] = resolved
	}
	deadline := time.Now().Add(realGrokTimeout)
	for time.Now().Before(deadline) {
		var response struct {
			Sessions []struct {
				SessionID        string `json:"sessionId"`
				ProjectDirectory string `json:"projectDir"`
			} `json:"sessions"`
		}
		if fixture.request(testingContext, http.MethodGet, "/api/sessions?providerId=grok", nil, &response) == http.StatusOK {
			found := map[string]string{}
			for _, session := range response.Sessions {
				if _, wanted := canonical[session.SessionID]; wanted {
					found[session.SessionID] = session.ProjectDirectory
				}
			}
			valid := len(found) == len(canonical)
			for sessionID, directory := range canonical {
				valid = valid && found[sessionID] == directory
			}
			if valid {
				return
			}
		}
		time.Sleep(realGrokPollInterval)
	}
	testingContext.Fatal("/api/sessions 未返回精确 canonical projectDir")
}

func assertRealGrokRoot(testingContext *testing.T, fixture realGrokFixture, sessionID, expected string) {
	testingContext.Helper()
	var response struct {
		Root string `json:"root"`
	}
	status := fixture.request(testingContext, http.MethodGet, "/api/fs/root?providerId=grok&sessionId="+url.QueryEscape(sessionID), nil, &response)
	canonical, errorValue := filepath.EvalSymlinks(expected)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if status != http.StatusOK || response.Root != canonical {
		testingContext.Fatalf("session root mismatch (status=%d)", status)
	}
}

func awaitRealGrokNonEmpty(testingContext *testing.T, fixture realGrokFixture, path string) {
	testingContext.Helper()
	deadline := time.Now().Add(realGrokTimeout)
	for time.Now().Before(deadline) {
		var response struct {
			Count int `json:"count"`
		}
		if fixture.request(testingContext, http.MethodGet, path, nil, &response) == http.StatusOK && response.Count > 0 {
			return
		}
		time.Sleep(realGrokPollInterval)
	}
	testingContext.Fatalf("%s count remained empty", path)
}

func assertRealGrokArchive(testingContext *testing.T, fixture realGrokFixture, sessionID string, archived bool) {
	testingContext.Helper()
	if status := fixture.request(testingContext, http.MethodPost, "/api/session/archived", map[string]any{"sessionId": sessionID, "archived": archived}, nil); status != http.StatusOK {
		testingContext.Fatalf("archive POST status=%d", status)
	}
	var response struct {
		IDs []string `json:"ids"`
	}
	if status := fixture.request(testingContext, http.MethodGet, "/api/session/archived", nil, &response); status != http.StatusOK {
		testingContext.Fatalf("archive GET status=%d", status)
	}
	found := false
	for _, identifier := range response.IDs {
		found = found || identifier == sessionID
	}
	if found != archived {
		testingContext.Fatalf("archive membership mismatch for exact session ID")
	}
}

func sortedRealGrokKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
