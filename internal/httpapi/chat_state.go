package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"wisp/internal/events"
)

func (s *Server) chatOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 200 {
		jsonOut(w, 400, map[string]string{"error": "search is too long"})
		return
	}
	rows, err := s.wa.ChatOptions(r.Context(), q)
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "Could not load chats and WhatsApp contacts. Try again."})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, 200, rows)
}
func (s *Server) changeChatState(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Action  string `json:"action"`
		MuteFor string `json:"mute_for"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		jsonOut(w, 400, map[string]string{"error": "Invalid chat action"})
		return
	}
	id = s.st.ResolveChat(id)
	var until int64
	switch in.Action {
	case "open":
		if err := s.wa.OpenChat(r.Context(), id); err != nil {
			jsonOut(w, 404, map[string]string{"error": "Chat or WhatsApp contact not found"})
			return
		}
	case "close":
	case "mute":
		switch in.MuteFor {
		case "8h":
			until = time.Now().Add(8 * time.Hour).Unix()
		case "1w":
			until = time.Now().Add(7 * 24 * time.Hour).Unix()
		case "forever":
			until = -1
		case "off":
			until = 0
		default:
			jsonOut(w, 400, map[string]string{"error": "Choose 8h, 1w, forever or off"})
			return
		}
	default:
		jsonOut(w, 400, map[string]string{"error": "Invalid chat action"})
		return
	}
	p, err := s.st.ChangeChatState(id, in.Action, until)
	if err != nil {
		code := 500
		if errors.Is(err, sql.ErrNoRows) {
			code = 404
		}
		jsonOut(w, code, map[string]string{"error": "Could not update chat"})
		return
	}
	s.bus.Publish(events.Event{Type: "chat-state", Data: p})
	jsonOut(w, 200, p)
}
