package whatsapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	buspkg "wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

const (
	defaultInitialChatLimit       = 10
	defaultInitialMessagesPerChat = 50
)

type Service struct {
	readSyncMu    sync.Mutex
	readWake      chan struct{}
	readTransport readSyncTransport

	outgoingMu             sync.Mutex
	outgoingPending        map[string]chan struct{}
	sender                 messageTransport
	reactionSendMu         sync.Mutex
	mu                     sync.RWMutex
	status                 model.Status
	client                 *whatsmeow.Client
	container              *sqlstore.Container
	store                  *store.Store
	bus                    *buspkg.Bus
	lastEvent, lastMessage time.Time
	historyMu              sync.Mutex
	historyWait            map[string]chan struct{}
	initialChatLimit       int
	initialMessagesPerChat int
	groupMu                sync.RWMutex
	groupNames             map[string]string
	mediaMu                sync.Mutex
	mediaFetchMu           sync.Mutex
	mediaRecent            map[string]whatsmeow.DownloadableMessage
	mediaRetryPending      map[string]*mediaRetryAttempt
	mediaIO                mediaTransport
	mediaRoot              string
	maxMediaBytes          int64
	rootCtx                context.Context
	initialSyncMu          sync.Mutex
	initialSyncGeneration  uint64
}

type Options struct {
	MediaDir               string
	MaxMediaMB             int
	InitialChatLimit       int
	InitialMessagesPerChat int
}

func New(st *store.Store, b *buspkg.Bus, o Options) *Service {
	if o.MediaDir == "" {
		o.MediaDir = "/data/media"
	}
	if o.MaxMediaMB < 1 {
		o.MaxMediaMB = 100
	}
	if o.InitialChatLimit < 1 {
		o.InitialChatLimit = defaultInitialChatLimit
	}
	if o.InitialMessagesPerChat < 1 {
		o.InitialMessagesPerChat = defaultInitialMessagesPerChat
	}
	return &Service{readWake: make(chan struct{}, 1), status: model.Status{State: "starting"}, store: st, bus: b, historyWait: make(map[string]chan struct{}), groupNames: make(map[string]string), mediaRecent: make(map[string]whatsmeow.DownloadableMessage), mediaRetryPending: make(map[string]*mediaRetryAttempt), mediaRoot: o.MediaDir, maxMediaBytes: int64(o.MaxMediaMB) * 1024 * 1024, initialChatLimit: o.InitialChatLimit, initialMessagesPerChat: o.InitialMessagesPerChat}
}
func (s *Service) Status() model.Status { s.mu.RLock(); defer s.mu.RUnlock(); return s.status }
func (s *Service) set(p model.Status) {
	s.mu.Lock()
	previous := s.status.State
	s.status = p
	s.lastEvent = time.Now()
	s.mu.Unlock()
	if previous != p.State {
		log.Printf("[whatsapp] event=state_changed from=%q to=%q", previous, p.State)
	}
	s.bus.Publish(buspkg.Event{Type: "connection", Data: p})
}
func (s *Service) setSync(syncing bool, phase string) {
	s.mu.Lock()
	s.status.Syncing = syncing
	s.status.SyncPhase = phase
	s.lastEvent = time.Now()
	p := s.status
	s.mu.Unlock()
	s.bus.Publish(buspkg.Event{Type: "connection", Data: p})
}
func (s *Service) Start(ctx context.Context, path string) error {
	s.rootCtx = ctx
	fmt.Printf("[wisp] bootstrap limits chats=%d messages_per_chat=%d\n", s.initialChatLimit, s.initialMessagesPerChat)
	c, e := sqlstore.New(ctx, "sqlite3", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)", waLog.Stdout("WA-DB", "INFO", true))
	if e != nil {
		return e
	}
	d, e := c.GetFirstDevice(ctx)
	if e != nil {
		return e
	}
	cl := whatsmeow.NewClient(d, waLog.Stdout("WA", "INFO", true))
	s.container = c
	s.client = cl
	go s.runReadSync(ctx)
	cl.AddEventHandler(s.handle)
	if cl.Store.ID == nil {
		if e = s.startPairing(ctx); e != nil {
			return e
		}
	} else if e = cl.Connect(); e != nil {
		return e
	}
	return nil
}
func (s *Service) startPairing(ctx context.Context) error {
	s.mu.Lock()
	s.status.InitialBootstrap = true
	s.mu.Unlock()
	qr, err := s.client.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	if err = s.client.ConnectContext(ctx); err != nil {
		return err
	}
	go func() {
		for q := range qr {
			switch q.Event {
			case "code":
				png, _ := qrcode.Encode(q.Code, qrcode.Medium, 320)
				uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
				s.set(model.Status{State: "waiting_for_qr", QR: &uri, InitialBootstrap: true})
			case "success":
				s.set(model.Status{State: "authenticating", InitialBootstrap: true})
			}
		}
	}()
	return nil
}

func (s *Service) Reconnect(ctx context.Context) error {
	if s.client == nil {
		return errors.New("WhatsApp client is not initialized")
	}
	s.set(model.Status{State: "reconnecting"})
	if s.client.IsConnected() {
		s.client.ResetConnection()
		return nil
	}
	if err := s.client.ConnectContext(ctx); err != nil {
		msg := err.Error()
		s.set(model.Status{State: "error", Error: &msg})
		return err
	}
	return nil
}

func (s *Service) LogoutAndPair(ctx context.Context) error {
	if s.client == nil {
		return errors.New("WhatsApp client is not initialized")
	}
	s.readSyncMu.Lock()
	defer s.readSyncMu.Unlock()
	// Never carry pending reads into a newly linked WhatsApp account.
	if err := s.store.CancelReadSync(); err != nil {
		return err
	}
	s.set(model.Status{State: "disconnecting"})
	if err := s.client.Logout(ctx); err != nil {
		msg := err.Error()
		s.set(model.Status{State: "error", Error: &msg})
		return err
	}
	pairCtx := s.rootCtx
	if pairCtx == nil {
		pairCtx = context.Background()
	}
	return s.startPairing(pairCtx)
}

