package hub

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const clientDirectionWait = 300 * time.Millisecond

func TestClientTerminalRequestIsRejectedBeforeAgent(testInstance *testing.T) {
	workspace := testInstance.TempDir()
	marker := filepath.Join(workspace, "api-marker")
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), workspace, nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()
	request := map[string]any{"jsonrpc": "2.0", "id": "terminal-request", "method": "terminal/create", "params": map[string]any{
		"sessionId": "test-session", "command": "printf compromised > api-marker", "cwd": workspace,
	}}
	if operationError := client.WriteJSON(request); operationError != nil {
		testInstance.Fatal(operationError)
	}
	response := readResponse(testInstance, client, "terminal-request")
	errorObject, _ := response["error"].(map[string]any)
	if errorObject["code"] != float64(methodNotCallableErrorCode) {
		testInstance.Fatalf("error = %#v", response)
	}
	select {
	case message := <-agent.messages:
		testInstance.Fatalf("request forwarded: %#v", message)
	case <-time.After(clientDirectionWait):
	}
	if _, statOperationError := os.Stat(marker); !os.IsNotExist(statOperationError) {
		testInstance.Fatalf("marker exists or stat failed: %v", statOperationError)
	}
	if count := hubInstance.terminals.count(); count != 0 {
		testInstance.Fatalf("terminal count = %d", count)
	}
}

func TestClientUnknownTerminalNotificationIsDropped(testInstance *testing.T) {
	workspace := testInstance.TempDir()
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), workspace, nil)
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()
	if operationError := client.WriteJSON(map[string]any{"jsonrpc": "2.0", "method": "terminal/input", "params": map[string]any{"sessionId": "test-session", "stdin": "bad"}}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	if count := hubInstance.terminals.count(); count != 0 {
		testInstance.Fatalf("terminal count = %d", count)
	}
	_ = client.SetReadDeadline(time.Now().Add(clientDirectionWait))
	for {
		var responseObject map[string]any
		if operationError := client.ReadJSON(&responseObject); operationError != nil {
			break
		}
		if _, hasIdentifier := responseObject["id"]; hasIdentifier {
			testInstance.Fatalf("notification unexpectedly produced response: %#v", responseObject)
		}
	}
	select {
	case message := <-agent.messages:
		testInstance.Fatalf("notification forwarded: %#v", message)
	case <-time.After(clientDirectionWait):
	}
}

func TestCallRPCTerminalRequestRejectedBeforeEnsure(testInstance *testing.T) {
	workspace := testInstance.TempDir()
	marker := filepath.Join(workspace, "api-marker")
	var ensureCalls atomic.Int32
	providerInstance := &testProvider{workingDirectory: workspace, sessionDirectories: map[string]string{}}
	hubInstance := New("ws://127.0.0.1:1", providerInstance, providerInstance, func(context.Context) error {
		ensureCalls.Add(1)
		return nil
	}, nil)
	defer hubInstance.Close()
	_, operationError := hubInstance.CallRPC(context.Background(), "terminal/create", map[string]any{"sessionId": "test-session", "command": "printf compromised > api-marker", "cwd": workspace})
	if operationError == nil {
		testInstance.Fatal("CallRPC unexpectedly succeeded")
	}
	if ensureCalls.Load() != 0 {
		testInstance.Fatalf("Ensure calls = %d", ensureCalls.Load())
	}
	if _, statOperationError := os.Stat(marker); !os.IsNotExist(statOperationError) {
		testInstance.Fatalf("marker exists or stat failed: %v", statOperationError)
	}
	if count := hubInstance.terminals.count(); count != 0 {
		testInstance.Fatalf("terminal count = %d", count)
	}
}
