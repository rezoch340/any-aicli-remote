package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/grok-remote/grok-remote-app/backend/internal/history"
)

func TestServerSessionRoutes(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	sessionID := "session-route-test"
	directory := filepath.Join(fixture.server.configuration.SessionsDirectory, history.EncodeSessionWorkingDirectory(fixture.root), sessionID)
	if errorValue := os.MkdirAll(directory, 0o755); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	writeRouteJSON(testingContext, filepath.Join(directory, "summary.json"), map[string]any{
		"remote_title": "Original title",
		"cwd":          fixture.root,
	})
	writeRouteJSON(testingContext, filepath.Join(directory, "signals.json"), map[string]any{
		"context_tokens_used": 25, "context_window_tokens": 100,
		"turnCount": 2, "toolCallCount": 1, "primary_model_id": "grok-test",
	})
	update := map[string]any{
		"jsonrpc": "2.0", "method": "session/update",
		"params": map[string]any{"sessionId": sessionID, "update": map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": "hello from disk"},
		}},
	}
	line, _ := json.Marshal(update)
	if errorValue := os.WriteFile(filepath.Join(directory, "updates.jsonl"), append(line, '\n'), 0o644); errorValue != nil {
		testingContext.Fatal(errorValue)
	}

	historyResponse := fixture.request(testingContext, http.MethodGet, "/api/session/history?sessionId="+sessionID+"&chat_only=1", nil, remotePeer, true)
	assertStatus(testingContext, historyResponse, http.StatusOK)
	historyBody := decodeObject(testingContext, historyResponse)
	if historyBody["ok"] != true || historyBody["title"] != "Original title" || historyBody["count"] != float64(1) {
		testingContext.Fatalf("history = %#v", historyBody)
	}

	titlesResponse := fixture.request(testingContext, http.MethodPost, "/api/session/titles", map[string]any{"ids": sessionID}, remotePeer, true)
	assertStatus(testingContext, titlesResponse, http.StatusOK)
	titles := decodeObject(testingContext, titlesResponse)
	if titles["count"] != float64(1) {
		testingContext.Fatalf("titles = %#v", titles)
	}

	signalsResponse := fixture.request(testingContext, http.MethodGet, "/api/session/signals?id="+sessionID, nil, remotePeer, true)
	assertStatus(testingContext, signalsResponse, http.StatusOK)
	signals := decodeObject(testingContext, signalsResponse)
	if signals["contextWindowUsage"] != float64(25) || signals["primaryModelId"] != "grok-test" {
		testingContext.Fatalf("signals = %#v", signals)
	}

	archivedResponse := fixture.request(testingContext, http.MethodPost, "/api/session/archived", map[string]any{"sessionId": sessionID, "archived": true}, remotePeer, true)
	assertStatus(testingContext, archivedResponse, http.StatusOK)
	archived := decodeObject(testingContext, archivedResponse)
	if archived["count"] != float64(1) {
		testingContext.Fatalf("archived = %#v", archived)
	}
	archivedResponse = fixture.request(testingContext, http.MethodGet, "/api/session/archived", nil, remotePeer, true)
	assertStatus(testingContext, archivedResponse, http.StatusOK)
	if decodeObject(testingContext, archivedResponse)["count"] != float64(1) {
		testingContext.Fatalf("archived get = %s", archivedResponse.Body.String())
	}

	renameResponse := fixture.request(testingContext, http.MethodPost, "/api/session/rename", map[string]any{"sessionId": sessionID, "title": "Renamed"}, remotePeer, true)
	assertStatus(testingContext, renameResponse, http.StatusOK)
	rename := decodeObject(testingContext, renameResponse)
	if rename["title"] != "Renamed" || rename["previous"] != "Original title" {
		testingContext.Fatalf("rename = %#v", rename)
	}
	var summary map[string]any
	readRouteJSON(testingContext, filepath.Join(directory, "summary.json"), &summary)
	if summary["remote_title"] != "Renamed" || summary["generated_title"] != "Renamed" || summary["session_summary"] != "Renamed" {
		testingContext.Fatalf("renamed summary = %#v", summary)
	}
}

func TestServerSessionRouteErrors(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	missingID := fixture.request(testingContext, http.MethodGet, "/api/session/history", nil, remotePeer, true)
	assertStatus(testingContext, missingID, http.StatusBadRequest)
	missing := fixture.request(testingContext, http.MethodGet, "/api/session/history?sessionId=missing", nil, remotePeer, true)
	assertStatus(testingContext, missing, http.StatusNotFound)
	body := decodeObject(testingContext, missing)
	if body["ok"] != false || body["error"] != "session dir not found" {
		testingContext.Fatalf("missing history = %#v", body)
	}
	badArchive := fixture.request(testingContext, http.MethodPost, "/api/session/archived", map[string]any{}, remotePeer, true)
	assertStatus(testingContext, badArchive, http.StatusBadRequest)
}

func writeRouteJSON(testingContext *testing.T, path string, value any) {
	testingContext.Helper()
	data, errorValue := json.Marshal(value)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if errorValue := os.WriteFile(path, data, 0o644); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
}

func readRouteJSON(testingContext *testing.T, path string, target any) {
	testingContext.Helper()
	data, errorValue := os.ReadFile(path)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if errorValue := json.Unmarshal(data, target); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
}
