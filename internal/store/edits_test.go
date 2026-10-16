package store

import (
	"context"
	"testing"
	"wisp/internal/model"
)

func edit(t *testing.T, s *Store, ts int64, body string) bool {
	t.Helper()
	changed, err := s.SaveEdit(model.MessageEdit{ChatID: "g@g.us", MessageID: "m", TimestampMS: ts, Body: body, Type: "chat", Raw: []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return changed
}
func TestEditsKeepVersionsPositionAndQuoteSnapshot(t *testing.T) {
	s := contextStore(t)
	original := model.Message{ID: "m", WhatsAppID: "m", ChatID: "g@g.us", Type: "chat", Body: "original", Timestamp: 10, Raw: []byte("original raw"), MentionJIDs: []string{"12@lid"}}
	if _, err := s.Save(original, "Group", "group"); err != nil {
		t.Fatal(err)
	}
	s.Save(model.Message{ID: "reply", ChatID: "g@g.us", Type: "chat", Body: "answer", Timestamp: 20, Reply: &model.Quote{ChatID: "g@g.us", MessageID: "m", Body: "original"}}, "Group", "group")
	if !edit(t, s, 40000, "newest") || edit(t, s, 40000, "newest") {
		t.Fatal("deduplication failed")
	}
	edit(t, s, 30000, "middle")
	state, err := s.EditState(context.Background(), "g@g.us", "m")
	if err != nil || state.Body != "newest" || state.Version != 3 || !state.OriginalAvailable || len(state.History) != 2 || state.History[0].Body != "original" || state.History[1].Body != "middle" {
		t.Fatalf("%+v %v", state, err)
	}
	// Replay of old history must not reset the latest body or mention metadata.
	if _, err = s.Save(original, "Group", "group"); err != nil {
		t.Fatal(err)
	}
	m, err := s.MessageByID("m")
	if err != nil || m.Body != "newest" || m.Timestamp != 10 || !m.Edited || string(m.Raw) != "original raw" {
		t.Fatal(m, err)
	}
	q, err := s.MessageByID("reply")
	if err != nil || q.Reply.Body != "original" || !q.Reply.OriginalEdited {
		t.Fatal(q, err)
	}
	var last int64
	s.DB.QueryRow(`SELECT last_message_at FROM chats WHERE id='g@g.us'`).Scan(&last)
	if last != 20 {
		t.Fatal("edit reordered chat", last)
	}
	rows, err := s.Messages("g@g.us", 50, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnrichMessageRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	if rows[0]["edited"] != true || rows[1]["reply"].(*model.Quote).Body != "original" {
		t.Fatal(rows)
	}
}
func TestEditBeforeOriginalAndDeletionRemainIndependent(t *testing.T) {
	s := contextStore(t)
	edit(t, s, 30000, "changed")
	state, err := s.EditState(context.Background(), "g@g.us", "m")
	if err != nil || state.OriginalAvailable || len(state.History) != 0 {
		t.Fatal(state, err)
	}
	_, err = s.Save(model.Message{ID: "m", ChatID: "g@g.us", Type: "image", Body: "caption", HasMedia: 1, Timestamp: 10}, "Group", "group")
	if err != nil {
		t.Fatal(err)
	}
	state, err = s.EditState(context.Background(), "g@g.us", "m")
	if err != nil || !state.OriginalAvailable || state.History[0].Body != "caption" || state.Version != 2 {
		t.Fatal(state, err)
	}
	s.SaveDeletion(Deletion{"g@g.us", "m", 40}, "12@lid", false)
	edit(t, s, 50000, "late edited caption")
	m, err := s.MessageByID("m")
	if err != nil || !m.Deleted || m.Type != "image" || m.HasMedia != 1 || m.Body != "late edited caption" {
		t.Fatal(m, err)
	}
	if _, err = s.DB.Exec(`DELETE FROM messages WHERE id='m'`); err != nil {
		t.Fatal(err)
	}
	state, err = s.EditState(context.Background(), "g@g.us", "m")
	if err != nil || state.Version != 0 {
		t.Fatal("edit history not removed", state, err)
	}
}
