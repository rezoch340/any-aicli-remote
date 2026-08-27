package hub

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReverseTerminalLifecycle(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	workingDirectory := testInstance.TempDir()
	hubInstance := newTestHub(agent.websocketURL(), workingDirectory, nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	command := "printf 'hello'; exit 7"
	created := reverseCall(testInstance, agent, "terminal-create", "terminal/create", map[string]any{
		"sessionId": "terminal-session", "command": command,
	})
	createResult := rpcResult(testInstance, created)
	terminalID, _ := createResult["terminalId"].(string)
	if terminalID == "" {
		testInstance.Fatalf("terminal/create result = %#v", created)
	}
	createdNotification := readMethod(testInstance, client, "_x.ai/remote/client_rpc")
	createdParams := rpcParams(testInstance, createdNotification)
	if createdParams["method"] != "terminal/create" || createdParams["terminalId"] != terminalID ||
		createdParams["command"] != command || createdParams["ok"] != true {
		testInstance.Fatalf("terminal/create client_rpc params = %#v", createdParams)
	}

	waited := reverseCall(testInstance, agent, "terminal-wait", "terminal/wait_for_exit", map[string]any{
		"sessionId": "terminal-session", "terminalId": terminalID,
	})
	waitResult := rpcResult(testInstance, waited)
	if waitResult["exitCode"] != float64(7) || waitResult["signal"] != nil {
		testInstance.Fatalf("terminal/wait_for_exit result = %#v", waited)
	}

	output := reverseCall(testInstance, agent, "terminal-output", "terminal/output", map[string]any{
		"sessionId": "terminal-session", "terminalId": terminalID,
	})
	outputResult := rpcResult(testInstance, output)
	if outputResult["output"] != "hello" || outputResult["truncated"] != false {
		testInstance.Fatalf("terminal/output result = %#v", output)
	}
	exitStatus, _ := outputResult["exitStatus"].(map[string]any)
	if exitStatus["exitCode"] != float64(7) || exitStatus["signal"] != nil {
		testInstance.Fatalf("terminal/output exitStatus = %#v", exitStatus)
	}

	released := reverseCall(testInstance, agent, "terminal-release", "terminal/release", map[string]any{
		"sessionId": "terminal-session", "terminalId": terminalID,
	})
	if result := rpcResult(testInstance, released); len(result) != 0 {
		testInstance.Fatalf("terminal/release result = %#v", released)
	}
	assertNoClientRPCNotification(testInstance, client)
}

func TestReverseTerminalRejectsMissingOrDifferentSession(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	_, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	created := reverseCall(testInstance, agent, "terminal-scoped-create", "terminal/create", map[string]any{
		"sessionId": "owner-session", "command": "printf scoped",
	})
	terminalID, _ := rpcResult(testInstance, created)["terminalId"].(string)
	if terminalID == "" {
		testInstance.Fatalf("terminal/create result = %#v", created)
	}
	for _, testCase := range []struct {
		name   string
		params map[string]any
	}{
		{name: "missing", params: map[string]any{"terminalId": terminalID}},
		{name: "different", params: map[string]any{"sessionId": "other-session", "terminalId": terminalID}},
	} {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			response := reverseCall(testInstance, agent, "terminal-scoped-"+testCase.name, "terminal/output", testCase.params)
			if response["error"] == nil {
				testInstance.Fatalf("terminal/output crossed session boundary: %#v", response)
			}
		})
	}

	released := reverseCall(testInstance, agent, "terminal-scoped-release", "terminal/release", map[string]any{
		"sessionId": "owner-session", "terminalId": terminalID,
	})
	if result := rpcResult(testInstance, released); len(result) != 0 {
		testInstance.Fatalf("terminal/release result = %#v", released)
	}
}

