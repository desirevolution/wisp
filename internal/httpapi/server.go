package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"wisp/internal/appconfig"
	"wisp/internal/events"
	"wisp/internal/mqttsvc"
	"wisp/internal/pushsvc"
	"wisp/internal/store"
	"wisp/internal/updates"
	"wisp/internal/whatsapp"
)

type Server struct {
	updateChecker *updates.Checker
	st            *store.Store
	wa            *whatsapp.Service
	bus           *events.Bus
	mqtt          *mqttsvc.Service
	push          *pushsvc.Service
	public        string
	assets        fs.FS

	mediaQueueMu sync.Mutex
	mediaQueued  map[string]struct{}
}

func (s *Server) SetUpdateChecker(checker *updates.Checker) { s.updateChecker = checker }

func New(st *store.Store, wa *whatsapp.Service, b *events.Bus, mq *mqttsvc.Service, push *pushsvc.Service, assets fs.FS, public string) *Server {
	return &Server{st: st, wa: wa, bus: b, mqtt: mq, push: push, assets: assets, public: public, mediaQueued: make(map[string]struct{})}
}

// queueVisibleMedia starts lazy downloads only for visual media that the current
// message page exposes. This avoids downloading the whole history while making
// the unchanged frontend receive its normal media SSE update once the file is ready.
func (s *Server) queueVisibleMedia(messageID, kind, status string) {
	// History imports create media rows as pending before any download has
	// started. Therefore pending must be eligible for this lazy queue. The
	// in-memory mediaQueued set below prevents duplicate queue work while a
	// visible item is being fetched. Failed items still require explicit retry.
	if status == "ready" || status == "failed" {
		return
	}
	switch kind {
	case "image", "video", "sticker":
	default:
		return
	}

	s.mediaQueueMu.Lock()
	if _, exists := s.mediaQueued[messageID]; exists {
		s.mediaQueueMu.Unlock()
		return
	}
	s.mediaQueued[messageID] = struct{}{}
	s.mediaQueueMu.Unlock()

	go func() {
		defer func() {
			s.mediaQueueMu.Lock()
			delete(s.mediaQueued, messageID)
			s.mediaQueueMu.Unlock()
		}()
		if err := s.wa.EnsureMedia(messageID); err != nil {
			fmt.Printf("[media-queue] failed message=%s: %v\n", messageID, err)
			return
		}
	}()
}
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/health", s.health)
	m.HandleFunc("/api/status", s.status)
	m.HandleFunc("/api/config", s.config)
	m.HandleFunc("/api/push/public-key", s.pushPublicKey)
	m.HandleFunc("/api/push/subscribe", s.pushSubscribe)
	m.HandleFunc("/api/push/test", s.pushTest)
	m.HandleFunc("/api/whatsapp/reconnect", s.whatsappReconnect)
	m.HandleFunc("/api/whatsapp/logout", s.whatsappLogout)
	m.HandleFunc("/api/chats", s.chats)
	m.HandleFunc("/api/chat-options", s.chatOptions)
	m.HandleFunc("/api/search", s.search)
	m.HandleFunc("/api/chats/", s.chat)
	m.HandleFunc("/api/events", s.sse)
	m.HandleFunc("/api/media/", s.media)
	m.HandleFunc("/", s.static)
	return m
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	state, le, lm := s.wa.Health()
	dbErr := s.st.Health()
	ok := state == "connected" && dbErr == nil
	code := 503
	if ok {
		code = 200
	}
	var a, b any
	if le != nil {
		a = le.Format(time.RFC3339)
	}
	if lm != nil {
		b = lm.Format(time.RFC3339)
	}
	jsonOut(w, code, map[string]any{"status": map[bool]string{true: "ok", false: "unhealthy"}[ok], "whatsapp": state, "database": map[bool]string{true: "ok", false: "error"}[dbErr == nil], "lastEventAt": a, "lastMessageAt": b, "mqtt": s.mqtt.Status()})
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	var update *updates.State
	if s.updateChecker != nil {
		state := s.updateChecker.Status()
		update = &state
	}
	prefs, err := s.st.ChatStates()
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "Could not load chat settings"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, 200, map[string]any{"chat_states": prefs, "version": appconfig.Version, "update": update, "whatsapp": s.wa.Status(), "push": s.push.Status(), "send": map[string]any{"enabled": !s.st.ConfigBool("mirrorOnly", false)}})
}
func (s *Server) pushPublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	jsonOut(w, 200, map[string]string{"publicKey": s.push.PublicKey()})
}
func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		if err != nil {
			jsonOut(w, 400, map[string]string{"error": "invalid subscription"})
			return
		}
		if err = s.push.Subscribe(raw); err != nil {
			jsonOut(w, 400, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, map[string]bool{"ok": true})
	case http.MethodDelete:
		var body struct {
			Endpoint string `json:"endpoint"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body) != nil || body.Endpoint == "" {
			jsonOut(w, 400, map[string]string{"error": "endpoint required"})
			return
		}
		if err := s.push.Unsubscribe(body.Endpoint); err != nil {
			jsonOut(w, 500, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) configResponse(mirror bool) map[string]any {
	return map[string]any{
		"sendEnabled":       !mirror,
		"mirrorOnly":        mirror,
		"followNewMessages": s.st.ConfigBool("followNewMessages", false),
		"readMode":          s.st.ReadMode(),
	}
}
func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		mirror := s.st.ConfigBool("mirrorOnly", false)
		jsonOut(w, 200, s.configResponse(mirror))
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		FollowNewMessages *bool   `json:"followNewMessages"`
		ReadMode          *string `json:"readMode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if body.ReadMode != nil {
		if !store.ValidReadMode(*body.ReadMode) {
			jsonOut(w, 400, map[string]string{"error": "Invalid read mode"})
			return
		}
		if err := s.wa.SetReadMode(*body.ReadMode); err != nil {
			jsonOut(w, 500, map[string]string{"error": "Could not save read mode"})
			return
		}
	}
	if body.FollowNewMessages != nil {
		if err := s.st.SetConfigBool("followNewMessages", *body.FollowNewMessages); err != nil {
			jsonOut(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	mirror := s.st.ConfigBool("mirrorOnly", false)
	s.bus.Publish(events.Event{Type: "config", Data: s.configResponse(mirror)})
	jsonOut(w, 200, s.configResponse(mirror))
}

func (s *Server) whatsappReconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.wa.Reconnect(ctx); err != nil {
		jsonOut(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) whatsappLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := s.wa.LogoutAndPair(ctx); err != nil {
		jsonOut(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) chats(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/chats" {
		http.NotFound(w, r)
		return
	}
	x, e := s.st.Chats()
	if e != nil {
		jsonOut(w, 500, map[string]string{"error": e.Error()})
		return
	}
	prefs, err := s.st.ChatStates()
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "Could not load chat settings"})
		return
	}
	states := map[string]store.ChatState{}
	for _, p := range prefs {
		states[p.ChatID] = p
	}
	visible := []map[string]any{}
	for _, row := range x {
		id, _ := row["id"].(string)
		p := states[id]
		if p.Closed {
			continue
		}
		row["mute_until"] = p.MuteUntil
		visible = append(visible, row)
	}
	x = visible
	s.wa.ResolveMentionRows(r.Context(), x, "id", "last_body", "last_message_id")
	jsonOut(w, 200, x)
}
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/chats/")
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id, _ := urlDecode(parts[0])
	if len(parts) == 2 && parts[1] == "read" {
		s.markChatRead(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "state" {
		s.changeChatState(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "edits" && r.Method == http.MethodGet {
		ids := r.URL.Query()["message_id"]
		if len(ids) > 200 {
			jsonOut(w, 400, map[string]string{"error": "too many messages"})
			return
		}
		states := []any{}
		for _, mid := range ids {
			state, err := s.wa.EditState(r.Context(), s.st.ResolveChat(id), mid)
			if err != nil {
				jsonOut(w, 500, map[string]string{"error": err.Error()})
				return
			}
			states = append(states, state)
		}
		jsonOut(w, 200, states)
		return
	}
	if len(parts) == 2 && parts[1] == "deletions" && r.Method == http.MethodGet {
		rows, err := s.st.Deletions(r.Context(), s.st.ResolveChat(id))
		if err != nil {
			jsonOut(w, 500, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, rows)
		return
	}
	if len(parts) == 2 && parts[1] == "reactions" && r.Method == http.MethodGet {
		ids := r.URL.Query()["message_id"]
		if len(ids) > 200 {
			jsonOut(w, 400, map[string]string{"error": "too many messages"})
			return
		}
		states := []any{}
		for _, mid := range ids {
			state, err := s.wa.ReactionState(r.Context(), s.st.ResolveChat(id), mid)
			if err != nil {
				jsonOut(w, 500, map[string]string{"error": err.Error()})
				return
			}
			states = append(states, state)
		}
		jsonOut(w, 200, states)
		return
	}
	if len(parts) == 4 && parts[1] == "messages" && parts[3] == "reactions" {
		mid, _ := urlDecode(parts[2])
		target, err := s.st.MessageByID(mid)
		if err != nil || target.ChatID != s.st.ResolveChat(id) {
			jsonOut(w, 404, map[string]string{"error": "Message not found"})
			return
		}
		if r.Method == http.MethodGet {
			rows, err := s.wa.ReactionState(r.Context(), target.ChatID, mid)
			if err != nil {
				jsonOut(w, 500, map[string]string{"error": err.Error()})
				return
			}
			jsonOut(w, 200, rows)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.st.ConfigBool("mirrorOnly", false) {
			jsonOut(w, 403, map[string]string{"error": "Wisp is in mirror-only mode"})
			return
		}
		var body struct {
			Emoji *string `json:"emoji"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || body.Emoji == nil {
			jsonOut(w, 400, map[string]string{"error": "emoji is required"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		rows, err := s.wa.SendReaction(ctx, id, mid, *body.Emoji)
		if err != nil {
			code := 502
			if errors.Is(err, whatsapp.ErrInvalidAction) {
				code = 400
			}
			jsonOut(w, code, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, map[string]any{"ok": true, "reactions": rows.Reactions, "version": rows.Version, "chat_id": rows.ChatID, "message_id": rows.MessageID})
		return
	}
	if len(parts) == 4 && parts[1] == "messages" && parts[3] == "context" && r.Method == "GET" {
		mid, _ := urlDecode(parts[2])
		s.messageContext(w, r, id, mid)
		return
	}
	if len(parts) == 2 && parts[1] == "media" && r.Method == "GET" {
		rows, err := s.st.GalleryMedia(id, r.URL.Query().Get("kind"), 100)
		if err != nil {
			jsonOut(w, 500, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, rows)
		return
	}
	if len(parts) == 2 && parts[1] == "links" && r.Method == "GET" {
		rows, err := s.st.Links(id, 100)
		if err != nil {
			jsonOut(w, 500, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, rows)
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	if parts[1] == "send-image" && r.Method == "POST" {
		s.sendImage(w, r, id)
		return
	}
	if parts[1] == "send" && r.Method == "POST" {
		if s.st.ConfigBool("mirrorOnly", false) {
			jsonOut(w, 403, map[string]string{"error": "Wisp is in mirror-only mode"})
			return
		}
		var body struct {
			Text    string `json:"text"`
			ReplyTo string `json:"reply_to"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		if err := dec.Decode(&body); err != nil {
			jsonOut(w, 400, map[string]string{"error": "invalid request body"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		msgID, err := s.wa.SendText(ctx, id, body.Text, body.ReplyTo, r.Header.Get("X-Wisp-Message-ID"))
		if err != nil {
			code := 502
			if errors.Is(err, whatsapp.ErrInvalidAction) {
				code = 400
			}
			jsonOut(w, code, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, map[string]any{"ok": true, "id": msgID})
		return
	}
	if parts[1] != "messages" || r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	s.timeline(w, r, id)
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/media/")
	if strings.HasPrefix(p, "message/") && strings.HasSuffix(p, "/retry") && r.Method == "POST" {
		id := strings.TrimSuffix(strings.TrimPrefix(p, "message/"), "/retry")
		id, _ = urlDecode(id)
		if err := s.wa.RetryMedia(id); err != nil {
			jsonOut(w, 409, map[string]string{"error": err.Error()})
			return
		}
		jsonOut(w, 200, map[string]any{"ok": true})
		return
	}
	if r.Method != "GET" || strings.Contains(p, "/") {
		http.NotFound(w, r)
		return
	}
	id, _ := urlDecode(p)
	row, err := s.st.Media(id)
	if err != nil {
		fmt.Printf("[media] lookup failed id=%s: %v\n", id, err)
		jsonOut(w, 404, map[string]string{"error": "Media not found"})
		return
	}

	// Historical media is intentionally not downloaded during bootstrap. Fetch it
	// on first access, cache it locally, then serve it in this same request.
	if row.Status != "ready" || !row.Path.Valid {
		if err := s.wa.EnsureMedia(row.MessageID); err != nil {
			fmt.Printf("[media] lazy fetch failed id=%s message=%s: %v\n", row.ID, row.MessageID, err)
			jsonOut(w, 409, map[string]string{"error": err.Error()})
			return
		}
		row, err = s.st.Media(id)
		if err != nil || row.Status != "ready" || !row.Path.Valid {
			jsonOut(w, 404, map[string]string{"error": "Media not found after fetch"})
			return
		}
	}

	f, err := os.Open(row.Path.String)
	if err != nil {
		jsonOut(w, 404, map[string]string{"error": "Media not found"})
		return
	}
	defer f.Close()
	if row.MimeType.Valid {
		w.Header().Set("Content-Type", row.MimeType.String)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	name := filepath.Base(row.Path.String)
	if row.Filename.Valid {
		name = strings.NewReplacer("\"", "_", "\r", "_", "\n", "_").Replace(row.Filename.String)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", name))
	http.ServeContent(w, r, name, time.Unix(0, 0), f)
}

func urlDecode(x string) (string, error) {
	r, e := http.NewRequest("GET", "http://x/"+x, nil)
	if e != nil {
		return x, e
	}
	return strings.TrimPrefix(r.URL.Path, "/"), nil
}
func (s *Server) sse(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	writeEvent(w, "connection", s.wa.Status())
	f.Flush()
	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case e := <-ch:
			writeEvent(w, e.Type, e.Data)
			f.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
func writeEvent(w http.ResponseWriter, t string, d any) {
	b, _ := json.Marshal(d)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", t, b)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		jsonOut(w, 404, map[string]string{"error": "Not found"})
		return
	}
	switch r.URL.Path {
	case "/", "/index.html", "/sw.js", "/manifest.webmanifest":
		w.Header().Set("Cache-Control", "no-cache")
	}
	isBuildAsset := strings.HasPrefix(r.URL.Path, "/assets/")
	// PUBLIC_DIR is a development escape hatch. Production serves the Vite build embedded in the Go binary.
	if s.public != "" {
		p := filepath.Join(s.public, filepath.Clean(r.URL.Path))
		if r.URL.Path == "/" {
			p = filepath.Join(s.public, "index.html")
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		if isBuildAsset {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(s.public, "index.html"))
		return
	}
	assetPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if assetPath == "." || assetPath == "" {
		assetPath = "index.html"
	}
	if f, err := s.assets.Open(assetPath); err == nil {
		f.Close()
		http.FileServer(http.FS(s.assets)).ServeHTTP(w, r)
		return
	}
	if isBuildAsset {
		http.NotFound(w, r)
		return
	}
	// FileServer redirects /index.html to ./; serve the root for SPA fallbacks.
	// Work on a clone so the original request is not modified.
	w.Header().Set("Cache-Control", "no-cache")
	fallback := r.Clone(r.Context())
	fallback.URL.Path = "/"
	http.FileServer(http.FS(s.assets)).ServeHTTP(w, fallback)
}