func (s *Service) Close() {
	if s.client != nil {
		s.client.Disconnect()
	}
	if s.container != nil {
		s.container.Close()
	}
}
func (s *Service) handle(raw any) {
	switch e := raw.(type) {
	case *events.Connected:
		var a *string
		if s.client.Store.ID != nil {
			x := s.client.Store.ID.User
			a = &x
		}
		s.mu.RLock()
		initialBootstrap := s.status.InitialBootstrap
		s.mu.RUnlock()
		s.set(model.Status{State: "connected", Account: a, InitialBootstrap: initialBootstrap})
		s.WakeReadSync()
	case *events.Disconnected:
		s.set(model.Status{State: "disconnected"})
	case *events.LoggedOut:
		x := fmt.Sprint(e.Reason)
		s.set(model.Status{State: "error", Error: &x})
	case *events.Receipt:
		s.onReadReceipt(e)
	case *events.MarkChatAsRead:
		s.onChatRead(e)
	case *events.Message:
		s.onMessage(e)
	case *events.HistorySync:
		s.onHistory(e)
	case *events.PushName:
		s.onPushName(e)
	case *events.GroupInfo:
		s.onGroupInfo(e)
	case *events.MediaRetry:
		s.onMediaRetry(e)
	}
}

func (s *Service) onPushName(e *events.PushName) {
	if e == nil || e.NewPushName == "" {
		return
	}
	ctx := context.Background()
	jid := s.canonicalJID(ctx, e.JID)
	_ = s.store.AddAlias(e.JID.String(), jid.String(), jidKind(e.JID))
	if !e.JIDAlt.IsEmpty() {
		_ = s.store.AddAlias(e.JIDAlt.String(), jid.String(), jidKind(e.JIDAlt))
	}
	if err := s.store.UpdateChatName(jid.String(), s.contactName(ctx, jid, e.NewPushName)); err == nil {
		s.bus.Publish(buspkg.Event{Type: "metadata", Data: map[string]any{"reason": "push_name", "chatId": jid.String()}})
	}
}

func (s *Service) onGroupInfo(e *events.GroupInfo) {
	if e == nil || e.Name == nil || e.Name.Name == "" {
		return
	}
	name := e.Name.Name
	key := e.JID.String()
	s.groupMu.Lock()
	s.groupNames[key] = name
	s.groupMu.Unlock()
	if err := s.store.UpdateChatName(key, name); err == nil {
		s.bus.Publish(buspkg.Event{Type: "metadata", Data: map[string]any{"reason": "group_name", "chatId": key}})
	}
}

func (s *Service) canonicalJID(ctx context.Context, j types.JID) types.JID {
	if j.IsEmpty() {
		return j
	}
	if j.Server == types.HiddenUserServer && s.client != nil && s.client.Store != nil && s.client.Store.LIDs != nil {
		if pn, e := s.client.Store.LIDs.GetPNForLID(ctx, j); e == nil && !pn.IsEmpty() {
			return pn
		}
	}
	return j
}
func bestContactName(ci types.ContactInfo, fallback string) string {
	if ci.FullName != "" {
		return ci.FullName
	}
	if ci.BusinessName != "" {
		return ci.BusinessName
	}
	if ci.PushName != "" {
		return ci.PushName
	}
	if ci.FirstName != "" {
		return ci.FirstName
	}
	return fallback
}
func (s *Service) contactName(ctx context.Context, j types.JID, fallback string) string {
	if s.client == nil || s.client.Store == nil || s.client.Store.Contacts == nil {
		return fallback
	}
	if ci, e := s.client.Store.Contacts.GetContact(ctx, j); e == nil && ci.Found {
		return bestContactName(ci, fallback)
	}
	return fallback
}

