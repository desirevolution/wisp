package store

import (
	"context"
	"testing"
	"wisp/internal/model"
)

func TestDeletionRetainsOriginalAndMarksQuotes(t *testing.T) {
	s := contextStore(t)
	original := model.Message{ID: "m", WhatsAppID: "m", ChatID: "g@g.us", Body: "retained", Type: "chat", Timestamp: 5, Raw: []byte{1}}
	s.Save(original, "Group", "group")
	s.Save(model.Message{ID: "r", ChatID: "g@g.us", Type: "chat", Timestamp: 6, Reply: &model.Quote{ChatID: "g@g.us", MessageID: "m", Body: "retained"}}, "Group", "group")
	changed, err := s.SaveDeletion(Deletion{"g@g.us", "m", 10}, "a@lid", false)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	changed, err = s.SaveDeletion(Deletion{"g@g.us", "m", 11}, "a@lid", false)
	if err != nil || changed {
		t.Fatal("duplicate", changed, err)
	}
	got, err := s.MessageByID("m")
	if err != nil || !got.Deleted || got.Body != "retained" || len(got.Raw) != 1 {
		t.Fatal(got, err)
	}
	quote, err := s.MessageByID("r")
	if err != nil || !quote.Reply.Deleted {
		t.Fatal(quote, err)
	}
	var last int64
	s.DB.QueryRow(`SELECT last_message_at FROM chats WHERE id='g@g.us'`).Scan(&last)
	if last != 6 {
		t.Fatal("reordered", last)
	}
	rows, _ := s.Messages("g@g.us", 50, nil, "")
	if err = s.EnrichMessageRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	if rows[0]["deleted"] != true {
		t.Fatal(rows)
	}
}
func TestOriginalArrivingAfterDeletionStaysDeleted(t *testing.T) {
	s := contextStore(t)
	s.SaveDeletion(Deletion{"g@g.us", "m", 10}, "a@lid", false)
	got, err := s.MessageByID("m")
	if err != nil || !got.Deleted || got.Type != "deleted" {
		t.Fatal(got, err)
	}
	_, err = s.Save(model.Message{ID: "m", WhatsAppID: "m", ChatID: "g@g.us", Type: "chat", Body: "later history", Timestamp: 4}, "Group", "group")
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.MessageByID("m")
	if err != nil || !got.Deleted || got.Body != "later history" || got.Timestamp != 4 {
		t.Fatal(got, err)
	}
}
