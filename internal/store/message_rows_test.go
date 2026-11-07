package store

import (
	"context"
	"fmt"
	"testing"

	"wisp/internal/model"
)

func TestMessageContextUsesStableNeighborsAtSameTimestamp(t *testing.T) {
	s := contextStore(t)
	for i := 0; i < 80; i++ {
		id := fmt.Sprintf("m%02d", i)
		if _, err := s.Save(model.Message{ID: id, WhatsAppID: id, ChatID: "chat", Timestamp: 100}, "Chat", "private"); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.MessageContext("chat", "m60", 3)
	if err != nil || len(rows) != 7 {
		t.Fatalf("context returned %d rows: %v", len(rows), err)
	}
	for i, row := range rows {
		if want := fmt.Sprintf("m%02d", 57+i); row["id"] != want {
			t.Fatalf("row %d: got %v, want %s", i, row["id"], want)
		}
	}
}

func TestSearchContextLimitsMessagesToSelectedChat(t *testing.T) {
	s := contextStore(t)
	for _, id := range []string{"first", "second"} {
		if _, err := s.Save(model.Message{ID: id, WhatsAppID: id, ChatID: id, Type: "chat", Body: "shared phrase", Timestamp: 10}, id, "private"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.SearchContext(context.Background(), "shared", 50, "second")
	if err != nil {
		t.Fatal(err)
	}
	rows := result["messages"].([]map[string]any)
	if len(rows) != 1 || rows[0]["id"] != "second" || len(result["chats"].([]map[string]any)) != 0 {
		t.Fatalf("unexpected scoped search result: %#v", result)
	}
}
