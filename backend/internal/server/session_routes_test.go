package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestServerSessionRoutes(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	sessionID := "session-route-test"
	directory := filepath.Join(fixture.sessions, "route-project", sessionID)
	if errorValue := os.MkdirAll(directory, 0o755); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	writeRouteJSON(testingContext, filepath.Join(directory, "summary.json"), map[string]any{
		"info":         map[string]any{"id": sessionID, "cwd": fixture.root},
		"remote_title": "Original title", "session_summary": "Route summary",
		"created_at": "2026-01-01T00:00:00Z", "last_active_at": "2026-01-02T00:00:00Z",
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
	if errorValue := os.WriteFile(filepath.Join(directory, "chat_history.jsonl"), []byte("{\"type\":\"user\",\"content\":\"hello from chat history\",\"timestamp\":1700000000}\n"), 0o644); errorValue != nil {
		testingContext.Fatal(errorValue)
	}

	sessionsResponse := fixture.request(testingContext, http.MethodGet, "/api/sessions", nil, remotePeer, true)
	assertStatus(testingContext, sessionsResponse, http.StatusOK)
	sessionsBody := decodeObject(testingContext, sessionsResponse)
	if sessionsBody["providerId"] != "grok" || sessionsBody["count"] != float64(3) {
		testingContext.Fatalf("sessions = %#v", sessionsBody)
	}
	var routeMetadata map[string]any
	for _, rawSession := range sessionsBody["sessions"].([]any) {
		metadata, _ := rawSession.(map[string]any)
		if metadata["sessionId"] == sessionID {
			routeMetadata = metadata
			break
		}
	}
	if routeMetadata == nil || routeMetadata["providerId"] != "grok" || routeMetadata["projectDir"] != fixture.root || routeMetadata["title"] != "Original title" || routeMetadata["sourcePath"] == "" || routeMetadata["resumeCommand"] == "" {
		testingContext.Fatalf("route session metadata = %#v", routeMetadata)
	}
	messagesResponse := fixture.request(testingContext, http.MethodGet, "/api/sessions/"+sessionID+"/messages", nil, remotePeer, true)
	assertStatus(testingContext, messagesResponse, http.StatusOK)
	messagesBody := decodeObject(testingContext, messagesResponse)
	messages, _ := messagesBody["messages"].([]any)
	if messagesBody["providerId"] != "grok" || messagesBody["count"] != float64(1) || len(messages) != 1 || messages[0].(map[string]any)["content"] != "hello from chat history" {
		testingContext.Fatalf("messages = %#v", messagesBody)
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
	if body["ok"] != false || body["error"] != "session not found" {
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
