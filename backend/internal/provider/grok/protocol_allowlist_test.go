package grok

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestPrepareClientRequestAllowlist(testContext *testing.T) {
	workingDirectory := testContext.TempDir()
	providerInstance := New(Config{SessionsDirectory: filepath.Join(workingDirectory, "sessions")})
	allowedMethods := []struct {
		method             string
		parameters         map[string]any
		expectedSessionID  string
		expectsSessionNeed bool
	}{
		{method: acp.AgentMethodInitialize, parameters: map[string]any{}},
		{method: acp.AgentMethodSessionNew, parameters: map[string]any{"cwd": workingDirectory}},
		{method: acp.AgentMethodSessionLoad, parameters: map[string]any{"sessionId": "load-session"}, expectedSessionID: "load-session", expectsSessionNeed: true},
		{method: acp.AgentMethodSessionPrompt, parameters: map[string]any{"sessionId": "prompt-session"}, expectedSessionID: "prompt-session", expectsSessionNeed: true},
		{method: acp.AgentMethodSessionCancel, parameters: map[string]any{"sessionId": "cancel-session"}, expectedSessionID: "cancel-session", expectsSessionNeed: true},
		{method: grokMethodSessionSetModel, parameters: map[string]any{"sessionId": "model-session"}, expectedSessionID: "model-session", expectsSessionNeed: true},
		{method: grokMethodRemotePing, parameters: map[string]any{"t": "ping"}},
	}
	for _, allowedMethod := range allowedMethods {
		preparedRequest, operationError := providerInstance.PrepareClientRequest(context.Background(), allowedMethod.method, allowedMethod.parameters)
		if operationError != nil {
			testContext.Errorf("method %s rejected: %v", allowedMethod.method, operationError)
			continue
		}
		if preparedRequest.SessionID != allowedMethod.expectedSessionID || preparedRequest.RequiresSession != allowedMethod.expectsSessionNeed {
			testContext.Errorf("method %s session handling = %#v", allowedMethod.method, preparedRequest)
		}
	}
}

func TestPrepareClientRequestRejectsUnlistedMethods(testContext *testing.T) {
	providerInstance := New(Config{SessionsDirectory: testContext.TempDir()})
	rejectedMethods := []string{
		acp.ClientMethodTerminalCreate,
		acp.ClientMethodFsReadTextFile,
		acp.ClientMethodSessionRequestPermission,
		"agent/exec",
		"provider/run_command",
	}
	for _, rejectedMethod := range rejectedMethods {
		_, operationError := providerInstance.PrepareClientRequest(context.Background(), rejectedMethod, map[string]any{})
		if operationError == nil || !strings.Contains(operationError.Error(), "unsupported client RPC method") {
			testContext.Errorf("method %s error = %v", rejectedMethod, operationError)
		}
	}
}
