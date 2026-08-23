package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
)

const (
	smokeSecretValue      = "lifecycle-smoke-secret-value"
	smokeOperationTimeout = 8 * time.Second
	smokePollInterval     = 20 * time.Millisecond
	smokeFileMode         = 0o600
)

type healthResponse struct {
	OK             bool `json:"ok"`
	AgentListening bool `json:"agent_listening"`
}

type pairingResponse struct {
	LANIP           string `json:"lan_ip"`
	PairingURL      string `json:"pairing_url"`
	PairingDeepLink string `json:"pairing_deep_link"`
}

func TestDaemonLifecycleSmoke(testingContext *testing.T) {
	paths := newSmokePaths(testingContext)
	resetSmokeEnvironment(testingContext, paths.homeDirectory)
	listenerPorts := distinctSmokePorts(testingContext)
	document := prepareSmokeDocument(testingContext, paths, listenerPorts)
	applySmokeDocument(testingContext, paths.configurationPath, document)
	assertPrivateFileMode(testingContext, paths.configurationPath)
	assertSmokeDocumentPreserved(testingContext, paths.configurationPath, document)
	assertLaunchArguments(testingContext, paths.configurationPath, paths.secretPath)
	for cycleNumber := 0; cycleNumber < 2; cycleNumber++ {
		materializeSmokeSecret(testingContext, paths.secretPath)
		runSmokeCycle(testingContext, paths, document.Network.Port, document.Agent.Port)
		assertNoSessionOrWorkspaceState(testingContext, paths)
	}
	assertNoLegacyHomeState(testingContext, paths.homeDirectory)
}

func runSmokeCycle(testingContext *testing.T, paths smokePaths, daemonPort, agentPort int) {
	testingContext.Helper()
	executionContext, cancel := context.WithCancel(context.Background())
	completionChannel := make(chan error, 1)
	go func() {
		completionChannel <- runDaemonWithContext(executionContext, daemonLaunchArguments(paths.configurationPath, paths.secretPath), io.Discard)
	}()
	completionReceived := false
	defer func() {
		if !completionReceived {
			awaitDaemonCompletion(testingContext, cancel, completionChannel)
			return
		}
		cancel()
	}()

	client := &http.Client{Timeout: smokeOperationTimeout}
	health := awaitHealthyDaemon(testingContext, client, daemonPort)
	if !health.OK || health.AgentListening {
		testingContext.Fatalf("unexpected health payload: %#v", health)
	}
	assertAuthenticatedPairing(testingContext, client, daemonPort, daemonPort)
	assertRuntimeConfigDoesNotPersistSecrets(testingContext, paths.dataDirectory)
	removeSmokeSecret(testingContext, paths.secretPath)
	stopDaemon(testingContext, client, daemonPort)
	awaitStoppedDaemon(testingContext, completionChannel)
	completionReceived = true
	assertHealthUnavailable(testingContext, client, daemonPort)
	assertPortAvailable(testingContext, agentPort)
}

func awaitDaemonCompletion(testingContext *testing.T, cancel context.CancelFunc, completionChannel <-chan error) {
	testingContext.Helper()
	cancel()
	select {
	case operationError := <-completionChannel:
		if operationError != nil {
			testingContext.Errorf("daemon returned after cancellation: %v", operationError)
		}
	case <-time.After(smokeOperationTimeout):
		testingContext.Error("daemon goroutine did not exit after cancellation")
	}
}

func awaitHealthyDaemon(testingContext *testing.T, client *http.Client, daemonPort int) healthResponse {
	testingContext.Helper()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/health", daemonPort)
	deadline := time.Now().Add(smokeOperationTimeout)
	for time.Now().Before(deadline) {
		response, operationError := client.Get(endpoint)
		if operationError == nil {
			health, decodeError := decodeHealthResponse(response)
			if closeError := response.Body.Close(); closeError != nil {
				testingContext.Fatal(closeError)
			}
			if decodeError == nil && response.StatusCode == http.StatusOK && health.OK {
				return health
			}
		}
		time.Sleep(smokePollInterval)
	}
	testingContext.Fatalf("daemon health did not become available at %s", endpoint)
	return healthResponse{}
}

