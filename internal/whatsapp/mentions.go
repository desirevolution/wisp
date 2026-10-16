package whatsapp

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
	"wisp/internal/model"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// Scan URL/email tokens as a whole so IDs inside them are never substituted.
var mentionTokens = regexp.MustCompile(`(?i)https?://[^\s<>]+|www\.[^\s<>]+|[^\s<>@]+@[0-9]+\.[^\s<>]+|@[0-9]+`)

func messageContext(message *waE2E.Message) *waE2E.ContextInfo {
	m, _ := unwrapContent(message)
	if m == nil {
		return nil
	}
	var c *waE2E.ContextInfo
	switch {
	case m.GetExtendedTextMessage() != nil:
		c = m.GetExtendedTextMessage().GetContextInfo()
	case m.GetImageMessage() != nil:
		c = m.GetImageMessage().GetContextInfo()
	case m.GetVideoMessage() != nil:
		c = m.GetVideoMessage().GetContextInfo()
	case m.GetAudioMessage() != nil:
		c = m.GetAudioMessage().GetContextInfo()
	case m.GetDocumentMessage() != nil:
		c = m.GetDocumentMessage().GetContextInfo()
	case m.GetStickerMessage() != nil:
		c = m.GetStickerMessage().GetContextInfo()
	}
	return c
}

func messageMentionJIDs(message *waE2E.Message) []string {
	return append([]string(nil), messageContext(message).GetMentionedJID()...)
}

func mentionWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '@'
}

func replaceMentionTokens(body string, lookup func(string) string) string {
	var out strings.Builder
	last := 0
	for _, loc := range mentionTokens.FindAllStringIndex(body, -1) {
		start, end := loc[0], loc[1]
		token := body[start:end]
		if !strings.HasPrefix(token, "@") {
			continue
		}
		if start > 0 {
			prev, _ := utf8.DecodeLastRuneInString(body[:start])
			if mentionWord(prev) || strings.ContainsRune("/.:+-=", prev) {
				continue
			}
		}
		if end < len(body) {
			next, _ := utf8.DecodeRuneInString(body[end:])
			if mentionWord(next) || next == '/' {
				continue
			}
		}
		name := strings.TrimSpace(lookup(token[1:]))
		if name == "" {
			continue
		}
		out.WriteString(body[last:start])
		out.WriteByte('@')
		out.WriteString(name)
		last = end
	}
	if last == 0 {
		return body
	}
	out.WriteString(body[last:])
	return out.String()
}

func meaningfulMentionName(name string, j types.JID) string {
	name = strings.TrimSpace(name)
	if name == "" || name == j.String() || name == j.User || name == "+"+j.User || name == "@"+j.User {
		return ""
	}
	if strings.HasSuffix(name, "@lid") || strings.HasSuffix(name, "@s.whatsapp.net") || strings.HasSuffix(name, "@c.us") {
		return ""
	}
	return name
}

func (s *Service) mentionName(ctx context.Context, raw string) string {
	j, err := types.ParseJID(raw)
	if err != nil || (j.Server != types.DefaultUserServer && j.Server != types.HiddenUserServer) {
		return ""
	}
	j = j.ToNonAD()
	canonical := s.canonicalJID(ctx, j).ToNonAD()
	for _, candidate := range []types.JID{canonical, j} {
		if name := meaningfulMentionName(s.contactName(ctx, candidate, ""), candidate); name != "" {
			return name
		}
	}
	if s.store != nil {
		for _, candidate := range []types.JID{canonical, j} {
			if name := meaningfulMentionName(s.store.KnownContactName(ctx, candidate.String()), candidate); name != "" {
				return name
			}
		}
	}
	return ""
}

// Use the same contact-first policy for live messages, imports and stored rows.
// Try both PN and original LID, since contacts may be indexed under either ID.
func (s *Service) senderDisplayName(ctx context.Context, j types.JID, fallback string) string {
	j = j.ToNonAD()
	for _, candidate := range []types.JID{s.canonicalJID(ctx, j).ToNonAD(), j} {
		if name := meaningfulMentionName(s.contactName(ctx, candidate, ""), candidate); name != "" {
			return name
		}
	}
	return fallback
}

