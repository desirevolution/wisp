package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

type recoveringReadTransport struct {
	fakeReadTransport
	badHash               bool
	fetchErr, peerErr     error
	fetches, peers        int
	afterFetch, afterPeer func()
}

func (f *recoveringReadTransport) SendAppState(_ context.Context, patch appstate.PatchInfo) error {
	f.patches = append(f.patches, patch)
	if f.badHash {
		return fmt.Errorf("409 conflict (applying patches failed: %w)", appstate.ErrMismatchingLTHash)
	}
	return nil
}
func (f *recoveringReadTransport) FetchAppState(_ context.Context, name appstate.WAPatchName, full, onlyIfNotSynced bool) error {
	f.fetches++
	if name != appstate.WAPatchRegularLow || !full || onlyIfNotSynced {
		panic("incorrect recovery scope")
	}
	if f.afterFetch != nil {
		f.afterFetch()
	}
	return f.fetchErr
}
func (f *recoveringReadTransport) SendPeerMessage(_ context.Context, m *waE2E.Message) (whatsmeow.SendResponse, error) {
	f.peers++
	p := m.GetProtocolMessage()
	if p.GetType() != waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE || p.GetAppStateFatalExceptionNotification() != nil {
		panic("destructive recovery request")
	}
	if p.GetPeerDataOperationRequestMessage().GetSyncdCollectionFatalRecoveryRequest().GetCollectionName() != "regular_low" {
		panic("incorrect collection")
	}
	if f.afterPeer != nil {
		f.afterPeer()
	}
	return whatsmeow.SendResponse{}, f.peerErr
}
func recoveryService(t *testing.T) (*Service, *recoveringReadTransport) {
	t.Helper()
	s := mentionService(t)
	f := &recoveringReadTransport{badHash: true}
	s.readTransport = f
	if err := s.SetReadMode(store.ReadModeSelf); err != nil {
		t.Fatal(err)
	}
	for i, chat := range []string{"123@s.whatsapp.net", "456@s.whatsapp.net"} {
		id := fmt.Sprintf("incoming-%d", i)
		if _, err := s.store.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: chat, Type: "chat", Timestamp: 10}, "C", "private"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.store.MarkChatReadThrough(chat, 1, id); err != nil {
			t.Fatal(err)
		}
	}
	return s, f
}
func TestReadHashFailureRebuildsCollectionAndRetriesRetainedJobs(t *testing.T) {
	s, f := recoveryService(t)
	s.flushReadSync(context.Background())
	if !s.readRecoveryPending() || len(f.patches) != 1 || f.fetches != 0 {
		t.Fatal("hash failure was not queued or failure storm continued", f)
	}
	jobs, _ := s.store.ReadSyncJobs()
	if len(jobs) != 2 {
		t.Fatal("pending reads lost", jobs)
	}
	// Simulate WhatsMeow dispatching completion before FetchAppState returns.
	f.afterFetch = func() {
		f.badHash = false
		s.handle(&events.AppStateSyncComplete{Name: appstate.WAPatchRegularLow, Version: 364})
	}
	if !s.processReadRecovery(context.Background()) || s.readRecoveryPending() || f.fetches != 1 || f.peers != 0 {
		t.Fatal(f, s.readRecovery)
	}
	s.flushReadSync(context.Background())
	jobs, _ = s.store.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
	if len(f.calls) != 2 {
		t.Fatal(f.calls)
	}
	for _, call := range f.calls {
		if call.kind != "read-self" {
			t.Fatal("recovery changed privacy mode", call)
		}
	}
}
func TestReadHashRecoveryEscalatesToPrimaryAndWaitsForVerifiedSnapshot(t *testing.T) {
	s, f := recoveryService(t)
	s.flushReadSync(context.Background())
	f.fetchErr = appstate.ErrMismatchingLTHash
	// Fetch callbacks must only enqueue, never recursively fetch or send messages.
	f.afterFetch = func() {
		s.handle(&events.AppStateSyncError{Name: appstate.WAPatchRegularLow, FullSync: true, Error: appstate.ErrMismatchingLTHash})
	}
	if s.processReadRecovery(context.Background()) || f.fetches != 1 || f.peers != 0 {
		t.Fatal(f)
	}
	if s.processReadRecovery(context.Background()) || f.peers != 1 {
		t.Fatal(f)
	}
	// A peer delivery ACK is not proof that the primary supplied a usable snapshot.
	if !s.readRecoveryPending() {
		t.Fatal("resumed before snapshot")
	}
	s.processReadRecovery(context.Background())
	s.flushReadSync(context.Background())
	if f.peers != 1 || len(f.patches) != 1 {
		t.Fatal("cooldown ignored", f)
	}
	f.badHash = false
	s.handle(&events.AppStateSyncComplete{Name: appstate.WAPatchRegularLow, Version: 365, Recovery: true})
	if !s.processReadRecovery(context.Background()) {
		t.Fatal("snapshot did not resume reads")
	}
	s.flushReadSync(context.Background())
	jobs, _ := s.store.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
}
func TestReadRecoveryHandlesImmediatePeerResponseAndAlreadyTriedFullSync(t *testing.T) {
	s, f := recoveryService(t)
	s.handle(&events.AppStateSyncError{Name: appstate.WAPatchRegularLow, FullSync: true, Error: appstate.ErrMismatchingLTHash})
	f.afterPeer = func() {
		f.badHash = false
		s.handle(&events.AppStateSyncComplete{Name: appstate.WAPatchRegularLow, Version: 400, Recovery: true})
	}
	if !s.processReadRecovery(context.Background()) || s.readRecoveryPending() || f.fetches != 0 || f.peers != 1 {
		t.Fatal("early response was overwritten", f, s.readRecovery)
	}
}
func TestReadRecoveryIgnoresOtherCollectionsAndDoesNotEscalateNetworkErrors(t *testing.T) {
	s, f := recoveryService(t)
	s.handle(&events.AppStateSyncError{Name: appstate.WAPatchRegular, Error: appstate.ErrMismatchingLTHash})
	s.queueReadRecovery(errors.New("ordinary network timeout"), false)
	if s.readRecoveryPending() {
		t.Fatal("unrelated error queued recovery")
	}
	s.queueReadRecovery(appstate.ErrMismatchingLTHash, false)
	f.fetchErr = errors.New("network timeout")
	if s.processReadRecovery(context.Background()) || f.fetches != 1 || f.peers != 0 {
		t.Fatal(f)
	}
	s.processReadRecovery(context.Background())
	if f.fetches != 1 {
		t.Fatal("network retries unbounded")
	}
	if err := s.SetReadMode(store.ReadModeLocal); err != nil {
		t.Fatal(err)
	}
	s.queueReadRecovery(appstate.ErrMismatchingLTHash, true)
	if s.readRecoveryPending() {
		t.Fatal("local-only mode initiated recovery")
	}
}
func TestReadRecoveryPeerFailureHasCooldownAndCanceledWorkDoesNotReactivate(t *testing.T) {
	s, f := recoveryService(t)
	s.queueReadRecovery(appstate.ErrMismatchingLTHash, true)
	f.peerErr = errors.New("phone unavailable")
	if s.processReadRecovery(context.Background()) || f.peers != 1 {
		t.Fatal(f)
	}
	s.processReadRecovery(context.Background())
	if f.peers != 1 {
		t.Fatal("peer retry spam")
	}
	// A setting change during a network operation must win over its late result.
	s.readRecoveryMu.Lock()
	s.readRecovery.nextAttempt = time.Time{}
	s.readRecoveryMu.Unlock()
	f.afterPeer = func() { s.SetReadMode(store.ReadModeLocal) }
	s.processReadRecovery(context.Background())
	if s.readRecoveryPending() {
		t.Fatal("canceled recovery reactivated")
	}
}

func TestModeChangeCanCancelRecoveryWhileFetchIsInFlight(t *testing.T) {
	s, f := recoveryService(t)
	s.queueReadRecovery(appstate.ErrMismatchingLTHash, false)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	f.fetchErr = appstate.ErrMismatchingLTHash
	f.afterFetch = func() { close(started); <-release }
	go func() { defer close(done); s.processReadRecovery(context.Background()) }()
	<-started
	if err := s.SetReadMode(store.ReadModeLocal); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recovery blocked settings change")
	}
	if s.readRecoveryPending() || f.peers != 0 {
		t.Fatal("canceled fetch escalated", s.readRecovery, f.peers)
	}
}