func (s *Service) groupName(ctx context.Context, j types.JID, fallback string) string {
	key := j.String()
	s.groupMu.RLock()
	name := s.groupNames[key]
	s.groupMu.RUnlock()
	if name != "" {
		return name
	}
	if s.client == nil || j.Server != types.GroupServer {
		return fallback
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := s.client.GetGroupInfo(cctx, j)
	if err != nil || info == nil || info.Name == "" {
		return fallback
	}
	s.groupMu.Lock()
	s.groupNames[key] = info.Name
	s.groupMu.Unlock()
	return info.Name
}

func (s *Service) onMessage(e *events.Message) {
	i := e.Info
	if i.Chat.String() == "status@broadcast" {
		return
	}
	if s.onEdit(e) {
		return
	}
	if s.onDeletion(e.Message, i.Chat, i.Timestamp) {
		return
	}
	if s.onReaction(e) {
		return
	}
	ctx := context.Background()
	originalChat := i.Chat
	chat := s.canonicalJID(ctx, originalChat)
	senderJID := s.canonicalJID(ctx, i.Sender)
	kind, body, media := summary(e.Message)
	if !isVisibleMessage(kind, body, media) {
		return
	}
	sid := senderJID.String()
	var sender *string
	if sid != "" {
		sender = &sid
	}
	sn := s.senderDisplayName(ctx, i.Sender, i.PushName)
	var senderName *string
	if sn != "" {
		senderName = &sn
	}
	m := model.Message{MentionJIDs: messageMentionJIDs(e.Message), ID: i.ID, WhatsAppID: i.ID, ChatID: chat.String(), SenderID: sender, SenderName: senderName, Type: kind, Body: body, HasMedia: media, Timestamp: i.Timestamp.Unix()}
	if i.IsFromMe {
		m.FromMe = 1
	}
	chatType := "private"
	if i.IsGroup {
		chatType = "group"
	}
	chatName := ""
	if i.IsGroup {
		chatName = s.groupName(ctx, chat, "")
	} else {
		chatName = s.contactName(ctx, chat, sn)
	}
	raw := e.RawMessage
	if raw == nil {
		raw = e.Message
	}
	if err := s.addContext(ctx, &m, raw, i.Chat.String(), i.Sender.ToNonAD().String()); err != nil {
		fmt.Printf("[context] encode: %v\n", err)
		return
	}
	m.OriginalInfo, _ = json.Marshal(e.Info)
	if e.SourceWebMsg != nil {
		m.HistoryRaw, _ = proto.Marshal(e.SourceWebMsg)
	}
	inserted, err := s.store.SaveLive(m, chatName, chatType)
	if err != nil {
		fmt.Println("[wisp] save message:", err)
		return
	}
	if !inserted {
		// WhatsApp may redeliver an event with the same message ID. The database
		// already contains it, so do not repeat SSE, push or MQTT side effects.
		return
	}
	var mediaDownload whatsmeow.DownloadableMessage
	if media != 0 {
		if raw, err := proto.Marshal(e.Message); err == nil {
			_ = s.store.SaveMediaPayload(m.ID, raw)
		}
		if dl := downloadable(e.Message); dl != nil {
			s.rememberMedia(m.ID, dl)
			mediaDownload = dl
		} else {
			fmt.Printf("[media] metadata unavailable message=%s\n", m.ID)
			err := fmt.Errorf("media metadata unavailable")
			_ = s.store.MediaFailed(m.ID, err)
		}
	}
	_ = s.store.AddAlias(originalChat.String(), chat.String(), jidKind(originalChat))
	_ = s.store.AddAlias(chat.String(), chat.String(), jidKind(chat))
	s.mu.Lock()
	s.lastMessage = time.Now()
	s.mu.Unlock()

	// Publish the message before starting its media download. A fast local/CDN
	// download can otherwise emit the "media ready" event before the frontend
	// has inserted the corresponding message. The frontend then discards that
	// unmatched media event and leaves the just-arrived message at "loading".
	if err := s.store.DecorateEdit(&m); err != nil {
		fmt.Printf("[edit] decorate: %v\n", err)
	}
	s.decorateEditHistory(ctx, m.ChatID, m.EditHistory)
	m.Body = s.mentionBody(ctx, m.ChatID, m.Body, m.MentionJIDs, true)
	if state, err := s.store.ReactionState(ctx, m.ChatID, m.ID); err == nil {
		m.Reactions = state.Reactions
		m.ReactionVersion = state.Version
	}
	s.store.DecorateDeletion(&m)
	if m.Deleted {
		s.bus.Publish(buspkg.Event{Type: "deletion", Data: map[string]any{"chat_id": m.ChatID, "message_id": m.ID}})
	} else {
		s.publishLiveMessage(m)
	}
	if mediaDownload != nil {
		go s.downloadMedia(m.ID, m.ChatID, mediaDownload)
	}
}

func (s *Service) onHistory(e *events.HistorySync) {
	if e.Data == nil {
		return
	}
	onDemand := e.Data.GetSyncType() == waHistorySync.HistorySync_ON_DEMAND
	if names := e.Data.GetPushnames(); len(names) > 0 {
		s.client.DangerousInternals().HandleHistoricalPushNames(context.Background(), names)
		fmt.Printf("[history-names] applied pushnames=%d\n", len(names))
	}
	if !onDemand {
		s.setSync(true, "importing_history")
	}
	total := 0
	skipped := 0
	for _, conv := range e.Data.GetConversations() {
		if conv == nil {
			continue
		}
		n := s.importConversation(conv, onDemand)
		if n < 0 {
			skipped++
			continue
		}
		total += n
	}
	pruned := 0
	if !onDemand {
		if n, err := s.store.PruneChats(s.initialChatLimit); err != nil {
			fmt.Printf("[wisp] prune bootstrap chats: %v\n", err)
		} else {
			pruned = n
		}
	}
	fmt.Printf("[wisp] history sync type=%s conversations=%d imported=%d skipped=%d pruned_chats=%d\n", e.Data.GetSyncType().String(), len(e.Data.GetConversations()), total, skipped, pruned)
	// One coarse event is enough for the browser to refresh the chat list. Do not
	// emit thousands of message events while importing history.
	s.bus.Publish(buspkg.Event{Type: "metadata", Data: map[string]any{"reason": "history_sync", "imported": total}})
	if !onDemand {
		s.scheduleInitialSyncFinalize()
	}
	if onDemand {
		s.historyMu.Lock()
		for key, ch := range s.historyWait {
			close(ch)
			delete(s.historyWait, key)
		}
		s.historyMu.Unlock()
	}
}

// scheduleInitialSyncFinalize debounces WhatsApp's bootstrap HistorySync batches.
// A fresh pairing commonly delivers several batches seconds apart; marking the
// bootstrap complete after the first batch can expose a temporary hole in the UI.
func (s *Service) scheduleInitialSyncFinalize() {
	s.initialSyncMu.Lock()
	s.initialSyncGeneration++
	generation := s.initialSyncGeneration
	s.initialSyncMu.Unlock()
	s.setSync(true, "settling_history")
	go func() {
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-s.rootCtx.Done():
			return
		}
		s.initialSyncMu.Lock()
		if generation != s.initialSyncGeneration {
			s.initialSyncMu.Unlock()
			return
		}
		s.initialSyncMu.Unlock()
		s.finalizeInitialSync(generation)
	}()
}

func (s *Service) finalizeInitialSync(generation uint64) {
	s.setSync(true, "checking_history")
	chats, err := s.store.Chats()
	if err != nil {
		fmt.Printf("[initial-sync] chat list failed: %v\n", err)
		s.mu.Lock()
		s.status.Syncing = false
		s.status.SyncPhase = ""
		s.status.InitialBootstrap = false
		s.lastEvent = time.Now()
		failedStatus := s.status
		s.mu.Unlock()
		s.bus.Publish(buspkg.Event{Type: "connection", Data: failedStatus})
		return
	}
	if updated, err := s.backfillHistorySenderNames(); err != nil {
		fmt.Printf("[initial-sync] sender-name backfill failed: %v\n", err)
	} else if updated > 0 {
		s.bus.Publish(buspkg.Event{Type: "metadata", Data: map[string]any{"reason": "sender_names", "updated": updated}})
	}

	checked, healed := 0, 0
	// Only bootstrap-retained chats are relevant. Chats() is newest-first and the
	// initial import has already been pruned to initialChatLimit.
	for _, row := range chats {
		id, _ := row["id"].(string)
		if id == "" {
			continue
		}
		checked++
		if s.healInitialGap(id) {
			healed++
		}
	}
	fmt.Printf("[initial-sync] ready generation=%d checked_chats=%d healed_chats=%d\n", generation, checked, healed)
	s.mu.Lock()
	s.status.Syncing = false
	s.status.SyncPhase = ""
	s.status.InitialBootstrap = false
	s.lastEvent = time.Now()
	readyStatus := s.status
	s.mu.Unlock()
	s.bus.Publish(buspkg.Event{Type: "connection", Data: readyStatus})
	s.bus.Publish(buspkg.Event{Type: "metadata", Data: map[string]any{"reason": "initial_sync_ready", "checked_chats": checked, "healed_chats": healed}})
}

