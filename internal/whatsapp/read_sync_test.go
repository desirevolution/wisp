package whatsapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"wisp/internal/model"
	"wisp/internal/store"
)

type readCall struct {
	ids          []types.MessageID
	chat, sender types.JID
	kind         types.ReceiptType
}
type fakeReadTransport struct {
	calls   []readCall
	patches []appstate.PatchInfo
	fail    bool
}

func (f *fakeReadTransport) MarkRead(_ context.Context, ids []types.MessageID, _ time.Time, chat, sender types.JID, kind ...types.ReceiptType) error {
	f.calls = append(f.calls, readCall{append([]types.MessageID{}, ids...), chat, sender, kind[0]})
	if f.fail {
		return errors.New("offline")
	}
	return nil
}
func (f *fakeReadTransport) SendAppState(_ context.Context, patch appstate.PatchInfo) error {
	f.patches = append(f.patches, patch)
	return nil
}
func TestReadSyncModesRetryAndGroupSenders(t *testing.T) {
	s := mentionService(t)
	f := &fakeReadTransport{}
	s.readTransport = f
	chat := "123@g.us"
	save := func(id, sender string) {
		t.Helper()
		if _, err := s.store.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: chat, SenderID: &sender, Timestamp: 10, Type: "chat"}, "G", "group"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetReadMode(store.ReadModeSelf); err != nil {
		t.Fatal(err)
	}
	save("one", "1@s.whatsapp.net")
	save("two", "2@lid")
	if _, err := s.store.MarkChatReadAndSync(chat, 2); err != nil {
		t.Fatal(err)
	}
	f.fail = true
	s.flushReadSync(context.Background())
	jobs, _ := s.store.ReadSyncJobs()
	if len(jobs) != 1 {
		t.Fatal("failed send lost")
	}
	f.calls = nil
	f.fail = false
	s.flushReadSync(context.Background())
	if len(f.calls) != 2 || f.calls[0].kind != types.ReceiptTypeReadSelf || f.calls[1].kind != types.ReceiptTypeReadSelf || f.calls[0].sender == f.calls[1].sender {
		t.Fatal(f.calls)
	}
	if len(f.patches) != 1 || !f.patches[0].Mutations[0].Value.GetMarkChatAsReadAction().GetRead() {
		t.Fatal(f.patches)
	}
	jobs, _ = s.store.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
	s.SetReadMode(store.ReadModeReceipts)
	save("three", "1@s.whatsapp.net")
	s.store.MarkChatReadAndSync(chat, 3)
	s.flushReadSync(context.Background())
	if f.calls[len(f.calls)-1].kind != types.ReceiptTypeRead {
		t.Fatal(f.calls)
	}
	s.SetReadMode(store.ReadModeLocal)
	save("four", "1@s.whatsapp.net")
	s.store.MarkChatReadAndSync(chat, 4)
	count := len(f.calls)
	s.flushReadSync(context.Background())
	if len(f.calls) != count {
		t.Fatal("local mode sent receipt")
	}
}
func TestOwnDeviceReadsDistinguishedFromPartnerAndNoEcho(t *testing.T) {
	s := mentionService(t)
	chat := types.NewJID("123", types.DefaultUserServer)
	for _, id := range []string{"first", "second", "third"} {
		s.store.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: chat.String(), Timestamp: 10, Type: "chat"}, "C", "private")
	}
	receipt := &events.Receipt{MessageSource: types.MessageSource{Chat: chat, IsFromMe: true}, Type: types.ReceiptTypeReadSelf, MessageIDs: []string{"first"}}
	s.handle(receipt)
	p, _ := s.store.ChatState(chat.String())
	if p.ReadSequence != 0 {
		t.Fatal("local mode imported read", p)
	}
	s.SetReadMode(store.ReadModeSelf)
	receipt.IsFromMe = false
	s.handle(receipt)
	p, _ = s.store.ChatState(chat.String())
	if p.ReadSequence != 0 {
		t.Fatal("partner cleared unread", p)
	}
	receipt.IsFromMe = true
	s.handle(receipt)
	p, _ = s.store.ChatState(chat.String())
	if p.ReadSequence != 1 {
		t.Fatal(p)
	}
	// The boundary is second, even though third arrived in the same second.
	s.handle(&events.MarkChatAsRead{JID: chat, Timestamp: time.Now(), Action: &waSyncAction.MarkChatAsReadAction{Read: proto.Bool(true), MessageRange: &waSyncAction.SyncActionMessageRange{LastMessageTimestamp: proto.Int64(10), Messages: []*waSyncAction.SyncActionMessage{{Key: &waCommon.MessageKey{ID: proto.String("second"), FromMe: proto.Bool(false)}}}}}})
	p, _ = s.store.ChatState(chat.String())
	if p.ReadSequence != 2 || p.FirstUnreadID != "third" {
		t.Fatal(p)
	}
	jobs, _ := s.store.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal("echo loop", jobs)
	}
	receipt.Type = types.ReceiptTypeDelivered
	receipt.MessageIDs = []string{"third"}
	s.handle(receipt)
	p, _ = s.store.ChatState(chat.String())
	if p.ReadSequence != 2 {
		t.Fatal(p)
	}
}