func TestReverseFileRejectsMissingSessionEvenInsideLoadedWorkspace(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	workingDirectory := testInstance.TempDir()
	hubInstance := newTestHub(agent.websocketURL(), workingDirectory, nil)
	defer hubInstance.Close()
	_, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	response := reverseCall(testInstance, agent, "unscoped-read", "fs/read_text_file", map[string]any{
		"path": filepath.Join(workingDirectory, "inside.txt"),
	})
	if response["error"] == nil {
		testInstance.Fatalf("unscoped file read was accepted: %#v", response)
	}
}

func TestReverseFileWriteThenRead(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	workingDirectory := testInstance.TempDir()
	hubInstance := newTestHub(agent.websocketURL(), workingDirectory, nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	path := filepath.Join(workingDirectory, "nested", "sample.txt")
	content := "one\r\ntwo\nthree"
	written := reverseCall(testInstance, agent, "fs-write", "fs/write_text_file", map[string]any{
		"sessionId": "file-session", "path": path, "content": content,
	})
	if result := rpcResult(testInstance, written); len(result) != 0 {
		testInstance.Fatalf("fs/write_text_file result = %#v", written)
	}
	writtenNotification := readMethod(testInstance, client, "_x.ai/remote/client_rpc")
	assertFileClientRPC(testInstance, writtenNotification, "fs/write_text_file", path)
	writtenBytes, operationError := os.ReadFile(path)
	if operationError != nil {
		testInstance.Fatal(operationError)
	}
	if string(writtenBytes) != content {
		testInstance.Fatalf("written content = %q", writtenBytes)
	}

	read := reverseCall(testInstance, agent, "fs-read", "fs/read_text_file", map[string]any{
		"sessionId": "file-session", "path": path, "line": 2, "limit": 1,
	})
	if result := rpcResult(testInstance, read); result["content"] != "two\n" {
		testInstance.Fatalf("fs/read_text_file result = %#v", read)
	}
	readNotification := readMethod(testInstance, client, "_x.ai/remote/client_rpc")
	assertFileClientRPC(testInstance, readNotification, "fs/read_text_file", path)
}

func TestReverseWorkspaceOperationsRejectBoundRootReplacement(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	parentDirectory := testInstance.TempDir()
	workingDirectory := filepath.Join(parentDirectory, "workspace")
	originalDirectory := filepath.Join(parentDirectory, "workspace-original")
	replacementDirectory := testInstance.TempDir()
	if operationError := os.Mkdir(workingDirectory, 0o755); operationError != nil {
		testInstance.Fatal(operationError)
	}
	hubInstance := newTestHub(agent.websocketURL(), workingDirectory, nil)
	defer hubInstance.Close()
	_, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()
	if _, operationError := hubInstance.ResolveSessionWorkspace(context.Background(), "test", "bound-session"); operationError != nil {
		testInstance.Fatal(operationError)
	}
	if operationError := os.Rename(workingDirectory, originalDirectory); operationError != nil {
		testInstance.Fatal(operationError)
	}
	if operationError := os.Symlink(replacementDirectory, workingDirectory); operationError != nil {
		testInstance.Fatal(operationError)
	}

	outsideFile := filepath.Join(replacementDirectory, "escaped.txt")
	writeResponse := reverseCall(testInstance, agent, "root-replaced-write", "fs/write_text_file", map[string]any{
		"sessionId": "bound-session", "path": outsideFile, "content": "escaped",
	})
	if writeResponse["error"] == nil {
		testInstance.Fatalf("file write accepted replacement workspace: %#v", writeResponse)
	}
	if _, operationError := os.Stat(outsideFile); !errors.Is(operationError, os.ErrNotExist) {
		testInstance.Fatalf("replacement workspace was written: %v", operationError)
	}
	terminalResponse := reverseCall(testInstance, agent, "root-replaced-terminal", "terminal/create", map[string]any{
		"sessionId": "bound-session", "command": "pwd",
	})
	if terminalResponse["error"] == nil {
		testInstance.Fatalf("terminal accepted replacement workspace: %#v", terminalResponse)
	}
}

func TestReversePermissionRoutesOnlyToSubscribedSessionClient(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	subscribedClient, closeSubscribedClient := connectHubClient(testInstance, hubInstance)
	defer closeSubscribedClient()
	hubInstance.stateMutex.Lock()
	var subscribedServerClient *clientConnection
	for connectedClient := range hubInstance.clients {
		subscribedServerClient = connectedClient
	}
	if subscribedServerClient == nil {
		hubInstance.stateMutex.Unlock()
		testInstance.Fatal("subscribed server connection missing")
	}
	hubInstance.subscribeSessionClientLocked(subscribedServerClient, "test", "permission-session")
	hubInstance.stateMutex.Unlock()
	unrelatedClient, closeUnrelatedClient := connectHubClient(testInstance, hubInstance)
	defer closeUnrelatedClient()

	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": "permission", "method": "session/request_permission", "params": map[string]any{
		"sessionId": "permission-session",
		"options": []any{
			map[string]any{"optionId": "deny_once"},
			map[string]any{"optionId": "approve_once"},
		},
		"toolCall": map[string]any{"title": "Read project files"},
	}})
	permissionRequest := readMethod(testInstance, subscribedClient, "session/request_permission")
	params := rpcParams(testInstance, permissionRequest)
	clientPermissionIdentifier, valid := numericID(permissionRequest["id"])
	if !valid || params["providerId"] != "test" || params["sessionId"] != "permission-session" {
		testInstance.Fatalf("scoped permission request = %#v", permissionRequest)
	}

	if operationError := unrelatedClient.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": clientPermissionIdentifier,
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "approve_once"}},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	select {
	case unexpected := <-agent.messages:
		if unexpected["method"] == nil && idKey(unexpected["id"]) == idKey("permission") {
			testInstance.Fatalf("unrelated client answered permission: %#v", unexpected)
		}
	case <-time.After(150 * time.Millisecond):
	}

	if operationError := subscribedClient.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": clientPermissionIdentifier,
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "deny_once"}},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	response := readAgentResponse(testInstance, agent, "permission")
	outcome, _ := rpcResult(testInstance, response)["outcome"].(map[string]any)
	if outcome["outcome"] != "selected" || outcome["optionId"] != "deny_once" {
		testInstance.Fatalf("permission result = %#v", response)
	}
	if hubInstance.reverseActive.Load() != 0 {
		testInstance.Fatalf("permission request entered local reverse worker: %d", hubInstance.reverseActive.Load())
	}
}