func (s *Service) backfillHistorySenderNames() (int, error) {
	ids, err := s.store.MissingSenderNameIDs()
	if err != nil {
		return 0, err
	}
	updated := 0
	ctx := context.Background()
	for _, raw := range ids {
		jid, err := types.ParseJID(raw)
		if err != nil {
			continue
		}
		jid = s.canonicalJID(ctx, jid)
		name := s.contactName(ctx, jid, "")
		if name == "" {
			continue
		}
		n, err := s.store.UpdateSenderName(raw, name)
		if err != nil {
			return updated, err
		}
		updated += n
	}
	return updated, nil
}

// healInitialGap performs the same conservative repair used by explicit older
// history loading, but once after a fresh bootstrap so a restart is not needed.
func (s *Service) healInitialGap(chat string) bool {
	const gapThreshold = int64(7 * 24 * 60 * 60)
	const maxPages = 12
	gap, err := s.store.LargestHistoryGap(chat, gapThreshold)
	if err != nil {
		fmt.Printf("[initial-sync] gap detect failed chat=%s: %v\n", chat, err)
		return false
	}
	if gap == nil {
		return false
	}
	anchorID, anchorTS, lowerTS := gap.UpperID, gap.UpperTS, gap.LowerTS
	for page := 1; page <= maxPages && anchorTS > lowerTS; page++ {
		ctx, cancel := context.WithTimeout(s.rootCtx, 15*time.Second)
		err = s.FetchOlder(ctx, chat, anchorID, anchorTS, 50)
		cancel()
		if err != nil {
			fmt.Printf("[initial-sync] gap fetch failed chat=%s page=%d: %v\n", chat, page, err)
			break
		}
		pageBefore := anchorTS
		rows, queryErr := s.store.Messages(chat, 200, &pageBefore, anchorID)
		if queryErr != nil {
			break
		}
		nextID := ""
		var nextTS int64
		for _, row := range rows {
			rid, okID := row["id"].(string)
			rts, okTS := row["timestamp"].(int64)
			if !okID || !okTS || rts <= lowerTS || rts >= anchorTS {
				continue
			}
			if nextID == "" || rts < nextTS || (rts == nextTS && rid < nextID) {
				nextID, nextTS = rid, rts
			}
		}
		if nextID == "" {
			break
		}
		anchorID, anchorTS = nextID, nextTS
	}
	remaining, _ := s.store.LargestHistoryGap(chat, gapThreshold)
	closed := remaining == nil
	return closed
}

type normalizedHistoryMessage struct {
	Message model.Message
	Raw     *waE2E.Message
	Kind    string
	Wrapper string
}

func (s *Service) normalizeHistoryMessage(ctx context.Context, w *waWeb.WebMessageInfo, chatJID types.JID, canonical string) (*normalizedHistoryMessage, string) {
	if w == nil || w.GetKey() == nil {
		return nil, "missing_message_or_key"
	}
	key := w.GetKey()
	id := key.GetID()
	if id == "" {
		return nil, "missing_id"
	}

	senderRaw := key.GetParticipant()
	if senderRaw == "" {
		senderRaw = w.GetParticipant()
	}
	if senderRaw == "" && key.GetFromMe() && s.client.Store.ID != nil {
		senderRaw = s.client.Store.ID.String()
	}
	if senderRaw == "" && chatJID.Server != types.GroupServer {
		senderRaw = canonical
	}

	senderJID, _ := types.ParseJID(senderRaw)
	senderJID = s.canonicalJID(ctx, senderJID)
	sid := senderJID.String()
	var sender *string
	if sid != "" {
		sender = &sid
	}

	pushName := w.GetPushName()
	sn := s.senderDisplayName(ctx, parseSenderJID(senderRaw), pushName)
	var senderName *string
	if sn != "" {
		senderName = &sn
	}

	content, wrapper := unwrapContent(w.GetMessage())
	kind, body, media := summary(content)
	if !isVisibleMessage(kind, body, media) {
		return nil, fmt.Sprintf("non_visible:%s:%s", wrapper, kind)
	}
	ts := int64(w.GetMessageTimestamp())
	if ts == 0 {
		return nil, "missing_timestamp"
	}

	m := model.Message{
		MentionJIDs: messageMentionJIDs(content),
		ID:          id, WhatsAppID: id, ChatID: canonical,
		SenderID: sender, SenderName: senderName,
		Type: kind, Body: body, HasMedia: media, Timestamp: ts,
	}
	if key.GetFromMe() {
		m.FromMe = 1
	}
	if err := s.addContext(ctx, &m, w.GetMessage(), chatJID.String(), senderRaw); err != nil {
		return nil, "invalid_payload"
	}
	m.HistoryRaw, _ = proto.Marshal(w)
	return &normalizedHistoryMessage{Message: m, Raw: w.GetMessage(), Kind: kind, Wrapper: wrapper}, ""
}

