package whatsapp

import (
	"context"
	"fmt"
	"go.mau.fi/whatsmeow/types"
	"sort"
	"strings"
	"time"
	buspkg "wisp/internal/events"
	"wisp/internal/model"
)

// Internal message events keep the UI in sync even when notifications are muted.
// Snapshot the mute decision: queued consumers must not replay a muted message
// merely because its deadline expires before their worker handles it.
func (s *Service) publishLiveMessage(m model.Message) {
	p, changed, err := s.store.LiveChatMessage(m.ChatID)
	if err != nil {
		m.Silent = true
		fmt.Printf("[chat-state] live update: %v\n", err)
	} else {
		m.Silent = p.MutedAt(time.Now().Unix())
		if changed {
			s.bus.Publish(buspkg.Event{Type: "chat-state", Data: p})
		}
	}
	s.bus.Publish(buspkg.Event{Type: "message", Data: m})
}

type ChatOption struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Closed    bool   `json:"closed"`
	MuteUntil int64  `json:"mute_until"`
	Contact   bool   `json:"contact"`
}

func (s *Service) ChatOptions(ctx context.Context, query string) ([]ChatOption, error) {
	rows, err := s.store.Chats()
	if err != nil {
		return nil, err
	}
	states, err := s.store.ChatStates()
	if err != nil {
		return nil, err
	}
	prefs := map[string]ChatOption{}
	for _, p := range states {
		prefs[p.ChatID] = ChatOption{Closed: p.Closed, MuteUntil: p.MuteUntil}
	}
	options := map[string]ChatOption{}
	for _, row := range rows {
		id, _ := row["id"].(string)
		name, _ := row["name"].(string)
		kind, _ := row["type"].(string)
		p := prefs[id]
		p.ID = id
		p.Name = name
		p.Type = kind
		options[id] = p
	}
	if s.client != nil && s.client.Store != nil && s.client.Store.Contacts != nil {
		contacts, e := s.client.Store.Contacts.GetAllContacts(ctx)
		if e != nil {
			return nil, fmt.Errorf("load WhatsApp contacts: %w", e)
		}
		for jid, c := range contacts {
			if jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer {
				continue
			}
			jid = s.canonicalJID(ctx, jid).ToNonAD()
			id := jid.String()
			if _, ok := options[id]; ok {
				continue
			}
			name := c.FullName
			if name == "" {
				name = c.FirstName
			}
			if name == "" {
				name = c.PushName
			}
			if name == "" {
				continue
			}
			options[id] = ChatOption{ID: id, Name: name, Type: "private", Contact: true}
		}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := []ChatOption{}
	for _, o := range options {
		if q == "" || strings.Contains(strings.ToLower(o.Name), q) || strings.Contains(strings.ToLower(o.ID), q) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Contact != out[j].Contact {
			return !out[i].Contact
		}
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a == b {
			return out[i].ID < out[j].ID
		}
		return a < b
	})
	return out, nil
}

// The chooser only opens a known chat or a contact from the linked account.
func (s *Service) OpenChat(ctx context.Context, id string) error {
	id = s.store.ResolveChat(id)
	var exists int
	if err := s.store.DB.QueryRow(`SELECT 1 FROM chats WHERE id=?`, id).Scan(&exists); err == nil {
		return nil
	}
	rows, err := s.ChatOptions(ctx, "")
	if err != nil {
		return err
	}
	for _, o := range rows {
		if o.ID == id {
			return s.store.OpenChat(o.ID, o.Name, o.Type)
		}
	}
	return fmt.Errorf("chat or WhatsApp contact not found")
}
