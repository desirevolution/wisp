package whatsapp

import (
	"context"
	"errors"
	"fmt"
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
	if len(f.patches) != 2 || !f.patches[0].Mutations[0].Value.GetMarkChatAsReadAction().GetRead() {
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

func TestOwnBoundaryUsesFromMeWithoutSendingOwnMessageReceipts(t *testing.T) {
	s := mentionService(t)
	f := &fakeReadTransport{}
	s.readTransport = f
	s.SetReadMode(store.ReadModeSelf)
	sender := "me@s.whatsapp.net"
	s.store.Save(model.Message{ID: "mine", WhatsAppID: "mine", ChatID: "123@g.us", SenderID: &sender, FromMe: 1, Timestamp: 20, Type: "chat"}, "G", "group")
	if _, err := s.store.MarkChatReadThrough("123@g.us", 0, "mine"); err != nil {
		t.Fatal(err)
	}
	s.flushReadSync(context.Background())
	if len(f.calls) != 0 || len(f.patches) != 1 {
		t.Fatal(f.calls, f.patches)
	}
	action := f.patches[0].Mutations[0].Value.GetMarkChatAsReadAction()
	key := action.GetMessageRange().GetMessages()[0].GetKey()
	if !action.GetRead() || !key.GetFromMe() || key.GetID() != "mine" || key.GetRemoteJID() != "123@g.us" || key.GetParticipant() != sender {
		t.Fatal(action)
	}
}
func TestAlreadyReadIncomingStillSynchronizesAndChunksReceipts(t *testing.T) {
	s := mentionService(t)
	f := &fakeReadTransport{}
	messages := []store.ReadSyncMessage{}
	for i := 0; i < 250; i++ {
		messages = append(messages, store.ReadSyncMessage{ID: fmt.Sprint(i), Chat: "123@s.whatsapp.net", Sender: "123@s.whatsapp.net", Timestamp: int64(i + 1)})
	}
	messages[len(messages)-1].Boundary = true
	if err := s.sendReadSync(context.Background(), f, store.ReadSyncJob{Mode: store.ReadModeSelf, Messages: messages}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 || len(f.calls[0].ids) != 100 || len(f.calls[1].ids) != 100 || len(f.calls[2].ids) != 50 {
		t.Fatal(f.calls)
	}
	for _, call := range f.calls {
		if call.kind != types.ReceiptTypeReadSelf {
			t.Fatal("private mode sent normal receipt", call)
		}
	}
}

func TestDeletedLatestMessageCanBoundReadWithoutReceipt(t *testing.T) {
	s := mentionService(t)
	f := &fakeReadTransport{}
	s.readTransport = f
	s.SetReadMode(store.ReadModeSelf)
	s.store.Save(model.Message{ID: "deleted", WhatsAppID: "deleted", ChatID: "123@s.whatsapp.net", Type: "deleted", Timestamp: 20}, "C", "private")
	if _, err := s.store.MarkChatReadThrough("123@s.whatsapp.net", 0, "deleted"); err != nil {
		t.Fatal(err)
	}
	s.flushReadSync(context.Background())
	if len(f.calls) != 0 || len(f.patches) != 1 || f.patches[0].Mutations[0].Value.GetMarkChatAsReadAction().GetMessageRange().GetMessages()[0].GetKey().GetID() != "deleted" {
		t.Fatal(f.calls, f.patches)
	}
}

func TestRetryDoesNotLetNewerBoundaryOvertakeFailedChat(t *testing.T) {
	s := mentionService(t)
	f := &fakeReadTransport{fail: true}
	s.readTransport = f
	s.SetReadMode(store.ReadModeSelf)
	sender := "123@s.whatsapp.net"
	for i, id := range []string{"first", "second"} {
		s.store.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: sender, SenderID: &sender, Timestamp: int64(i + 10), Type: "chat"}, "C", "private")
		if _, err := s.store.MarkChatReadThrough(sender, int64(i+1), id); err != nil {
			t.Fatal(err)
		}
	}
	s.flushReadSync(context.Background())
	if len(f.patches) != 1 {
		t.Fatal("newer boundary overtook failed read", f.patches)
	}
	f.fail = false
	s.flushReadSync(context.Background())
	if len(f.patches) != 3 {
		t.Fatal(f.patches)
	}
	jobs, _ := s.store.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
}