func (s *Service) importConversation(conv *waHistorySync.Conversation, onDemand bool) int {
	rawID := conv.GetID()
	if shouldIgnoreHistoryChat(rawID) {
		return -1
	}
	canonical := rawID
	if conv.GetPnJID() != "" {
		canonical = conv.GetPnJID()
	}
	chatJID, err := types.ParseJID(canonical)
	if err != nil {
		return 0
	}
	rawJID, _ := types.ParseJID(rawID)
	if chatJID.Server == types.HiddenUserServer {
		chatJID = s.canonicalJID(context.Background(), chatJID)
		canonical = chatJID.String()
	}
	name := conv.GetDisplayName()
	if name == "" {
		name = conv.GetName()
	}
	if name == "" && chatJID.Server != types.GroupServer {
		name = s.contactName(context.Background(), chatJID, "")
	}
	if chatJID.Server == types.GroupServer {
		if name == "" {
			name = s.groupName(context.Background(), chatJID, "")
		}
		if name != "" {
			s.groupMu.Lock()
			s.groupNames[chatJID.String()] = name
			s.groupMu.Unlock()
		}
	}
	chatType := "private"
	if chatJID.Server == types.GroupServer {
		chatType = "group"
	}
	// Make conversation aliases available before normalizing any participant.
	_ = s.store.AddAlias(rawID, canonical, jidKind(rawJID))
	if conv.GetLidJID() != "" {
		_ = s.store.AddAlias(conv.GetLidJID(), canonical, "lid")
	}
	if conv.GetPnJID() != "" {
		_ = s.store.AddAlias(conv.GetPnJID(), canonical, "pn")
	}
	_ = s.store.AddAlias(canonical, canonical, jidKind(chatJID))
	msgs := append([]*waHistorySync.HistorySyncMsg(nil), conv.GetMessages()...)
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].GetMessage().GetMessageTimestamp() > msgs[j].GetMessage().GetMessageTimestamp()
	})
	if !onDemand && len(msgs) > s.initialMessagesPerChat {
		msgs = msgs[:s.initialMessagesPerChat]
	}
	imported := 0
	for _, hm := range msgs {
		w := hm.GetMessage()
		if w == nil || w.GetKey() == nil {
			continue
		}
		key := w.GetKey()
		actorRaw := key.GetParticipant()
		if actorRaw == "" {
			actorRaw = w.GetParticipant()
		}
		actor, _ := types.ParseJID(actorRaw)
		if actor.IsEmpty() && key.GetFromMe() {
			actor = s.selfJID(context.Background())
		}
		if actor.IsEmpty() && chatJID.Server != types.GroupServer {
			actor = chatJID
		}
		if s.onEdit(&events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chatJID, Sender: actor, IsFromMe: key.GetFromMe(), IsGroup: chatJID.Server == types.GroupServer}, Timestamp: time.Unix(int64(w.GetMessageTimestamp()), 0)}, Message: w.GetMessage()}) {
			continue
		}
		id := key.GetID()
		if id == "" {
			continue
		}
		if w.GetMessageStubType() == waWeb.WebMessageInfo_REVOKE || w.GetMessageStubType() == waWeb.WebMessageInfo_ADMIN_REVOKE {
			s.onDeletion(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: key}}, chatJID, time.Unix(int64(w.GetMessageTimestamp()), 0))
			continue
		}
		if s.onDeletion(w.GetMessage(), chatJID, time.Unix(int64(w.GetMessageTimestamp()), 0)) {
			continue
		}
		content, _ := unwrapContent(w.GetMessage())
		if content.GetReactionMessage() != nil || content.GetEncReactionMessage() != nil {
			actorRaw := key.GetParticipant()
			if actorRaw == "" {
				actorRaw = w.GetParticipant()
			}
			actor, _ := types.ParseJID(actorRaw)
			if actor.IsEmpty() && chatJID.Server != types.GroupServer {
				actor = chatJID
			}
			s.onReaction(&events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chatJID, Sender: actor, IsFromMe: key.GetFromMe(), IsGroup: chatJID.Server == types.GroupServer}, ID: key.GetID(), Timestamp: time.Unix(int64(w.GetMessageTimestamp()), 0), PushName: w.GetPushName()}, Message: content, RawMessage: w.GetMessage(), SourceWebMsg: w})
			continue
		}
		norm, _ := s.normalizeHistoryMessage(context.Background(), w, chatJID, canonical)
		if norm == nil {
			continue
		}
		m := norm.Message
		if _, err := s.store.Save(m, name, chatType); err == nil {
			imported++
			s.publishEdit(context.Background(), m.ChatID, m.ID)
			s.importReactions(context.Background(), m.ChatID, m.ID, w)
			if m.HasMedia != 0 {
				raw, marshalErr := proto.Marshal(norm.Raw)
				if marshalErr != nil {
					fmt.Printf("[history-media] descriptor marshal failed message=%s chat=%s type=%s: %v\n", m.ID, canonical, norm.Kind, marshalErr)
				} else if err := s.store.SaveMediaPayload(m.ID, raw); err != nil {
					fmt.Printf("[history-media] descriptor store failed message=%s chat=%s type=%s bytes=%d: %v\n", m.ID, canonical, norm.Kind, len(raw), err)
				}
			}
		} else {
			fmt.Printf("[wisp] history save failed message=%s chat=%s: %v\n", m.ID, canonical, err)
		}
	}
	_ = s.store.AddAlias(rawID, canonical, jidKind(rawJID))
	if conv.GetLidJID() != "" {
		_ = s.store.AddAlias(conv.GetLidJID(), canonical, "lid")
	}
	if conv.GetPnJID() != "" {
		_ = s.store.AddAlias(conv.GetPnJID(), canonical, "pn")
	}
	_ = s.store.AddAlias(canonical, canonical, jidKind(chatJID))
	if !onDemand {
		_ = s.store.TrimChat(canonical, s.initialMessagesPerChat)
	}
	// Paging anchors come from the concrete API beforeId/beforeTs boundary.
	// Bootstrap and ON_DEMAND imports never move an independent cursor.
	if onDemand {
		switch conv.GetEndOfHistoryTransferType() {
		case waHistorySync.Conversation_COMPLETE_AND_NO_MORE_MESSAGE_REMAIN_ON_PRIMARY:
			s.store.SetHistoryExhausted(canonical, true)
		default:
			// Values 0, 2 and 3 are not equivalent to "no more messages".
			s.store.SetHistoryExhausted(canonical, false)
		}
	}
	return imported
}
func shouldIgnoreHistoryChat(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || id == "status@broadcast" {
		return true
	}
	// Wisp mirrors user/group conversations. WhatsApp status, broadcast lists and
	// channels/newsletters are not useful chat-list entries for this app.
	return strings.HasSuffix(id, "@broadcast") || strings.HasSuffix(id, "@newsletter")
}

