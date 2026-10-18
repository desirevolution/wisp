package store

import (
	"context"
	"path/filepath"
	"testing"
	"wisp/internal/model"
)

func contextStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestContextSnapshotAndRawPayloadSurviveWithoutQuotedOriginal(t *testing.T) {
	s := contextStore(t)
	m := model.Message{ID: "reply", WhatsAppID: "reply", ChatID: "g@g.us", Type: "chat", Body: "answer", Raw: []byte{1, 2, 3}, OriginalChat: "g@g.us", OriginalSender: "1@lid", Reply: &model.Quote{MessageID: "missing", SenderName: "Arthur", Body: "question", Type: "chat"}}
	if _, err := s.Save(m, "Group", "group"); err != nil {
		t.Fatal(err)
	}
	got, err := s.MessageByID(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reply == nil || got.Reply.Body != "question" || string(got.Raw) != string(m.Raw) || got.OriginalSender != "1@lid" {
		t.Fatalf("context lost: %#v", got)
	}
	if _, err = s.DB.Exec(`DELETE FROM messages WHERE id='reply'`); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM message_context`).Scan(&n)
	if n != 0 {
		t.Fatal("raw data retained after message deletion")
	}
}
func TestReactionsAreIdempotentOrderedAndRetainRemoval(t *testing.T) {
	s := contextStore(t)
	ctx := context.Background()
	r := model.Reaction{ChatID: "g@g.us", MessageID: "not-imported", ActorID: "a", Emoji: "🦊", TimestampMS: 100, EventID: "r1", Raw: []byte{1}}
	if changed, err := s.SaveReaction(r); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := s.SaveReaction(r); err != nil || changed {
		t.Fatal("duplicate changed state", changed, err)
	}
	r.Emoji = "👍"
	r.TimestampMS = 200
	r.EventID = "r2"
	s.SaveReaction(r)
	r.Emoji = ""
	r.TimestampMS = 300
	r.EventID = "r3"
	s.SaveReaction(r)
	r.Emoji = "🦊"
	r.TimestampMS = 100
	r.EventID = "old-history"
	s.SaveReaction(r)
	state, err := s.ReactionState(ctx, r.ChatID, r.MessageID)
	if err != nil || len(state.Reactions) != 0 || state.Version != 3 {
		t.Fatalf("removal lost: %#v %v", state, err)
	}
	r.ActorID = "b"
	r.Emoji = "💯"
	r.TimestampMS = 50
	s.SaveReaction(r)
	state, _ = s.ReactionState(ctx, r.ChatID, r.MessageID)
	if len(state.Reactions) != 1 || state.Reactions[0].Emoji != "💯" || state.Version != 4 {
		t.Fatalf("other participant lost: %#v", state)
	}
	// Importing the target later must retain previously received reactions.
	s.Save(model.Message{ID: r.MessageID, WhatsAppID: r.MessageID, ChatID: r.ChatID, Body: "target", Type: "chat"}, "Group", "group")
	rows, _ := s.Messages(r.ChatID, 50, nil, "")
	if err := s.EnrichMessageRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if len(rows[0]["reactions"].([]model.Reaction)) != 1 {
		t.Fatal("pending reaction not attached")
	}
	s.DB.Exec(`DELETE FROM messages WHERE id=?`, r.MessageID)
	state, _ = s.ReactionState(ctx, r.ChatID, r.MessageID)
	if len(state.Reactions) != 0 || state.Version != 0 {
		t.Fatal("reaction data not cleaned")
	}
}
func TestLIDMergeKeepsNewerRemoval(t *testing.T) {
	s := contextStore(t)
	s.SaveReaction(model.Reaction{ChatID: "g", MessageID: "m", ActorID: "123@lid", Emoji: "👍", TimestampMS: 10})
	s.SaveReaction(model.Reaction{ChatID: "g", MessageID: "m", ActorID: "491@s.whatsapp.net", Emoji: "", TimestampMS: 20})
	if err := s.MergeReactor("g", "m", "123@lid", "491@s.whatsapp.net"); err != nil {
		t.Fatal(err)
	}
	state, _ := s.ReactionState(context.Background(), "g", "m")
	if len(state.Reactions) != 0 {
		t.Fatal("identity merge resurrected reaction")
	}
	ids, _ := s.ReactionActors(context.Background(), "g", "m")
	if len(ids) != 1 || ids[0] != "491@s.whatsapp.net" {
		t.Fatal(ids)
	}
}

func TestRejectOldApplicationSchemaWithoutMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`PRAGMA user_version=0`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if reopened, err := Open(path); err == nil {
		reopened.Close()
		t.Fatal("incompatible database was accepted")
	}
}

func TestImageReplyPreviewForExistingSnapshots(t *testing.T) {
	s := contextStore(t)
	photo := model.Message{ID: "photo", WhatsAppID: "photo", ChatID: "g@g.us", Type: "image", HasMedia: 1}
	reply := model.Message{ID: "answer", WhatsAppID: "answer", ChatID: photo.ChatID, Type: "chat", Body: "Nice!", Reply: &model.Quote{MessageID: photo.ID, Type: "image"}}
	for _, m := range []model.Message{photo, reply} {
		if _, err := s.Save(m, "Group", "group"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.MessageByID(reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reply.MediaID != photo.ID {
		t.Fatalf("missing preview: %#v", got.Reply)
	}
	rows, err := s.Messages(photo.ChatID, 50, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnrichMessageRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row["id"] == reply.ID {
			found = true
			if row["reply"].(*model.Quote).MediaID != photo.ID {
				t.Fatal("list preview missing")
			}
		}
	}
	if !found {
		t.Fatal("reply not returned")
	}
	for _, q := range []*model.Quote{
		{MessageID: photo.ID, ChatID: "other@g.us", Type: "image", MediaID: "stale"},
		{MessageID: "missing", Type: "image", MediaID: "stale"},
		{MessageID: photo.ID, Type: "chat", MediaID: "stale"},
	} {
		s.DecorateQuote(q, photo.ChatID)
		if q.MediaID != "" {
			t.Fatalf("invalid preview: %#v", q)
		}
	}
}