func parseSenderJID(raw string) types.JID {
	j, _ := types.ParseJID(raw)
	return j
}

// All lookups are local (WhatsApp contacts/LID mappings and Wisp sender names).
// No group-info network request is made while rendering messages.
func (s *Service) mentionBody(ctx context.Context, chat, body string, jids []string, known bool) string {
	return s.mentionBodyCached(ctx, chat, body, jids, known, make(map[string]string))
}
func (s *Service) mentionBodyCached(ctx context.Context, chat, body string, jids []string, known bool, cache map[string]string) string {
	if !known || !strings.HasSuffix(chat, "@g.us") || !strings.Contains(body, "@") {
		return body
	}
	lookup := func(jid string) string {
		if name, ok := cache[jid]; ok {
			return name
		}
		name := s.mentionName(ctx, jid)
		cache[jid] = name
		return name
	}
	explicit := make(map[string][]string)
	for _, raw := range jids {
		j, err := types.ParseJID(raw)
		if err == nil {
			explicit[j.User] = append(explicit[j.User], raw)
		}
	}
	return replaceMentionTokens(body, func(id string) string {
		candidates := explicit[id]
		name := ""
		for _, jid := range candidates {
			resolved := lookup(jid)
			if resolved == "" {
				continue
			}
			if name != "" && name != resolved {
				return ""
			} // Ambiguous mention metadata.
			name = resolved
		}
		return name
	})
}

// Display-only decoration keeps original message text and IDs intact in SQLite.
// field arguments also cover chat-list previews and search result snippets.
func (s *Service) ResolveMentionRows(ctx context.Context, rows []map[string]any, chatField, bodyField, messageField string) {
	if s.store == nil {
		return
	}
	if chatField == "chat_id" {
		for _, row := range rows {
			chat, _ := row["chat_id"].(string)
			id, _ := row["id"].(string)
			s.normalizeReactionActors(ctx, chat, id)
		}
		if err := s.store.EnrichMessageRows(ctx, rows); err != nil {
			fmt.Printf("[context] load: %v\n", err)
		}
	}
	for _, row := range rows {
		if history, ok := row["edit_history"].([]model.MessageVersion); ok {
			chat, _ := row[chatField].(string)
			s.decorateEditHistory(ctx, chat, history)
		}
		if chat, _ := row[chatField].(string); strings.HasSuffix(chat, "@g.us") {
			if sender, ok := row["sender_id"].(string); ok && sender != "" {
				fallback, _ := row["sender_name"].(string)
				row["sender_name"] = s.senderDisplayName(ctx, parseSenderJID(sender), fallback)
			}
		}
		if reactions, ok := row["reactions"].([]model.Reaction); ok {
			s.decorateReactions(ctx, reactions)
		}
		if quote, ok := row["reply"].(*model.Quote); ok {
			quote.SenderName = s.senderDisplayName(ctx, parseSenderJID(quote.SenderID), quote.SenderName)
			if quote.SenderName == "" {
				quote.SenderName = s.mentionName(ctx, quote.SenderID)
			}
		}
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		chat, _ := row[chatField].(string)
		body, _ := row[bodyField].(string)
		id, _ := row[messageField].(string)
		if strings.HasSuffix(chat, "@g.us") && strings.Contains(body, "@") && id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	metadata, err := s.store.MessageMentions(ctx, ids)
	if err != nil {
		fmt.Printf("[mentions] read metadata: %v\n", err)
		return
	}
	cache := make(map[string]string)
	for _, row := range rows {
		chat, _ := row[chatField].(string)
		body, ok := row[bodyField].(string)
		if !ok {
			continue
		}
		id, _ := row[messageField].(string)
		jids, known := metadata[id]
		row[bodyField] = s.mentionBodyCached(ctx, chat, body, jids, known, cache)
	}
}