func jidKind(j types.JID) string {
	if j.Server == types.HiddenUserServer {
		return "lid"
	}
	if j.Server == types.GroupServer {
		return "group"
	}
	return "pn"
}

// FetchOlder asks the primary device for the next history page and waits briefly
// for the ON_DEMAND HistorySync event to be persisted.
func (s *Service) FetchOlder(ctx context.Context, chat, beforeID string, beforeTS int64, count int) error {
	chat = s.store.ResolveChat(chat)
	if s.store.HistoryExhausted(chat) {
		return nil
	}

	// The concrete API page boundary is authoritative. Looking up an independent
	// persisted cursor here can select a newer bootstrap message and skip the gap
	// the browser is explicitly trying to fill.
	anchor, e := s.store.MessageByID(beforeID)
	if e != nil {
		return fmt.Errorf("history anchor %s not found: %w", beforeID, e)
	}
	if anchor.ChatID != chat {
		return fmt.Errorf("history anchor %s belongs to %s, not %s", beforeID, anchor.ChatID, chat)
	}
	cj, e := types.ParseJID(chat)
	if e != nil {
		return e
	}
	sj := cj
	if anchor.SenderID != nil {
		if x, err := types.ParseJID(*anchor.SenderID); err == nil {
			sj = x
		}
	}
	info := &types.MessageInfo{MessageSource: types.MessageSource{Chat: cj, Sender: sj, IsFromMe: anchor.FromMe != 0, IsGroup: cj.Server == types.GroupServer}, ID: anchor.ID, Timestamp: time.Unix(anchor.Timestamp, 0)}
	req := s.client.BuildHistorySyncRequest(info, count)
	wait := make(chan struct{})
	s.historyMu.Lock()
	s.historyWait[chat] = wait
	s.historyMu.Unlock()
	if _, e = s.client.SendPeerMessage(ctx, req); e != nil {
		s.historyMu.Lock()
		delete(s.historyWait, chat)
		s.historyMu.Unlock()
		return e
	}
	select {
	case <-wait:
		return nil
	case <-ctx.Done():
		s.historyMu.Lock()
		delete(s.historyWait, chat)
		s.historyMu.Unlock()
		return ctx.Err()
	}
}

func unwrapContent(m *waE2E.Message) (*waE2E.Message, string) {
	if m == nil {
		return nil, "nil"
	}
	seen := map[*waE2E.Message]bool{}
	path := []string{}
	for m != nil && !seen[m] {
		seen[m] = true
		switch {
		case m.GetDeviceSentMessage() != nil && m.GetDeviceSentMessage().GetMessage() != nil:
			path = append(path, "device_sent")
			m = m.GetDeviceSentMessage().GetMessage()
		case m.GetEphemeralMessage() != nil && m.GetEphemeralMessage().GetMessage() != nil:
			path = append(path, "ephemeral")
			m = m.GetEphemeralMessage().GetMessage()
		case m.GetViewOnceMessage() != nil && m.GetViewOnceMessage().GetMessage() != nil:
			path = append(path, "view_once")
			m = m.GetViewOnceMessage().GetMessage()
		case m.GetViewOnceMessageV2() != nil && m.GetViewOnceMessageV2().GetMessage() != nil:
			path = append(path, "view_once_v2")
			m = m.GetViewOnceMessageV2().GetMessage()
		case m.GetDocumentWithCaptionMessage() != nil && m.GetDocumentWithCaptionMessage().GetMessage() != nil:
			path = append(path, "document_with_caption")
			m = m.GetDocumentWithCaptionMessage().GetMessage()
		case m.GetEditedMessage() != nil && m.GetEditedMessage().GetMessage() != nil:
			path = append(path, "edited")
			m = m.GetEditedMessage().GetMessage()
		default:
			if len(path) == 0 {
				return m, "direct"
			}
			return m, strings.Join(path, ">")
		}
	}
	return m, strings.Join(path, ">")
}

func summary(m *waE2E.Message) (string, string, int) {
	m, _ = unwrapContent(m)
	if m == nil {
		return "other", "", 0
	}
	switch {
	case m.GetConversation() != "":
		return "chat", m.GetConversation(), 0
	case m.GetExtendedTextMessage() != nil:
		return "chat", m.GetExtendedTextMessage().GetText(), 0
	case m.GetImageMessage() != nil:
		return "image", m.GetImageMessage().GetCaption(), 1
	case m.GetVideoMessage() != nil:
		return "video", m.GetVideoMessage().GetCaption(), 1
	case m.GetAudioMessage() != nil:
		if m.GetAudioMessage().GetPTT() {
			return "ptt", "", 1
		}
		return "audio", "", 1
	case m.GetDocumentMessage() != nil:
		if caption := m.GetDocumentMessage().GetCaption(); caption != "" {
			return "document", caption, 1
		}
		return "document", m.GetDocumentMessage().GetFileName(), 1
	case m.GetStickerMessage() != nil:
		return "sticker", "", 1
	}
	return "other", "", 0
}

// isVisibleMessage is the timeline boundary. WhatsApp also transports protocol,
// sync and control messages through message-shaped events. Those must never
// become empty chat bubbles. Unknown messages are kept out of the timeline
// until they have an explicit normalizer.
func isVisibleMessage(kind, body string, media int) bool {
	if media != 0 {
		return true
	}
	return kind == "chat" && strings.TrimSpace(body) != ""
}