func decodeHealthResponse(response *http.Response) (healthResponse, error) {
	responseBody, operationError := io.ReadAll(response.Body)
	if operationError != nil {
		return healthResponse{}, operationError
	}
	var health healthResponse
	if operationError = json.Unmarshal(responseBody, &health); operationError != nil {
		return healthResponse{}, operationError
	}
	return health, nil
}

func assertAuthenticatedPairing(testingContext *testing.T, client *http.Client, daemonPort, pairingPort int) {
	testingContext.Helper()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/config.json", daemonPort)
	unauthenticatedRequest, operationError := http.NewRequest(http.MethodGet, endpoint, nil)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	unauthenticatedResponse, operationError := client.Do(unauthenticatedRequest)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if closeError := unauthenticatedResponse.Body.Close(); closeError != nil {
		testingContext.Fatal(closeError)
	}
	if unauthenticatedResponse.StatusCode != http.StatusUnauthorized {
		testingContext.Fatalf("unexpected unauthenticated status %d", unauthenticatedResponse.StatusCode)
	}
	authenticatedRequest, operationError := http.NewRequest(http.MethodGet, endpoint, nil)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	authenticatedRequest.Header.Set("X-Any-AI-CLI-Remote-Key", smokeSecretValue)
	authenticatedResponse, operationError := client.Do(authenticatedRequest)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	responseBody, operationError := io.ReadAll(authenticatedResponse.Body)
	if closeError := authenticatedResponse.Body.Close(); closeError != nil {
		testingContext.Fatal(closeError)
	}
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if authenticatedResponse.StatusCode != http.StatusOK {
		testingContext.Fatalf("unexpected authenticated status %d: %s", authenticatedResponse.StatusCode, responseBody)
	}
	var pairing pairingResponse
	if operationError = json.Unmarshal(responseBody, &pairing); operationError != nil {
		testingContext.Fatal(operationError)
	}
	expectedConfiguration := config.Config{Port: pairingPort, PairingSecret: smokeSecretValue, PublicHost: ""}
	if pairing.PairingURL != expectedConfiguration.PairingURL(pairing.LANIP) ||
		pairing.PairingDeepLink != expectedConfiguration.PairingDeepLink(pairing.LANIP) {
		testingContext.Fatalf("unexpected pairing payload: %#v", pairing)
	}
	if !strings.Contains(pairing.PairingURL, fmt.Sprintf(":%d", pairingPort)) || !strings.Contains(pairing.PairingURL, smokeSecretValue) {
		testingContext.Fatalf("pairing URL omits port or secret: %s", pairing.PairingURL)
	}
}

func stopDaemon(testingContext *testing.T, client *http.Client, daemonPort int) {
	testingContext.Helper()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/stack/stop", daemonPort)
	stopRequest, operationError := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"keep_agent":false}`))
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	stopRequest.Header.Set("X-Any-AI-CLI-Remote-Key", smokeSecretValue)
	stopRequest.Header.Set("Content-Type", "application/json")
	stopResponse, operationError := client.Do(stopRequest)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if closeError := stopResponse.Body.Close(); closeError != nil {
		testingContext.Fatal(closeError)
	}
	if stopResponse.StatusCode < http.StatusOK || stopResponse.StatusCode >= http.StatusMultipleChoices {
		testingContext.Fatalf("unexpected stop status %d", stopResponse.StatusCode)
	}
}

func awaitStoppedDaemon(testingContext *testing.T, completionChannel <-chan error) {
	testingContext.Helper()
	select {
	case operationError := <-completionChannel:
		if operationError != nil {
			testingContext.Fatal(operationError)
		}
	case <-time.After(smokeOperationTimeout):
		testingContext.Fatal("daemon did not stop")
	}
}

func assertHealthUnavailable(testingContext *testing.T, client *http.Client, daemonPort int) {
	testingContext.Helper()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/health", daemonPort)
	deadline := time.Now().Add(smokeOperationTimeout)
	for time.Now().Before(deadline) {
		response, operationError := client.Get(endpoint)
		if operationError != nil {
			return
		}
		if closeError := response.Body.Close(); closeError != nil {
			testingContext.Fatal(closeError)
		}
		time.Sleep(smokePollInterval)
	}
	testingContext.Fatalf("daemon health remained reachable at %s", endpoint)
}