func TestReversePermissionSurvivesClientDisconnectAndReplaysAfterReconnect(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	firstClient, closeFirstClient := connectHubClient(testInstance, hubInstance)
	hubInstance.stateMutex.Lock()
	var firstServerClient *clientConnection
	for connectedClient := range hubInstance.clients {
		firstServerClient = connectedClient
	}
	hubInstance.subscribeSessionClientLocked(firstServerClient, "test", "durable-session")
	hubInstance.stateMutex.Unlock()

	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": "durable-permission", "method": "session/request_permission", "params": map[string]any{
		"sessionId": "durable-session",
		"options":   []any{map[string]any{"optionId": "approve_once"}},
	}})
	firstRequest := readMethod(testInstance, firstClient, "session/request_permission")
	firstIdentifier, valid := numericID(firstRequest["id"])
	if !valid {
		testInstance.Fatalf("first permission id = %#v", firstRequest)
	}
	closeFirstClient()
	waitForTestCondition(testInstance, "first client removal", func() bool {
		hubInstance.stateMutex.Lock()
		defer hubInstance.stateMutex.Unlock()
		return len(hubInstance.clients) == 0
	})
	select {
	case unexpected := <-agent.messages:
		if unexpected["method"] == nil && idKey(unexpected["id"]) == idKey("durable-permission") {
			testInstance.Fatalf("disconnect cancelled permission: %#v", unexpected)
		}
	case <-time.After(150 * time.Millisecond):
	}

	secondClient, closeSecondClient := connectHubClient(testInstance, hubInstance)
	defer closeSecondClient()
	hubInstance.stateMutex.Lock()
	var secondServerClient *clientConnection
	for connectedClient := range hubInstance.clients {
		secondServerClient = connectedClient
	}
	hubInstance.subscribeSessionClientLocked(secondServerClient, "test", "durable-session")
	hubInstance.stateMutex.Unlock()
	secondRequest := readMethod(testInstance, secondClient, "session/request_permission")
	secondIdentifier, valid := numericID(secondRequest["id"])
	if !valid || secondIdentifier != firstIdentifier {
		testInstance.Fatalf("replayed permission = %#v", secondRequest)
	}
	if operationError := secondClient.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": secondIdentifier,
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "approve_once"}},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	response := readAgentResponse(testInstance, agent, "durable-permission")
	outcome, _ := rpcResult(testInstance, response)["outcome"].(map[string]any)
	if outcome["outcome"] != "selected" || outcome["optionId"] != "approve_once" {
		testInstance.Fatalf("replayed permission result = %#v", response)
	}
}

