package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"wisp/internal/model"
)

func TestHistorySyncRetainsLocalChatsAndMessages(t *testing.T) {
	s := mentionService(t)
	s.initialChatLimit = 1
	s.initialMessagesPerChat = 1
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.rootCtx = ctx

	const existing = "1@s.whatsapp.net"
	const archived = "2@s.whatsapp.net"
	for _, msg := range []model.Message{
		{ID: "first", WhatsAppID: "first", ChatID: existing, Timestamp: 1},
		{ID: "second", WhatsAppID: "second", ChatID: existing, Timestamp: 2},
		{ID: "archive", WhatsAppID: "archive", ChatID: archived, Timestamp: 1},
	} {
		msg.Type, msg.Body = "chat", "Stored message"
		if _, err := s.store.Save(msg, "Stored chat", "private"); err != nil {
			t.Fatal(err)
		}
	}

	conversation := func(chat string, messages ...*waHistorySync.HistorySyncMsg) *waHistorySync.Conversation {
		return &waHistorySync.Conversation{ID: proto.String(chat), DisplayName: proto.String("History chat"), Messages: messages}
	}
	message := func(id string, timestamp uint64) *waHistorySync.HistorySyncMsg {
		return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
			Key: &waCommon.MessageKey{ID: proto.String(id)}, MessageTimestamp: proto.Uint64(timestamp),
			Message: &waE2E.Message{Conversation: proto.String(id)},
		}}
	}
	sync := func(kind waHistorySync.HistorySync_HistorySyncType, conversations ...*waHistorySync.Conversation) {
		s.onHistory(&events.HistorySync{Data: &waHistorySync.HistorySync{SyncType: kind.Enum(), Conversations: conversations}})
	}

	const newChat = "3@s.whatsapp.net"
	const limitedChat = "4@s.whatsapp.net"
	sync(waHistorySync.HistorySync_RECENT,
		conversation(limitedChat, message("older-new-chat", 10)),
		conversation(newChat, message("new-chat", 20)),
		conversation(existing, message("imported", 15), message("outside-message-limit", 14)),
	)
	sync(waHistorySync.HistorySync_RECENT, conversation(limitedChat, message("later-batch", 30)))

	for _, id := range []string{"first", "second", "archive", "new-chat", "imported"} {
		if _, err := s.store.MessageByID(id); err != nil {
			t.Errorf("expected message %s: %v", id, err)
		}
	}
	for _, id := range []string{"outside-message-limit", "older-new-chat", "later-batch"} {
		if _, err := s.store.MessageByID(id); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected %s to be excluded, got %v", id, err)
		}
	}

	sync(waHistorySync.HistorySync_ON_DEMAND, conversation(limitedChat, message("requested", 10), message("requested-older", 9)))
	for _, id := range []string{"requested", "requested-older"} {
		if _, err := s.store.MessageByID(id); err != nil {
			t.Errorf("on-demand import of %s: %v", id, err)
		}
	}
}
