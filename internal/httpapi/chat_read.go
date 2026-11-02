package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"wisp/internal/events"
	"wisp/internal/store"
)

func (s *Server) markChatRead(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(405)
		return
	}
	var in struct {
		Through *int64 `json:"through_sequence"`
		LastID  string `json:"last_message_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.Through == nil || *in.Through < 0 {
		jsonOut(w, 400, map[string]string{"error": "Invalid read sequence"})
		return
	}
	p, err := s.st.MarkChatReadThrough(id, *in.Through, in.LastID)
	if err != nil {
		code := 500
		if errors.Is(err, store.ErrInvalidReadSequence) || errors.Is(err, store.ErrInvalidReadBoundary) {
			code = 400
		}
		if errors.Is(err, sql.ErrNoRows) {
			code = 404
		}
		jsonOut(w, code, map[string]string{"error": "Could not mark chat as read"})
		return
	}
	s.bus.Publish(events.Event{Type: "unread", Data: p})
	if mode := s.st.ReadMode(); mode != store.ReadModeLocal {
		log.Printf("[read-sync] event=read_acknowledged mode=%s through=%d explicit_boundary=%t", mode, *in.Through, in.LastID != "")
	}
	s.wa.WakeReadSync()
	jsonOut(w, 200, p)
}
