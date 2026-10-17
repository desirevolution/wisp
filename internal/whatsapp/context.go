package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	buspkg "wisp/internal/events"
	"wisp/internal/model"
)

type messageTransport interface {
	IsConnected() bool
	IsLoggedIn() bool
	SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

func (s *Service) transport() messageTransport {
	if s.sender != nil {
		return s.sender
	}
	if s.client == nil {
		return nil
	}
	return s.client
}
func (s *Service) normalizeReactionActors(ctx context.Context, chat, id string) {
	actors, err := s.store.ReactionActors(ctx, chat, id)
	if err != nil {
		return
	}
	for _, raw := range actors {
		j, err := types.ParseJID(raw)
		if err != nil {
			continue
		}
		canonical := s.canonicalJID(ctx, j).ToNonAD()
		if s.isSelf(ctx, j) {
			canonical = s.selfJID(ctx)
		}
		if !canonical.IsEmpty() && canonical.String() != raw {
			if err := s.store.MergeReactor(chat, id, raw, canonical.String()); err != nil {
				fmt.Printf("[reaction] normalize: %v\n", err)
			}
		}
	}
}

var ErrInvalidAction = errors.New("invalid message action")

func (s *Service) isSelf(ctx context.Context, j types.JID) bool {
	if s.client == nil || s.client.Store == nil {
		return false
	}
	j = s.canonicalJID(ctx, j).ToNonAD()
	if s.client.Store.ID != nil && j == s.client.Store.ID.ToNonAD() {
		return true
	}
	return !j.IsEmpty() && j == s.client.Store.LID.ToNonAD()
}
func (s *Service) selfJID(ctx context.Context) types.JID {
	if s.client == nil || s.client.Store == nil || s.client.Store.ID == nil {
		return types.JID{}
	}
	return s.canonicalJID(ctx, *s.client.Store.ID).ToNonAD()
}
func (s *Service) quote(ctx context.Context, chat string, msg *waE2E.Message) *model.Quote {
	c := messageContext(msg)
	if c.GetStanzaID() == "" {
		return nil
	}
	j, _ := types.ParseJID(c.GetParticipant())
	j = s.canonicalJID(ctx, j).ToNonAD()
	name := s.contactName(ctx, j, "")
	if name == "" && s.store != nil {
		name = s.store.KnownContactName(ctx, j.String())
	}
	kind, body, _ := summary(c.GetQuotedMessage())
	qchat := c.GetRemoteJID()
	if qchat == "" {
		qchat = chat
	} else {
		if jid, err := types.ParseJID(qchat); err == nil {
			qchat = s.canonicalJID(ctx, jid).String()
		}
	}
	body = s.mentionBody(ctx, qchat, body, messageMentionJIDs(c.GetQuotedMessage()), true)
	return &model.Quote{MessageID: c.GetStanzaID(), ChatID: qchat, SenderID: j.String(), SenderName: name, FromMe: s.isSelf(ctx, j), Body: body, Type: kind}
}
func (s *Service) addContext(ctx context.Context, m *model.Message, raw *waE2E.Message, chat, sender string) error {
	var err error
	m.MediaWidth, m.MediaHeight = mediaDimensions(raw)
	m.Raw, err = proto.Marshal(raw)
	if err != nil {
		return err
	}
	m.OriginalChat = chat
	m.OriginalSender = sender
	m.Reply = s.quote(ctx, m.ChatID, raw)
	return nil
}

// Reactions are handled before the visible-message filter. They never publish
// a message event, update chat ordering or trigger push/MQTT.
func (s *Service) onReaction(e *events.Message) bool {
	msg, _ := unwrapContent(e.Message)
	if msg == nil {
		return false
	}
	reaction := msg.GetReactionMessage()
	if msg.GetEncReactionMessage() != nil {
		var err error
		reaction, err = s.client.DecryptReaction(context.Background(), e)
		if err != nil {
			fmt.Printf("[reaction] decrypt failed message=%s: %v\n", e.Info.ID, err)
			return true
		}
	}
	if reaction == nil {
		return false
	}
	if reaction.GetKey().GetID() == "" {
		return true
	}
	ctx := context.Background()
	chat := s.canonicalJID(ctx, e.Info.Chat).String()
	actor := s.canonicalJID(ctx, e.Info.Sender).ToNonAD()
	if e.Info.IsFromMe {
		actor = s.selfJID(ctx)
	}
	name := s.contactName(ctx, actor, e.Info.PushName)
	if name == "" {
		name = s.store.KnownContactName(ctx, actor.String())
	}
	ts := reaction.GetSenderTimestampMS()
	if ts <= 0 {
		ts = e.Info.Timestamp.UnixMilli()
	}
	original := e.RawMessage
	if original == nil {
		original = e.Message
	}
	raw, err := proto.Marshal(original)
	if err != nil {
		fmt.Printf("[reaction] encode: %v\n", err)
		return true
	}
	r := model.Reaction{ChatID: chat, MessageID: reaction.GetKey().GetID(), ActorID: actor.String(), ActorName: name, FromMe: e.Info.IsFromMe, Emoji: reaction.GetText(), TimestampMS: ts, EventID: e.Info.ID, Raw: raw, RawKind: "waE2E.Message"}
	if actor.IsEmpty() {
		return true
	}
	s.normalizeReactionActors(ctx, chat, r.MessageID)
	if changed, err := s.store.SaveReaction(r); err != nil {
		fmt.Printf("[reaction] save: %v\n", err)
	} else if changed {
		s.publishReactions(ctx, chat, r.MessageID)
	}
	return true
}
func (s *Service) decorateReactions(ctx context.Context, rows []model.Reaction) {
	for i := range rows {
		j, err := types.ParseJID(rows[i].ActorID)
		if err != nil {
			continue
		}
		if s.isSelf(ctx, j) {
			rows[i].FromMe = true
		}
		rows[i].ActorName = s.senderDisplayName(ctx, j, rows[i].ActorName)
		if rows[i].ActorName == "" {
			rows[i].ActorName = s.mentionName(ctx, j.String())
		}
	}
}
func (s *Service) ReactionState(ctx context.Context, chat, id string) (*model.ReactionState, error) {
	s.normalizeReactionActors(ctx, chat, id)
	state, err := s.store.ReactionState(ctx, chat, id)
	if err != nil {
		return nil, err
	}
	s.decorateReactions(ctx, state.Reactions)
	return state, nil
}
func (s *Service) publishReactions(ctx context.Context, chat, id string) {
	state, err := s.ReactionState(ctx, chat, id)
	if err != nil {
		fmt.Printf("[reaction] load: %v\n", err)
		return
	}
	s.bus.Publish(buspkg.Event{Type: "reaction", Data: state})
}
func (s *Service) importReactions(ctx context.Context, chat, id string, w *waWeb.WebMessageInfo) {
	changed := false
	for _, reaction := range w.GetReactions() {
		key := reaction.GetKey()
		fromMe := key.GetFromMe()
		actor, _ := types.ParseJID(key.GetParticipant())
		if fromMe {
			actor = s.selfJID(ctx)
		} else if actor.IsEmpty() && !strings.HasSuffix(chat, "@g.us") {
			actor, _ = types.ParseJID(chat)
		}
		actor = s.canonicalJID(ctx, actor).ToNonAD()
		if actor.IsEmpty() {
			continue
		}
		raw, err := proto.Marshal(reaction)
		if err != nil {
			continue
		}
		name := s.contactName(ctx, actor, s.store.KnownContactName(ctx, actor.String()))
		r := model.Reaction{ChatID: chat, MessageID: id, ActorID: actor.String(), ActorName: name, FromMe: fromMe, Emoji: reaction.GetText(), TimestampMS: reaction.GetSenderTimestampMS(), EventID: key.GetID(), Raw: raw, RawKind: "waWeb.Reaction"}
		s.normalizeReactionActors(ctx, chat, id)
		if ok, err := s.store.SaveReaction(r); err != nil {
			fmt.Printf("[reaction] history: %v\n", err)
		} else {
			changed = changed || ok
		}
	}
	if changed {
		s.publishReactions(ctx, chat, id)
	}
}

func (s *Service) actionTarget(ctx context.Context, chat, id string) (*model.Message, types.JID, types.JID, error) {
	m, err := s.store.MessageByID(id)
	if err != nil || m.ChatID != chat {
		return nil, types.JID{}, types.JID{}, fmt.Errorf("%w: original message is not available in this chat", ErrInvalidAction)
	}
	if m.Deleted {
		return nil, types.JID{}, types.JID{}, fmt.Errorf("%w: message was deleted", ErrInvalidAction)
	}
	jid, err := types.ParseJID(m.OriginalChat)
	if err != nil || jid.IsEmpty() {
		jid, err = types.ParseJID(chat)
	}
	if err != nil || (jid.Server != types.GroupServer && jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer) {
		return nil, jid, types.JID{}, fmt.Errorf("%w: unsupported chat", ErrInvalidAction)
	}
	sender, _ := types.ParseJID(m.OriginalSender)
	if m.FromMe != 0 && sender.IsEmpty() {
		sender = s.selfJID(ctx)
	} else if sender.IsEmpty() && m.SenderID != nil {
		sender, _ = types.ParseJID(*m.SenderID)
	}
	if sender.IsEmpty() {
		return nil, jid, sender, fmt.Errorf("%w: original sender is unknown", ErrInvalidAction)
	}
	return m, jid, sender, nil
}
func (s *Service) SendReaction(ctx context.Context, chat, id, emoji string) (*model.ReactionState, error) {
	if !model.ValidReaction(emoji) {
		return nil, fmt.Errorf("%w: emoji is not supported", ErrInvalidAction)
	}
	if s.store.ConfigBool("mirrorOnly", false) {
		return nil, fmt.Errorf("%w: mirror-only mode", ErrInvalidAction)
	}
	if s.transport() == nil || !s.transport().IsConnected() || !s.transport().IsLoggedIn() {
		return nil, fmt.Errorf("whatsapp is not connected")
	}
	chat = s.store.ResolveChat(chat)
	m, jid, sender, err := s.actionTarget(ctx, chat, id)
	if err != nil {
		return nil, err
	}
	// Serialize sends so rapid changes cannot overtake each other locally.
	s.reactionSendMu.Lock()
	defer s.reactionSendMu.Unlock()
	actor := s.selfJID(ctx)
	msg := s.client.BuildReaction(jid, sender, m.WhatsAppID, emoji)
	ts := time.Now().UnixMilli()
	if old := s.store.ReactionTimestamp(chat, id, actor.String()); ts <= old {
		ts = old + 1
	}
	msg.ReactionMessage.SenderTimestampMS = proto.Int64(ts)
	resp, err := s.transport().SendMessage(ctx, jid, msg)
	if err != nil {
		return nil, err
	}
	raw, _ := proto.Marshal(msg)
	r := model.Reaction{ChatID: chat, MessageID: id, ActorID: actor.String(), ActorName: s.contactName(ctx, actor, ""), FromMe: true, Emoji: emoji, TimestampMS: ts, EventID: resp.ID, Raw: raw, RawKind: "waE2E.Message"}
	if _, err = s.store.SaveReaction(r); err != nil {
		return nil, fmt.Errorf("reaction sent but local save failed: %w", err)
	}
	s.publishReactions(ctx, chat, id)
	return s.ReactionState(ctx, chat, id)
}
