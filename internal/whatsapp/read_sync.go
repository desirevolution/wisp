package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	buspkg "wisp/internal/events"
	"wisp/internal/store"
)

type readSyncTransport interface {
	MarkRead(context.Context, []types.MessageID, time.Time, types.JID, types.JID, ...types.ReceiptType) error
	SendAppState(context.Context, appstate.PatchInfo) error
}

func (s *Service) WakeReadSync() {
	select {
	case s.readWake <- struct{}{}:
	default:
	}
}
func (s *Service) SetReadMode(mode string) error {
	s.readSyncMu.Lock()
	defer s.readSyncMu.Unlock()
	return s.store.SetReadMode(mode)
}
func (s *Service) runReadSync(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.readWake:
		case <-ticker.C:
		}
		s.flushReadSync(ctx)
	}
}
func (s *Service) flushReadSync(ctx context.Context) {
	transport := s.readTransport
	if transport == nil {
		if s.client == nil || !s.client.IsConnected() {
			return
		}
		transport = s.client
	}
	jobs, err := s.store.ReadSyncJobs()
	if err != nil {
		log.Printf("[read-sync] event=load_failed error=%q", err)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		sent, sendErr := s.sendQueuedRead(ctx, transport, job)
		err = sendErr
		if err != nil {
			log.Printf("[read-sync] event=send_failed job=%d mode=%s error=%q", job.ID, job.Mode, err)
			// Retry next tick, but allow other chats to make progress.
			continue
		}
		if err = s.store.DeleteReadSyncJob(job.ID); err != nil {
			log.Printf("[read-sync] event=ack_failed job=%d error=%q", job.ID, err)
			return
		}
		if sent {
			log.Printf("[read-sync] event=sent job=%d mode=%s messages=%d", job.ID, job.Mode, len(job.Messages))
		}
	}
}

// Lock per job so changing the mode can interrupt a large retry backlog.
func (s *Service) sendQueuedRead(ctx context.Context, transport readSyncTransport, job store.ReadSyncJob) (bool, error) {
	s.readSyncMu.Lock()
	defer s.readSyncMu.Unlock()
	if s.store.ReadMode() != job.Mode || job.Mode == store.ReadModeLocal {
		return false, nil
	}
	// A mode change may already have canceled this job, then changed back.
	var exists int
	if err := s.store.DB.QueryRow(`SELECT 1 FROM read_sync_jobs WHERE id=?`, job.ID).Scan(&exists); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	err := s.sendReadSync(sendCtx, transport, job)
	return err == nil, err
}
func (s *Service) sendReadSync(ctx context.Context, transport readSyncTransport, job store.ReadSyncJob) error {
	receiptType := types.ReceiptTypeReadSelf
	if job.Mode == store.ReadModeReceipts {
		receiptType = types.ReceiptTypeRead
	}
	// Batch by original chat and sender, required for group read receipts.
	type group struct {
		chat, sender types.JID
		ids          []types.MessageID
		last         store.ReadSyncMessage
	}
	groups := map[string]*group{}
	order := []string{}
	for _, m := range job.Messages {
		chat, err := types.ParseJID(m.Chat)
		if err != nil {
			return err
		}
		if chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer && chat.Server != types.GroupServer {
			continue
		}
		sender, err := types.ParseJID(m.Sender)
		if m.Sender != "" && err != nil {
			return err
		}
		if chat.Server == types.GroupServer && sender.IsEmpty() {
			return fmt.Errorf("missing group sender")
		}
		key := chat.String() + "/" + sender.String()
		g := groups[key]
		if g == nil {
			g = &group{chat: chat, sender: sender}
			groups[key] = g
			order = append(order, key)
		}
		g.ids = append(g.ids, m.ID)
		if g.last.ID == "" || m.Timestamp >= g.last.Timestamp {
			g.last = m
		}
	}
	now := time.Now()
	for _, key := range order {
		g := groups[key]
		// WhatsMeow honors the account's receipt privacy setting in normal mode.
		if err := transport.MarkRead(ctx, g.ids, now, g.chat, g.sender, receiptType); err != nil {
			return err
		}
	}
	// One whole-chat app-state boundary per address. No later message, including
	// one arriving during a retry, can enter this persisted snapshot.
	latest := map[string]store.ReadSyncMessage{}
	for _, key := range order {
		m := groups[key].last
		if previous, ok := latest[m.Chat]; !ok || m.Timestamp >= previous.Timestamp {
			latest[m.Chat] = m
		}
	}
	for address, m := range latest {
		chat, _ := types.ParseJID(address)
		key := &waCommon.MessageKey{RemoteJID: proto.String(address), FromMe: proto.Bool(false), ID: proto.String(m.ID)}
		if chat.Server == types.GroupServer {
			key.Participant = proto.String(m.Sender)
		}
		if err := transport.SendAppState(ctx, appstate.BuildMarkChatAsRead(chat, true, time.Unix(m.Timestamp, 0), key)); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) publishRemoteRead(chat string, ids []string, before, readAt int64) {
	p, err := s.store.RemoteReadBoundary(chat, ids, before, readAt)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("[read-sync] event=receive_failed error=%q", err)
		return
	}
	s.bus.Publish(buspkg.Event{Type: "unread", Data: p})
}
func (s *Service) onReadReceipt(e *events.Receipt) {
	if e == nil || s.store.ReadMode() == store.ReadModeLocal || !e.IsFromMe {
		return
	}
	if e.Type != types.ReceiptTypeRead && e.Type != types.ReceiptTypeReadSelf {
		return
	}
	// IsFromMe describes the reader of a receipt, not the message author.
	chat := s.canonicalJID(context.Background(), e.Chat).ToNonAD().String()
	s.publishRemoteRead(chat, e.MessageIDs, 0, 0)
}
func (s *Service) onChatRead(e *events.MarkChatAsRead) {
	if e == nil || s.store.ReadMode() == store.ReadModeLocal || !e.Action.GetRead() {
		return
	}
	messageRange := e.Action.GetMessageRange()
	if messageRange == nil {
		return
	}
	ids := []string{}
	for _, m := range messageRange.GetMessages() {
		// An outgoing boundary key must not be mistaken for an incoming message.
		if key := m.GetKey(); key != nil && !key.GetFromMe() && key.GetID() != "" {
			ids = append(ids, key.GetID())
		}
	}
	chat := s.canonicalJID(context.Background(), e.JID).ToNonAD().String()
	s.publishRemoteRead(chat, ids, messageRange.GetLastMessageTimestamp(), e.Timestamp.Unix())
}