func (s *Service) SendText(ctx context.Context, chat, text, replyTo string, clientIDs ...string) (string, error) {
	if s.store.ConfigBool("mirrorOnly", false) {
		return "", fmt.Errorf("%w: mirror-only mode", ErrInvalidAction)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("message is empty")
	}
	if len([]rune(text)) > 4000 {
		return "", fmt.Errorf("message is too long")
	}
	id, already, release, err := s.beginOutgoing(ctx, chat, text, "chat", replyTo, clientIDs)
	if err != nil {
		return "", err
	}
	defer release()
	if already {
		return id, nil
	}
	if s.transport() == nil || !s.transport().IsConnected() || !s.transport().IsLoggedIn() {
		return "", fmt.Errorf("whatsapp is not connected")
	}
	chat = s.store.ResolveChat(chat)
	jid, err := types.ParseJID(chat)
	if err != nil {
		return "", fmt.Errorf("invalid chat: %w", err)
	}
	outgoing := &waE2E.Message{Conversation: proto.String(text)}
	if replyTo != "" {
		target, originalChat, sender, e := s.actionTarget(ctx, chat, replyTo)
		if e != nil {
			return "", e
		}
		var original waE2E.Message
		quotedRaw := target.Raw
		if len(target.CurrentRaw) > 0 {
			quotedRaw = target.CurrentRaw
		}
		if len(quotedRaw) == 0 || proto.Unmarshal(quotedRaw, &original) != nil {
			return "", fmt.Errorf("%w: original content is not available", ErrInvalidAction)
		}
		content, _ := unwrapContent(&original)
		outgoing = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text), ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String(target.WhatsAppID), Participant: proto.String(sender.ToNonAD().String()), RemoteJID: proto.String(originalChat.String()), QuotedMessage: content}}}
		jid = originalChat
	}
	resp, err := s.transport().SendMessage(ctx, jid, outgoing, outgoingExtra(id)...)
	if err != nil {
		return "", err
	}
	// Some WhatsApp paths (notably send-to-self) don't echo a normal Message
	// event back to this linked device. Persist the acknowledged send immediately.
	// Save() is idempotent on the WhatsApp message ID, so a later echo is deduped.
	chatName := s.contactName(ctx, jid, "")
	chatType := "private"
	if jid.Server == types.GroupServer {
		chatType = "group"
		chatName = s.groupName(ctx, jid, chatName)
	}
	self := ""
	if s.client.Store.ID != nil {
		self = s.canonicalJID(ctx, *s.client.Store.ID).String()
	}
	var sender *string
	if self != "" {
		sender = &self
	}
	ts := resp.Timestamp.Unix()
	if ts == 0 {
		ts = time.Now().Unix()
	}
	m := model.Message{ID: string(resp.ID), WhatsAppID: string(resp.ID), ChatID: chat, SenderID: sender, FromMe: 1, Type: "chat", Body: text, Timestamp: ts}
	if err := s.addContext(ctx, &m, outgoing, jid.String(), self); err != nil {
		return string(resp.ID), nil
	}
	inserted, err := s.store.Save(m, chatName, chatType)
	if err != nil {
		return string(resp.ID), nil
	}
	if inserted {
		m.Body = s.mentionBody(ctx, m.ChatID, m.Body, m.MentionJIDs, true)
		s.publishLiveMessage(m)
	}
	return string(resp.ID), nil
}

func (s *Service) Health() (string, *time.Time, *time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var e, m *time.Time
	if !s.lastEvent.IsZero() {
		x := s.lastEvent
		e = &x
	}
	if !s.lastMessage.IsZero() {
		x := s.lastMessage
		m = &x
	}
	return s.status.State, e, m
}

func downloadable(m *waE2E.Message) whatsmeow.DownloadableMessage {
	m, _ = unwrapContent(m)
	if m == nil {
		return nil
	}
	if x := m.GetImageMessage(); x != nil {
		return x
	}
	if x := m.GetVideoMessage(); x != nil {
		return x
	}
	if x := m.GetAudioMessage(); x != nil {
		return x
	}
	if x := m.GetDocumentMessage(); x != nil {
		return x
	}
	if x := m.GetStickerMessage(); x != nil {
		return x
	}
	return nil
}
func (s *Service) rememberMedia(id string, dl whatsmeow.DownloadableMessage) {
	s.mediaMu.Lock()
	s.mediaRecent[id] = dl
	s.mediaMu.Unlock()
	time.AfterFunc(15*time.Minute, func() { s.mediaMu.Lock(); delete(s.mediaRecent, id); s.mediaMu.Unlock() })
}
func safeExt(mimeType, filename string) string {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" && len(ext) <= 10 {
		return ext
	}
	switch strings.ToLower(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "application/pdf":
		return ".pdf"
	}
	return ""
}
func mediaMeta(m whatsmeow.DownloadableMessage) (string, string) {
	switch x := m.(type) {
	case *waE2E.ImageMessage:
		return x.GetMimetype(), ""
	case *waE2E.VideoMessage:
		return x.GetMimetype(), ""
	case *waE2E.AudioMessage:
		return x.GetMimetype(), ""
	case *waE2E.DocumentMessage:
		return x.GetMimetype(), x.GetFileName()
	case *waE2E.StickerMessage:
		return x.GetMimetype(), ""
	}
	return "application/octet-stream", ""
}
func mediaKey(dl whatsmeow.DownloadableMessage) []byte {
	switch m := dl.(type) {
	case *waE2E.ImageMessage:
		return m.GetMediaKey()
	case *waE2E.VideoMessage:
		return m.GetMediaKey()
	case *waE2E.AudioMessage:
		return m.GetMediaKey()
	case *waE2E.DocumentMessage:
		return m.GetMediaKey()
	case *waE2E.StickerMessage:
		return m.GetMediaKey()
	default:
		return nil
	}
}
func mediaDirectPath(dl whatsmeow.DownloadableMessage) string {
	switch m := dl.(type) {
	case *waE2E.ImageMessage:
		return m.GetDirectPath()
	case *waE2E.VideoMessage:
		return m.GetDirectPath()
	case *waE2E.AudioMessage:
		return m.GetDirectPath()
	case *waE2E.DocumentMessage:
		return m.GetDirectPath()
	case *waE2E.StickerMessage:
		return m.GetDirectPath()
	default:
		return ""
	}
}

func setDirectPath(dl whatsmeow.DownloadableMessage, path string) bool {
	switch m := dl.(type) {
	case *waE2E.ImageMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.VideoMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.AudioMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.DocumentMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.StickerMessage:
		m.DirectPath = proto.String(path)
	default:
		return false
	}
	return true
}
func downloadableToMessage(dl whatsmeow.DownloadableMessage) *waE2E.Message {
	switch m := dl.(type) {
	case *waE2E.ImageMessage:
		return &waE2E.Message{ImageMessage: m}
	case *waE2E.VideoMessage:
		return &waE2E.Message{VideoMessage: m}
	case *waE2E.AudioMessage:
		return &waE2E.Message{AudioMessage: m}
	case *waE2E.DocumentMessage:
		return &waE2E.Message{DocumentMessage: m}
	case *waE2E.StickerMessage:
		return &waE2E.Message{StickerMessage: m}
	default:
		return nil
	}
}