func TestReversePermissionWithoutMatchingClientCancels(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	defer hubInstance.Close()
	ensureContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if operationError := hubInstance.Ensure(ensureContext); operationError != nil {
		testInstance.Fatal(operationError)
	}
	response := reverseCall(testInstance, agent, "unattended-permission", "session/request_permission", map[string]any{
		"sessionId": "unattended-session",
		"options":   []any{map[string]any{"optionId": "approve_once"}},
	})
	outcome, _ := rpcResult(testInstance, response)["outcome"].(map[string]any)
	if outcome["outcome"] != "cancelled" || outcome["optionId"] != nil {
		testInstance.Fatalf("unattended permission result = %#v", response)
	}
}

func reverseCall(testInstance *testing.T, agent *fakeAgent, requestIdentifier, method string, params map[string]any) map[string]any {
	testInstance.Helper()
	agent.send(testInstance, map[string]any{"jsonrpc": "2.0", "id": requestIdentifier, "method": method, "params": params})
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case response := <-agent.messages:
			if response["method"] == nil && idKey(response["id"]) == idKey(requestIdentifier) {
				return response
			}
		case <-timer.C:
			testInstance.Fatalf("timed out waiting for reverse response id %q", requestIdentifier)
			return nil
		}
	}
}

func readAgentResponse(testInstance *testing.T, agent *fakeAgent, requestIdentifier any) map[string]any {
	testInstance.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case response := <-agent.messages:
			if response["method"] == nil && idKey(response["id"]) == idKey(requestIdentifier) {
				return response
			}
		case <-timer.C:
			testInstance.Fatalf("timed out waiting for reverse response id %v", requestIdentifier)
			return nil
		}
	}
}

func rpcResult(testInstance *testing.T, response map[string]any) map[string]any {
	testInstance.Helper()
	if response["error"] != nil {
		testInstance.Fatalf("unexpected RPC error: %#v", response["error"])
	}
	result, valid := response["result"].(map[string]any)
	if !valid {
		testInstance.Fatalf("RPC result = %#v", response)
	}
	return result
}

func rpcParams(testInstance *testing.T, object map[string]any) map[string]any {
	testInstance.Helper()
	params, valid := object["params"].(map[string]any)
	if !valid {
		testInstance.Fatalf("RPC params = %#v", object)
	}
	return params
}

func assertFileClientRPC(testInstance *testing.T, notification map[string]any, method, path string) {
	testInstance.Helper()
	params := rpcParams(testInstance, notification)
	if params["method"] != method || params["path"] != path || params["ok"] != true {
		testInstance.Fatalf("%s client_rpc params = %#v", method, params)
	}
}

func assertNoClientRPCNotification(testInstance *testing.T, connection *websocket.Conn) {
	testInstance.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	for {
		var object map[string]any
		operationError := connection.ReadJSON(&object)
		if operationError == nil {
			if object["method"] == "_x.ai/remote/client_rpc" {
				testInstance.Fatalf("unexpected client_rpc notification: %#v", object)
			}
			continue
		}
		var netError net.Error
		if errors.As(operationError, &netError) && netError.Timeout() {
			return
		}
		testInstance.Fatalf("read client notification: %v", operationError)
	}
}
