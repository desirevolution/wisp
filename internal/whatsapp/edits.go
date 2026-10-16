package whatsapp

import (
	"context"
	"fmt"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	buspkg "wisp/internal/events"
	"wisp/internal/model"
)

func (s *Service) onEdit(e *events.Message) bool {
	content, _ := unwrapContent(e.Message)
	p := content.GetProtocolMessage()
	// ParseWebMessage normalizes own-device edits to their new text/caption.
	// The target key and edit timestamp remain in the original envelope.
	if p == nil || p.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
		content, _ = unwrapContent(e.RawMessage)
		p = content.GetProtocolMessage()
	}
	if p == nil || p.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
		content, _ = unwrapContent(e.SourceWebMsg.GetMessage())
		p = content.GetProtocolMessage()
	}
	if p == nil || p.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
		if e.IsEdit || e.Info.Edit == types.EditAttributeMessageEdit {
			fmt.Printf("[edit] ignored message=%s reason=missing_protocol from_me=%t\n", e.Info.ID, e.Info.IsFromMe)
			return true
		}
		return false
	}
	key := p.GetKey()
	edited := p.GetEditedMessage()
	if key.GetID() == "" || edited == nil {
		return true
	}
	kind, body, _ := summary(edited)
	switch kind {
	case "chat", "image", "video", "document":
	default:
		return true
	}
	ctx := context.Background()
	chat := s.canonicalJID(ctx, e.Info.Chat)
	// The event envelope supplies the chat. Never update an ID in another chat.
	if remote, err := types.ParseJID(key.GetRemoteJID()); err == nil && !remote.IsEmpty() && s.canonicalJID(ctx, remote) != chat {
		return true
	}
	if target, err := s.store.MessageByID(key.GetID()); err == nil {
		if target.ChatID != chat.String() || (target.Type != "deleted" && target.Type != kind) {
			return true
		}
		if (target.FromMe != 0) != e.Info.IsFromMe {
			return true
		}
		actor := s.canonicalJID(ctx, e.Info.Sender).ToNonAD()
		// WhatsApp authenticates own-device envelopes as IsFromMe. Own authors
		// may use either our PN or LID, even before the mapping is available.
		if !e.Info.IsFromMe && target.SenderID != nil && !actor.IsEmpty() {
			original := s.canonicalJID(ctx, parseSenderJID(*target.SenderID)).ToNonAD()
			if !original.IsEmpty() && actor != original {
				return true
			}
		}
	}
	ts := p.GetTimestampMS()
	if ts <= 0 {
		ts = e.Info.Timestamp.UnixMilli()
	}
	if ts <= 0 {
		return true
	}
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(edited)
	if err != nil {
		return true
	}
	change := model.MessageEdit{ChatID: chat.String(), MessageID: key.GetID(), TimestampMS: ts, Body: body, Type: kind, MentionJIDs: messageMentionJIDs(edited), Raw: raw}
	if changed, err := s.store.SaveEdit(change); err != nil {
		fmt.Printf("[edit] save failed message=%s: %v\n", key.GetID(), err)
	} else if changed {
		fmt.Printf("[edit] applied message=%s from_me=%t\n", key.GetID(), e.Info.IsFromMe)
		s.publishEdit(ctx, chat.String(), key.GetID())
	}
	return true
}
func (s *Service) EditState(ctx context.Context, chat, id string) (*model.EditState, error) {
	state, err := s.store.EditState(ctx, chat, id)
	if err != nil {
		return nil, err
	}
	state.Body = s.mentionBody(ctx, chat, state.Body, state.MentionJIDs, true)
	s.decorateEditHistory(ctx, chat, state.History)
	return state, nil
}
func (s *Service) decorateEditHistory(ctx context.Context, chat string, history []model.MessageVersion) {
	for i := range history {
		history[i].Body = s.mentionBody(ctx, chat, history[i].Body, history[i].MentionJIDs, true)
	}
}
func (s *Service) publishEdit(ctx context.Context, chat, id string) {
	state, err := s.EditState(ctx, chat, id)
	if err != nil {
		fmt.Printf("[edit] load failed: %v\n", err)
		return
	}
	if state.EditedAt > 0 {
		s.bus.Publish(buspkg.Event{Type: "edit", Data: state})
	}
}
