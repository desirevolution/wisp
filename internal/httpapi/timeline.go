package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	limit := 50
	if x, _ := strconv.Atoi(r.URL.Query().Get("limit")); x > 0 {
		limit = x
	}
	out, err := s.st.SearchContext(r.Context(), r.URL.Query().Get("q"), limit, r.URL.Query().Get("chat"))
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if rows, ok := out["messages"].([]map[string]any); ok {
		s.wa.ResolveMediaDimensions(r.Context(), rows)
		s.wa.ResolveMentionRows(r.Context(), rows, "chat_id", "body", "id")
	}
	jsonOut(w, 200, out)
}
func (s *Server) messageContext(w http.ResponseWriter, r *http.Request, chat, message string) {
	rows, err := s.st.MessageContext(chat, message, 25)
	if errors.Is(err, sql.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "Message not found"})
		return
	}
	if err != nil {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "Could not load message context"})
		return
	}
	s.enrichTimeline(r, rows)
	jsonOut(w, http.StatusOK, rows)
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request, id string) {
	limit := 50
	if x, _ := strconv.Atoi(r.URL.Query().Get("limit")); x > 0 {
		limit = x
	}
	if limit > 200 {
		limit = 200
	}
	var before *int64
	x := r.URL.Query().Get("beforeTs")
	if x == "" {
		x = r.URL.Query().Get("before")
	}
	if x != "" {
		if n, e := strconv.ParseInt(x, 10, 64); e == nil {
			before = &n
		}
	}
	beforeID := r.URL.Query().Get("beforeId")
	msgs, e := s.st.Messages(id, limit, before, beforeID)
	if e != nil {
		jsonOut(w, 500, map[string]string{"error": e.Error()})
		return
	}

	// Gap healing can require multiple WhatsApp network round-trips. Never do it
	// while opening the newest page: the UI must be able to paint locally stored
	// messages immediately. Older-page requests are already an explicit history
	// operation, so they remain the safe place to heal a detected historical gap.
	const gapHealThreshold = int64(7 * 24 * 60 * 60)
	const maxGapHealPages = 12
	if before != nil && r.URL.Query().Get("localOnly") != "1" {
		if gap, gapErr := s.st.LargestHistoryGap(id, gapHealThreshold); gapErr != nil {
			fmt.Printf("[history-gap-fill] detect failed chat=%s: %v\n", id, gapErr)
		} else if gap != nil {
			anchorID, anchorTS, lowerTS := gap.UpperID, gap.UpperTS, gap.LowerTS
			for page := 1; page <= maxGapHealPages && anchorTS > lowerTS; page++ {
				ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
				fetchErr := s.wa.FetchOlder(ctx, id, anchorID, anchorTS, limit)
				cancel()
				if fetchErr != nil {
					fmt.Printf("[history-gap-fill] fetch failed chat=%s page=%d: %v\n", id, page, fetchErr)
					break
				}
				pageBefore := anchorTS
				rows, queryErr := s.st.Messages(id, 200, &pageBefore, anchorID)
				if queryErr != nil {
					break
				}
				nextID := ""
				var nextTS int64
				for _, row := range rows {
					rid, okID := row["id"].(string)
					rts, okTS := row["timestamp"].(int64)
					if !okID || !okTS || rts <= lowerTS || rts >= anchorTS {
						continue
					}
					if nextID == "" || rts < nextTS || (rts == nextTS && rid < nextID) {
						nextID, nextTS = rid, rts
					}
				}
				if nextID == "" {
					break
				}
				anchorID, anchorTS = nextID, nextTS
			}
			msgs, e = s.st.Messages(id, limit, before, beforeID)
			if e != nil {
				jsonOut(w, 500, map[string]string{"error": e.Error()})
				return
			}
		}
	}

	// A WhatsApp history page may contain non-visible protocol/control messages.
	// A partial Wisp page therefore does not mean history ended. Refill until the
	// browser has a full visible page or WhatsApp explicitly marks history exhausted.
	if before != nil && r.URL.Query().Get("localOnly") != "1" {
		for attempt := 1; len(msgs) < limit && !s.st.HistoryExhausted(id) && attempt <= 3; attempt++ {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			err := s.wa.FetchOlder(ctx, id, beforeID, *before, limit)
			cancel()
			if err != nil {
				fmt.Printf("[history] fetch older failed chat=%s: %v\n", id, err)
				break
			}
			msgs, e = s.st.Messages(id, limit, before, beforeID)
			if e != nil {
				jsonOut(w, 500, map[string]string{"error": e.Error()})
				return
			}
		}
	}
	s.enrichTimeline(r, msgs)
	jsonOut(w, 200, msgs)
}

func (s *Server) enrichTimeline(r *http.Request, rows []map[string]any) {
	for _, msg := range rows {
		if hm, ok := msg["has_media"].(int64); ok && hm != 0 {
			messageID, _ := msg["id"].(string)
			kind, _ := msg["type"].(string)
			status, _ := msg["media_status"].(string)
			if messageID != "" {
				s.queueVisibleMedia(messageID, kind, status)
			}
		}
	}
	s.wa.ResolveMediaDimensions(r.Context(), rows)
	s.wa.ResolveMentionRows(r.Context(), rows, "chat_id", "body", "id")
	s.wa.ResolveLocationRows(r.Context(), rows)
}
