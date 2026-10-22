package mqttsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

type Service struct {
	st              *store.Store
	bus             *events.Bus
	mu              sync.RWMutex
	lifecycle       sync.Mutex
	client          mqtt.Client
	enabled         bool
	broker, prefix  string
	subIDs          []int
	stop            chan struct{}
	started, closed bool // lifecycle protects lifecycle operations
	wg              sync.WaitGroup
	diagnostics     Status
	newClient       func(*mqtt.ClientOptions) mqtt.Client
	publishTimeout  time.Duration
}
type MessagePayload struct {
	ChatName   string `json:"chatName"`
	SenderName string `json:"senderName"`
	IsGroup    bool   `json:"isGroup"`
	Text       string `json:"text"`
}
type Status struct {
	Enabled            bool   `json:"enabled"`
	Connected          bool   `json:"connected"`
	Broker             string `json:"broker,omitempty"`
	TopicPrefix        string `json:"topicPrefix"`
	Published          uint64 `json:"published"`
	PublishErrors      uint64 `json:"publish_errors"`
	SkippedOffline     uint64 `json:"skipped_offline"`
	ReconnectAttempts  uint64 `json:"reconnect_attempts"`
	LastConnectedAt    string `json:"last_connected_at,omitempty"`
	LastDisconnectedAt string `json:"last_disconnected_at,omitempty"`
	LastPublishedAt    string `json:"last_published_at,omitempty"`
	LastError          string `json:"last_error,omitempty"`
}

