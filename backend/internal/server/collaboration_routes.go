package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/room"
	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

func (server *Server) handleRoomFeed(responseWriter http.ResponseWriter, request *http.Request) {
	messages, errorValue := server.room.FeedString(request.URL.Query().Get("since"), request.URL.Query().Get("limit"))
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	last := intQuery(request.URL.Query().Get("since"), 0)
	if len(messages) > 0 {
		last = messages[len(messages)-1].ID
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "messages": messages, "last": last, "limit": room.Limit})
}

func (server *Server) handleRoomSay(responseWriter http.ResponseWriter, request *http.Request) {
	var body struct {
		Who  string `json:"who"`
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	decodeLooseJSON(request, &body)
	result := server.room.Say(firstNonEmpty(body.Who, request.URL.Query().Get("who"), "agent"), firstNonEmpty(body.Text, request.URL.Query().Get("text")), firstNonEmpty(body.Kind, "say"))
	status := http.StatusOK
	if !result.OK {
		status = http.StatusInternalServerError
		if result.Error == "empty message" {
			status = http.StatusBadRequest
		}
	}
	writeJSON(responseWriter, status, result)
}

func (server *Server) handleRoomMembers(responseWriter http.ResponseWriter, _ *http.Request) {
	members, errorValue := server.room.Members(15 * time.Minute)
	if errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true, "members": members})
}

func (server *Server) handleRoomClear(responseWriter http.ResponseWriter, _ *http.Request) {
	if errorValue := server.room.Clear(); errorValue != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, errorValue)
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"ok": true})
}

func (server *Server) handleVoiceStatus(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.voice.Status())
}

func (server *Server) handleTTS(responseWriter http.ResponseWriter, request *http.Request) {
	var input voice.Request
	decodeLooseJSON(request, &input)
	audio, errorValue := server.voice.Synthesize(request.Context(), input)
	if errorValue != nil {
		switch {
		case errors.Is(errorValue, voice.APIKeyMissingError):
			writeJSON(responseWriter, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": errorValue.Error()})
		case errors.Is(errorValue, voice.TextRequiredError):
			writeText(responseWriter, http.StatusBadRequest, errorValue.Error())
		default:
			var upstream *voice.UpstreamError
			if errors.As(errorValue, &upstream) {
				status := http.StatusBadRequest
				if upstream.Status >= 500 {
					status = http.StatusBadGateway
				}
				writeJSON(responseWriter, status, map[string]any{"ok": false, "error": upstream.Error(), "status": upstream.Status})
				return
			}
			writeJSON(responseWriter, http.StatusBadGateway, map[string]any{"ok": false, "error": truncate(errorValue.Error(), 300)})
		}
		return
	}
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.Header().Set("Content-Type", audio.ContentType)
	responseWriter.Header().Set("X-Voice-Id", audio.VoiceID)
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write(audio.Data)
}
