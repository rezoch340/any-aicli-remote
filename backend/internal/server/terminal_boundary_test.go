package server

import (
	"net/http"
	"testing"
)

func TestRemoteTerminalExecutionRoutesAreAbsent(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	payload := map[string]any{
		"command": "ls",
		"args":    []string{"-la"},
		"stdin":   "whoami",
	}
	testCases := []struct {
		name   string
		target string
	}{
		{name: "terminal", target: "/api/terminal"},
		{name: "terminal exec", target: "/api/terminal/exec"},
		{name: "terminal input", target: "/api/terminal/input"},
		{name: "exec", target: "/api/exec"},
	}
	for _, testCase := range testCases {
		testingContext.Run(testCase.name, func(testingContext *testing.T) {
			response := fixture.request(testingContext, http.MethodPost, testCase.target, payload, remotePeer, true)
			assertStatus(testingContext, response, http.StatusNotFound)
		})
	}
}
