package whatsapp

import (
	"context"
	"fmt"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"time"
	buspkg "wisp/internal/events"
	"wisp/internal/store"
)

func (s *Service) onDeletion(raw *waE2E.Message, chat types.JID, timestamp time.Time) bool {
	m, _ := unwrapContent(raw)
	p := m.GetProtocolMessage()
	if p == nil || p.GetType() != waE2E.ProtocolMessage_REVOKE {
		return false
	}
	key := p.GetKey()
	if key.GetID() == "" {
		return true
	}
	// The authenticated envelope's chat is authoritative, never a different remote JID.
	ctx := context.Background()
	chat = s.canonicalJID(ctx, chat)
	actor, _ := types.ParseJID(key.GetParticipant())
	if key.GetFromMe() {
		actor = s.selfJID(ctx)
	} else if actor.IsEmpty() && chat.Server != types.GroupServer {
		actor = chat
	}
	actor = s.canonicalJID(ctx, actor)
	d := store.Deletion{ChatID: chat.String(), MessageID: key.GetID(), Timestamp: timestamp.Unix()}
	changed, err := s.store.SaveDeletion(d, actor.String(), key.GetFromMe())
	if err != nil {
		fmt.Printf("[deletion] save: %v\n", err)
	} else if changed {
		s.bus.Publish(buspkg.Event{Type: "deletion", Data: d})
	}
	return true
}
