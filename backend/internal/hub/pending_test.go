package hub

import (
	"strings"
	"testing"
	"time"
)

func pendingRequestCount(hubInstance *Hub) int {
	hubInstance.stateMutex.Lock()
	defer hubInstance.stateMutex.Unlock()
	return len(hubInstance.pending)
}

func TestPendingRequestWithoutAgentResponseExpires(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	hubInstance.pendingTimeout = 40 * time.Millisecond
	defer hubInstance.Close()
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	if operationError := client.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "timeout-request", "method": "test/no-response", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	select {
	case <-agent.messages:
	case <-time.After(2 * time.Second):
		testInstance.Fatal("request did not reach fake agent")
	}

	response := readResponse(testInstance, client, "timeout-request")
	rpcError, valid := response["error"].(map[string]any)
	if !valid || !strings.Contains(stringValue(rpcError["message"]), "timed out") {
		testInstance.Fatalf("timeout response = %#v", response)
	}
	waitForTestCondition(testInstance, "expired pending request removal", func() bool {
		return pendingRequestCount(hubInstance) == 0
	})
}

func TestCloseCancelsPendingRequestExpiry(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	client, closeClient := connectHubClient(testInstance, hubInstance)
	defer closeClient()

	if operationError := client.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "close-pending", "method": "test/no-response", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	select {
	case <-agent.messages:
	case <-time.After(2 * time.Second):
		testInstance.Fatal("request did not reach fake agent")
	}
	waitForTestCondition(testInstance, "pending request registration", func() bool {
		return pendingRequestCount(hubInstance) == 1
	})

	closeReturned := make(chan struct{})
	go func() {
		hubInstance.Close()
		close(closeReturned)
	}()
	select {
	case <-closeReturned:
	case <-time.After(2 * time.Second):
		testInstance.Fatal("hub close waited for pending request timeout")
	}
	if pendingCount := pendingRequestCount(hubInstance); pendingCount != 0 {
		testInstance.Fatalf("pending requests after close = %d", pendingCount)
	}
}

func TestPendingRequestLimitsRejectClientAndGlobalFloods(testInstance *testing.T) {
	agent := newFakeAgent(testInstance)
	hubInstance := newTestHub(agent.websocketURL(), testInstance.TempDir(), nil)
	hubInstance.pendingClientLimit = 1
	hubInstance.pendingLimit = 2
	defer hubInstance.Close()

	first, closeFirst := connectHubClient(testInstance, hubInstance)
	defer closeFirst()
	second, closeSecond := connectHubClient(testInstance, hubInstance)
	defer closeSecond()
	third, closeThird := connectHubClient(testInstance, hubInstance)
	defer closeThird()

	if operationError := first.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "first-accepted", "method": "test/no-response", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	select {
	case <-agent.messages:
	case <-time.After(2 * time.Second):
		testInstance.Fatal("first request did not reach fake agent")
	}
	if operationError := first.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "first-overflow", "method": "test/no-response", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	clientLimitResponse := readResponse(testInstance, first, "first-overflow")
	clientLimitError, valid := clientLimitResponse["error"].(map[string]any)
	if !valid || !strings.Contains(stringValue(clientLimitError["message"]), "client has too many") {
		testInstance.Fatalf("per-client limit response = %#v", clientLimitResponse)
	}

	if operationError := second.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "second-accepted", "method": "test/no-response", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	select {
	case <-agent.messages:
	case <-time.After(2 * time.Second):
		testInstance.Fatal("second request did not reach fake agent")
	}
	if operationError := third.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": "global-overflow", "method": "test/no-response", "params": map[string]any{},
	}); operationError != nil {
		testInstance.Fatal(operationError)
	}
	globalLimitResponse := readResponse(testInstance, third, "global-overflow")
	globalLimitError, valid := globalLimitResponse["error"].(map[string]any)
	if !valid || !strings.Contains(stringValue(globalLimitError["message"]), "too many in-flight") {
		testInstance.Fatalf("global limit response = %#v", globalLimitResponse)
	}
	select {
	case unexpected := <-agent.messages:
		testInstance.Fatalf("rejected request reached fake agent: %#v", unexpected)
	case <-time.After(100 * time.Millisecond):
	}
	if pendingCount := pendingRequestCount(hubInstance); pendingCount != 2 {
		testInstance.Fatalf("pending requests = %d, want 2", pendingCount)
	}
}