func (s *Service) downloadMedia(messageID, chatID string, dl whatsmeow.DownloadableMessage) {
	s.downloadMediaAttempt(messageID, chatID, dl, true)
}
func (s *Service) downloadMediaAttempt(messageID, chatID string, dl whatsmeow.DownloadableMessage, allowRetry bool) {
	mimeType, filename := mediaMeta(dl)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := s.mediaTransport().Download(ctx, dl)
	if err != nil {
		fmt.Printf("[media] download failed message=%s: %v\n", messageID, err)
		if errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith403) ||
			errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith404) ||
			errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410) ||
			errors.Is(err, whatsmeow.ErrInvalidMediaEncSHA256) ||
			errors.Is(err, whatsmeow.ErrInvalidMediaSHA256) ||
			errors.Is(err, whatsmeow.ErrInvalidMediaHMAC) {
			if allowRetry && s.requestMediaRetry(messageID, dl) {
				return
			}
		}
		if !allowRetry {
			fmt.Printf("[media-retry] final_failure message=%s stage=refreshed_download automatic_retry=false key_bytes=%d enc_hash_bytes=%d plain_hash_bytes=%d\n", messageID, len(dl.GetMediaKey()), len(dl.GetFileEncSHA256()), len(dl.GetFileSHA256()))
		}
		_ = s.store.MediaFailed(messageID, err)
		s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": messageID, "chatId": chatID, "status": "failed"}})
		return
	}
	if int64(len(data)) > s.maxMediaBytes {
		err = fmt.Errorf("media exceeds MAX_MEDIA_MB")
		_ = s.store.MediaFailed(messageID, err)
		fmt.Printf("[media] failed message=%s: %v\n", messageID, err)
		s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": messageID, "chatId": chatID, "status": "failed"}})
		return
	}
	now := time.Now()
	dir := filepath.Join(s.mediaRoot, fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", int(now.Month())))
	if err = os.MkdirAll(dir, 0755); err != nil {
		_ = s.store.MediaFailed(messageID, err)
		fmt.Printf("[media] failed message=%s: %v\n", messageID, err)
		s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": messageID, "chatId": chatID, "status": "failed"}})
		return
	}
	ext := safeExt(mimeType, filename)
	path := filepath.Join(dir, messageID+ext)
	if err = os.WriteFile(path, data, 0644); err != nil {
		_ = s.store.MediaFailed(messageID, err)
		fmt.Printf("[media] failed message=%s: %v\n", messageID, err)
		s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": messageID, "chatId": chatID, "status": "failed"}})
		return
	}
	if filename == "" {
		filename = filepath.Base(path)
	}
	row, err := s.store.MediaReady(messageID, mimeType, filename, path, int64(len(data)))
	if err != nil {
		fmt.Printf("[media] database update failed message=%s: %v\n", messageID, err)
		_ = s.store.MediaFailed(messageID, err)
		s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": messageID, "chatId": chatID, "status": "failed"}})
		return
	}
	s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": messageID, "chatId": chatID, "media": row}})
}
func (s *Service) mediaDownloadable(messageID string) (whatsmeow.DownloadableMessage, error) {
	s.mediaMu.Lock()
	dl := s.mediaRecent[messageID]
	s.mediaMu.Unlock()
	if dl != nil {
		return dl, nil
	}

	raw, err := s.store.MediaPayload(messageID)
	if err == nil && len(raw) > 0 {
		msg := &waE2E.Message{}
		if err = proto.Unmarshal(raw, msg); err == nil {
			dl = downloadable(msg)
			if dl != nil {
				s.rememberMedia(messageID, dl)
				return dl, nil
			}
		}
	}
	return nil, fmt.Errorf("media metadata is unavailable; reload older history to make it fetchable")
}

// EnsureMedia performs a lazy fetch for historical media. The GET handler calls
// this synchronously so the same HTTP request can return the newly cached file.
// A lock prevents chat view and gallery from downloading the same object twice.
func (s *Service) EnsureMedia(messageID string) error {
	if s.mediaRetryActive(messageID) {
		return fmt.Errorf("waiting for WhatsApp media retry")
	}
	s.mediaFetchMu.Lock()
	defer s.mediaFetchMu.Unlock()

	if s.mediaRetryActive(messageID) {
		return fmt.Errorf("waiting for WhatsApp media retry")
	}

	row, err := s.store.MediaByMessage(messageID)
	if err != nil {
		return err
	}
	if row.Status == "ready" && row.Path.Valid {
		if _, err := os.Stat(row.Path.String); err == nil {
			return nil
		}
	}

	dl, err := s.mediaDownloadable(messageID)
	if err != nil {
		return err
	}

	s.downloadMedia(messageID, row.ChatID, dl)

	row, err = s.store.MediaByMessage(messageID)
	if err != nil {
		return err
	}
	if row.Status != "ready" || !row.Path.Valid {
		if row.Error.Valid && row.Error.String != "" {
			return fmt.Errorf("%s", row.Error.String)
		}
		return fmt.Errorf("media download did not become ready")
	}
	return nil
}

func (s *Service) RetryMedia(messageID string) error {
	if s.mediaRetryActive(messageID) {
		return nil
	}
	dl, err := s.mediaDownloadable(messageID)
	if err != nil {
		return err
	}
	// Historical CDN descriptors can expire or point at ciphertext that no
	// longer matches the stored hash. Ask WhatsApp for a refreshed descriptor
	// first instead of immediately repeating the same doomed download.
	if err := s.store.MediaPending(messageID); err != nil {
		return err
	}
	if s.requestMediaRetry(messageID, dl) {
		return nil
	}
	// A receipt failure must not recursively request another receipt.
	row, err := s.store.MediaByMessage(messageID)
	if err != nil {
		return err
	}
	go s.downloadMediaAttempt(messageID, row.ChatID, dl, false)
	return nil
}
