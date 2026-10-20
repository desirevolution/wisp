package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"go.mau.fi/whatsmeow"
)

var outgoingID = regexp.MustCompile(`^3EB0[0-9A-F]{18}$`)

// The browser reuses one ID for the optimistic row, live echo and explicit retry.
// Serialize duplicate requests for that ID and return an already stored result.
func (s *Service) beginOutgoing(ctx context.Context, chat, text, kind, replyTo string, ids []string) (string, bool, func(), error) {
	if len(ids) == 0 || ids[0] == "" {
		return "", false, func() {}, nil
	}
	id := ids[0]
	if !outgoingID.MatchString(id) {
		return "", false, func() {}, fmt.Errorf("%w: invalid client message ID", ErrInvalidAction)
	}
	for {
		s.outgoingMu.Lock()
		if s.outgoingPending == nil {
			s.outgoingPending = make(map[string]chan struct{})
		}
		if done, exists := s.outgoingPending[id]; exists {
			s.outgoingMu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return "", false, func() {}, ctx.Err()
			}
		}
		done := make(chan struct{})
		s.outgoingPending[id] = done
		s.outgoingMu.Unlock()
		release := func() { s.outgoingMu.Lock(); delete(s.outgoingPending, id); close(done); s.outgoingMu.Unlock() }
		m, err := s.store.MessageByID(id)
		if err == nil {
			release()
			if m.ChatID != s.store.ResolveChat(chat) || m.FromMe != 1 || m.Body != text || m.Type != kind || (m.Reply == nil && replyTo != "") || (m.Reply != nil && m.Reply.MessageID != replyTo) {
				return "", false, func() {}, fmt.Errorf("%w: client message ID already used", ErrInvalidAction)
			}
			return id, true, func() {}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			release()
			return "", false, func() {}, err
		}
		return id, false, release, nil
	}
}
func outgoingExtra(id string) []whatsmeow.SendRequestExtra {
	if id == "" {
		return nil
	}
	return []whatsmeow.SendRequestExtra{{ID: id}}
}
