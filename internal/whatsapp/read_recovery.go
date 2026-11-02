package whatsapp

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"wisp/internal/store"
)

// Recovery is scoped to the regular_low protocol cache used by read markers.
// FetchAppState rebuilds this verified cache; it does not delete Wisp history.
// BuildAppStateRecoveryRequest asks our own primary device for a snapshot. The
// destructive BuildFatalAppStateExceptionNotification is deliberately not used.
type readStateRecoveryTransport interface {
	FetchAppState(context.Context, appstate.WAPatchName, bool, bool) error
	SendPeerMessage(context.Context, *waE2E.Message) (whatsmeow.SendResponse, error)
}
type readRecoveryState struct {
	pending     bool
	fullTried   bool
	nextAttempt time.Time
	epoch       uint64 // completion can arrive before SendPeerMessage returns
}

func isReadAppStateHashError(err error) bool {
	return err != nil && (errors.Is(err, appstate.ErrMismatchingLTHash) || strings.Contains(err.Error(), "mismatching LTHash"))
}
func (s *Service) resetReadRecovery() {
	s.readRecoveryMu.Lock()
	defer s.readRecoveryMu.Unlock()
	s.readRecovery = readRecoveryState{epoch: s.readRecovery.epoch + 1}
}
func (s *Service) readRecoveryPending() bool {
	s.readRecoveryMu.Lock()
	defer s.readRecoveryMu.Unlock()
	return s.readRecovery.pending
}
func (s *Service) queueReadRecovery(err error, fullTried bool) {
	if !isReadAppStateHashError(err) || s.store.ReadMode() == store.ReadModeLocal {
		return
	}
	s.readRecoveryMu.Lock()
	newlyPending := !s.readRecovery.pending
	s.readRecovery.pending = true
	s.readRecovery.fullTried = s.readRecovery.fullTried || fullTried
	s.readRecoveryMu.Unlock()
	if newlyPending {
		log.Printf("[read-sync] event=recovery_queued collection=regular_low")
		s.WakeReadSync()
	}
}
func (s *Service) onReadAppStateError(e *events.AppStateSyncError) {
	if e != nil && e.Name == appstate.WAPatchRegularLow {
		s.queueReadRecovery(e.Error, e.FullSync)
	}
}
func (s *Service) onReadAppStateComplete(e *events.AppStateSyncComplete) {
	if e == nil || e.Name != appstate.WAPatchRegularLow {
		return
	}
	s.readRecoveryMu.Lock()
	pending := s.readRecovery.pending
	s.readRecovery.pending = false
	s.readRecovery.nextAttempt = time.Time{}
	s.readRecovery.epoch++
	if e.Recovery {
		s.readRecovery.fullTried = false
	}
	s.readRecoveryMu.Unlock()
	if pending {
		log.Printf("[read-sync] event=recovery_complete collection=regular_low version=%d primary_snapshot=%t", e.Version, e.Recovery)
	}
	s.WakeReadSync()
}
func (s *Service) recoveryTransport() readStateRecoveryTransport {
	if s.readTransport != nil {
		transport, _ := s.readTransport.(readStateRecoveryTransport)
		return transport
	}
	if s.client == nil || !s.client.IsConnected() || !s.client.IsLoggedIn() {
		return nil
	}
	return s.client
}

// Called from the existing single read-sync worker. Callbacks only enqueue;
// they never fetch while WhatsMeow holds its internal app-state sync lock.
func (s *Service) processReadRecovery(ctx context.Context) bool {
	if s.store.ReadMode() == store.ReadModeLocal {
		s.resetReadRecovery()
		return true
	}
	s.readRecoveryMu.Lock()
	if !s.readRecovery.pending {
		s.readRecoveryMu.Unlock()
		return true
	}
	if time.Now().Before(s.readRecovery.nextAttempt) {
		s.readRecoveryMu.Unlock()
		return false
	}
	transport := s.recoveryTransport()
	if transport == nil {
		s.readRecoveryMu.Unlock()
		return false
	}
	full := !s.readRecovery.fullTried
	epoch := s.readRecovery.epoch
	s.readRecovery.fullTried = true
	s.readRecoveryMu.Unlock()
	recoveryCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var err error
	if full {
		log.Printf("[read-sync] event=full_resync_started collection=regular_low")
		err = transport.FetchAppState(recoveryCtx, appstate.WAPatchRegularLow, true, false)
	} else {
		log.Printf("[read-sync] event=peer_recovery_started collection=regular_low")
		_, err = transport.SendPeerMessage(recoveryCtx, whatsmeow.BuildAppStateRecoveryRequest(appstate.WAPatchRegularLow))
	}
	s.readRecoveryMu.Lock()
	// A verified snapshot or mode change may have completed during the operation.
	if s.readRecovery.epoch != epoch {
		ready := !s.readRecovery.pending
		s.readRecoveryMu.Unlock()
		return ready
	}
	if full && err == nil {
		s.readRecovery.pending = false
		s.readRecovery.nextAttempt = time.Time{}
		s.readRecoveryMu.Unlock()
		log.Printf("[read-sync] event=full_resync_complete collection=regular_low")
		return true
	}
	s.readRecovery.pending = true
	if full && isReadAppStateHashError(err) {
		// A fresh verified fetch still failed: request the primary's snapshot next.
		s.readRecovery.nextAttempt = time.Time{}
	} else if full {
		// Connectivity/key-fetch errors do not justify escalating to a peer request.
		s.readRecovery.fullTried = false
		s.readRecovery.nextAttempt = time.Now().Add(30 * time.Second)
	} else if err != nil {
		s.readRecovery.nextAttempt = time.Now().Add(time.Minute)
	} else {
		// Keep queued reads until AppStateSyncComplete confirms the response. A
		// primary device may be offline; retry a bounded recovery cycle after 5 min.
		s.readRecovery.fullTried = false
		s.readRecovery.nextAttempt = time.Now().Add(5 * time.Minute)
	}
	s.readRecoveryMu.Unlock()
	if err != nil {
		log.Printf("[read-sync] event=recovery_failed collection=regular_low full_sync=%t error=%q", full, err)
	} else {
		log.Printf("[read-sync] event=peer_recovery_requested collection=regular_low")
	}
	if full && isReadAppStateHashError(err) {
		s.WakeReadSync()
	}
	return false
}

func (s *Service) noteReadStateSuccess() {
	s.readRecoveryMu.Lock()
	defer s.readRecoveryMu.Unlock()
	if !s.readRecovery.pending {
		s.readRecovery.fullTried = false
		s.readRecovery.nextAttempt = time.Time{}
	}
}
