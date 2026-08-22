package hub

import (
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
	hubInstance := New(agent.websocketURL(), func() string { return testInstance.TempDir() }, nil, nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	command := "printf 'hello'; exit 7"
	created := reverseCall(testInstance, agent, "terminal-create", "terminal/create", map[string]any{
		"command": command,
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
		"terminalId": terminalID,
	})
	waitResult := rpcResult(testInstance, waited)
	if waitResult["exitCode"] != float64(7) || waitResult["signal"] != nil {
		testInstance.Fatalf("terminal/wait_for_exit result = %#v", waited)
	}

	output := reverseCall(testInstance, agent, "terminal-output", "terminal/output", map[string]any{
		"terminalId": terminalID,
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
		"terminalId": terminalID,
	})
	if result := rpcResult(testInstance, released); len(result) != 0 {
		testInstance.Fatalf("terminal/release result = %#v", released)
	}
	assertNoClientRPCNotification(testInstance, client)
}

func TestReverseFileWriteThenRead(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := New(agent.websocketURL(), func() string { return testInstance.TempDir() }, nil, nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	path := filepath.Join(testInstance.TempDir(), "nested", "sample.txt")
	content := "one\r\ntwo\nthree"
	written := reverseCall(testInstance, agent, "fs-write", "fs/write_text_file", map[string]any{
		"path": path, "content": content,
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
		"path": path, "line": 2, "limit": 1,
	})
	if result := rpcResult(testInstance, read); result["content"] != "two\n" {
		testInstance.Fatalf("fs/read_text_file result = %#v", read)
	}
	readNotification := readMethod(testInstance, client, "_x.ai/remote/client_rpc")
	assertFileClientRPC(testInstance, readNotification, "fs/read_text_file", path)
}

func TestReversePermissionAutomaticallySelectsAllowOption(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := New(agent.websocketURL(), func() string { return testInstance.TempDir() }, nil, nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	response := reverseCall(testInstance, agent, "permission", "session/request_permission", map[string]any{
		"options": []any{
			map[string]any{"optionId": "deny_once"},
			map[string]any{"optionId": "approve_once"},
		},
		"toolCall": map[string]any{"title": "Read project files"},
	})
	outcome, _ := rpcResult(testInstance, response)["outcome"].(map[string]any)
	if outcome["outcome"] != "selected" || outcome["optionId"] != "approve_once" {
		testInstance.Fatalf("permission result = %#v", response)
	}
	notification := readMethod(testInstance, client, "_x.ai/remote/auto_permission")
	params := rpcParams(testInstance, notification)
	if params["optionId"] != "approve_once" || params["tool"] != "Read project files" {
		testInstance.Fatalf("auto_permission params = %#v", params)
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