func New(st *store.Store, bus *events.Bus) *Service {
	return &Service{st: st, bus: bus, stop: make(chan struct{}), newClient: mqtt.NewClient, publishTimeout: 10 * time.Second}
}
func (s *Service) Start() {
	s.lifecycle.Lock()
	if s.started || s.closed {
		s.lifecycle.Unlock()
		return
	}
	s.started = true
	messageID, messages := s.bus.SubscribeReliable("message")
	connectionID, connections := s.bus.SubscribeReliable("connection")
	s.subIDs = []int{messageID, connectionID}
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.runEvents(messages, connections) }()
	s.lifecycle.Unlock()
	s.Reload()
}
func (s *Service) Close() {
	s.lifecycle.Lock()
	if s.closed {
		s.lifecycle.Unlock()
		return
	}
	s.closed = true
	close(s.stop)
	for _, id := range s.subIDs {
		s.bus.Unsubscribe(id)
	}
	s.mu.Lock()
	c := s.client
	s.client = nil
	s.mu.Unlock()
	// Disconnect also cancels initial connection retries. Never hold mu while
	// waiting for Paho: its asynchronous callbacks also inspect this service.
	if c != nil {
		c.Disconnect(250)
	}
	s.lifecycle.Unlock()
	s.wg.Wait()
}
func (s *Service) Status() Status {
	if s == nil {
		return Status{TopicPrefix: "wisp"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := s.diagnostics
	result.Enabled = s.enabled
	result.Broker = s.broker
	result.TopicPrefix = s.prefix
	// Callback order is not guaranteed. Paho's transport state is authoritative.
	result.Connected = s.enabled && s.client != nil && s.client.IsConnectionOpen()
	return result
}
func (s *Service) Reload() {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closed {
		return
	}
	enabled := s.st.ConfigBool("mqttEnabled", false)
	broker := strings.TrimSpace(s.st.ConfigString("mqttBroker", ""))
	prefix := strings.Trim(strings.TrimSpace(s.st.ConfigString("mqttTopicPrefix", "wisp")), "/")
	if prefix == "" {
		prefix = "wisp"
	}
	user, pass := s.st.ConfigString("mqttUsername", ""), s.st.ConfigString("mqttPassword", "")
	s.mu.Lock()
	previous := s.client
	s.client = nil
	s.enabled = enabled
	s.broker = broker
	s.prefix = prefix
	s.mu.Unlock()
	if previous != nil {
		previous.Disconnect(250)
	}
	if !enabled || broker == "" {
		return
	}
	opts := mqtt.NewClientOptions().AddBroker(broker).
		SetClientID(fmt.Sprintf("wisp-%d", time.Now().UnixNano())).
		SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(30 * time.Second).SetConnectTimeout(5 * time.Second).
		SetKeepAlive(20 * time.Second).SetPingTimeout(5 * time.Second).SetWriteTimeout(5 * time.Second)
	if user != "" {
		opts.SetUsername(user)
		opts.SetPassword(pass)
	}
	secrets := []string{pass}
	if parsed, err := url.Parse(broker); err == nil && parsed.User != nil {
		if p, ok := parsed.User.Password(); ok {
			secrets = append(secrets, p)
		}
	}
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		s.mu.Lock()
		if s.client != c || !c.IsConnectionOpen() {
			s.mu.Unlock()
			return
		}
		s.diagnostics.LastConnectedAt = time.Now().UTC().Format(time.RFC3339)
		s.diagnostics.LastError = ""
		s.mu.Unlock()
		log.Printf("[mqtt] event=connected")
		s.publishStatus(c, "connected")
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		s.mu.Lock()
		if s.client != c {
			s.mu.Unlock()
			return
		}
		// A delayed loss callback must never mark a reconnected transport offline.
		reason := redactError(err, secrets)
		if !c.IsConnectionOpen() {
			s.diagnostics.LastDisconnectedAt = time.Now().UTC().Format(time.RFC3339)
			s.diagnostics.LastError = reason
		}
		s.mu.Unlock()
		log.Printf("[mqtt] event=connection_lost error=%q", reason)
	})
	opts.SetConnectionNotificationHandler(func(c mqtt.Client, notification mqtt.ConnectionNotification) {
		s.mu.Lock()
		if s.client != c {
			s.mu.Unlock()
			return
		}
		switch event := notification.(type) {
		case mqtt.ConnectionNotificationConnecting:
			if event.IsReconnect {
				s.diagnostics.ReconnectAttempts++
			}
			s.mu.Unlock()
			log.Printf("[mqtt] event=connect_attempt reconnect=%t attempt=%d", event.IsReconnect, event.Attempt+1)
		case mqtt.ConnectionNotificationBrokerFailed:
			reason := redactError(event.Reason, secrets)
			if !c.IsConnectionOpen() {
				s.diagnostics.LastError = reason
			}
			s.mu.Unlock()
			log.Printf("[mqtt] event=connect_failed error=%q", reason)
		default:
			s.mu.Unlock()
		}
	})
	c := s.newClient(opts)
	s.mu.Lock()
	s.client = c
	s.mu.Unlock()
	token := c.Connect()
	// ConnectRetry keeps the token pending while the broker is unavailable.
	// Attempt notifications above report each failure, rather than silently timing out.
	if token != nil {
		go func() {
			select {
			case <-s.stop:
				return
			case <-token.Done():
				if err := token.Error(); err != nil {
					s.recordError(c, "connect_failed", redactError(err, secrets))
				}
			}
		}()
	}
}
func redactError(err error, secrets []string) string {
	if err == nil {
		return "connection closed"
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
			message = strings.ReplaceAll(message, url.QueryEscape(secret), "[redacted]")
		}
	}
	return message
}
func (s *Service) recordError(c mqtt.Client, event, reason string) {
	s.mu.Lock()
	if s.client != c {
		s.mu.Unlock()
		return
	}
	s.diagnostics.LastError = reason
	s.mu.Unlock()
	log.Printf("[mqtt] event=%s error=%q", event, reason)
}
func (s *Service) publishStatus(c mqtt.Client, state string) {
	s.publishFor(c, "status", 1, true, map[string]any{"state": state, "timestamp": time.Now().Unix()})
}
func (s *Service) publish(suffix string, qos byte, retained bool, value any) {
	s.mu.RLock()
	c := s.client
	enabled := s.enabled
	s.mu.RUnlock()
	if !enabled {
		return
	}
	if c == nil || !c.IsConnectionOpen() {
		if suffix == "message" {
			s.mu.Lock()
			s.diagnostics.SkippedOffline++
			s.mu.Unlock()
			log.Printf("[mqtt] event=message_skipped reason=offline")
		}
		return
	}
	s.publishFor(c, suffix, qos, retained, value)
}
func (s *Service) publishFor(c mqtt.Client, suffix string, qos byte, retained bool, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		s.recordError(c, "encode_failed", err.Error())
		return
	}
	s.mu.RLock()
	current := s.client == c && s.enabled
	topic := s.prefix + "/" + suffix
	s.mu.RUnlock()
	if !current {
		return
	}
	token := c.Publish(topic, qos, retained, payload)
	if token == nil {
		err = errors.New("MQTT client returned no publish token")
	} else {
		timer := time.NewTimer(s.publishTimeout)
		defer timer.Stop()
		select {
		case <-s.stop:
			return
		case <-token.Done():
			err = token.Error()
		case <-timer.C:
			err = errors.New("broker did not acknowledge publish before timeout")
			// Even a peer that still answers pings can stop acknowledging publishes.
			// Cancel the stuck session; the watchdog starts a fresh client afterwards.
			s.mu.RLock()
			current = s.client == c
			s.mu.RUnlock()
			if current {
				c.Disconnect(250)
			}
		}
	}
	s.mu.Lock()
	if s.client != c {
		s.mu.Unlock()
		return
	}
	if err != nil {
		s.diagnostics.PublishErrors++
		s.diagnostics.LastError = err.Error()
	} else {
		s.diagnostics.Published++
		s.diagnostics.LastPublishedAt = time.Now().UTC().Format(time.RFC3339)
	}
	s.mu.Unlock()
	if err != nil {
		log.Printf("[mqtt] event=publish_failed suffix=%s error=%q", suffix, err)
	} else {
		log.Printf("[mqtt] event=published suffix=%s qos=%d", suffix, qos)
	}
}
func (s *Service) recoverStopped() {
	s.mu.RLock()
	c := s.client
	enabled := s.enabled
	s.mu.RUnlock()
	// IsConnected also includes Paho's connecting/reconnecting states. Only replace
	// a client that stopped retrying entirely, never interrupt a healthy retry loop.
	if enabled && c != nil && !c.IsConnected() {
		log.Printf("[mqtt] event=restart_stopped_client")
		s.Reload()
	}
}
func (s *Service) run(ch <-chan events.Event) { s.runEvents(ch, nil) }
func (s *Service) runEvents(messages, connections <-chan events.Event) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.recoverStopped()
		case event, ok := <-messages:
			if !ok {
				return
			}
			s.handle(event)
		case event, ok := <-connections:
			if !ok {
				connections = nil
				continue
			}
			s.handle(event)
		}
	}
}
func (s *Service) handle(event events.Event) {
	switch event.Type {
	case "message":
		if m, ok := event.Data.(model.Message); ok && m.FromMe == 0 && !m.Silent && !s.st.ChatMuted(m.ChatID) {
			sender := ""
			if m.SenderName != nil {
				sender = *m.SenderName
			}
			s.publish("message", 1, false, MessagePayload{ChatName: s.st.ChatName(m.ChatID), SenderName: sender, IsGroup: s.st.ChatType(m.ChatID) == "group", Text: m.Body})
		}
	case "connection":
		s.publish("status", 1, true, event.Data)
	}
}
