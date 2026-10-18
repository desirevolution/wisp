package mqttsvc

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

type Service struct {
	st                 *store.Store
	bus                *events.Bus
	mu                 sync.RWMutex
	client             mqtt.Client
	enabled, connected bool
	broker, prefix     string
	subID              int
	stop               chan struct{}
}

type MessagePayload struct {
	ChatName   string `json:"chatName"`
	SenderName string `json:"senderName"`
	IsGroup    bool   `json:"isGroup"`
	Text       string `json:"text"`
}

type Status struct {
	Enabled     bool   `json:"enabled"`
	Connected   bool   `json:"connected"`
	Broker      string `json:"broker,omitempty"`
	TopicPrefix string `json:"topicPrefix"`
}

func New(st *store.Store, bus *events.Bus) *Service {
	return &Service{st: st, bus: bus, stop: make(chan struct{})}
}
func (s *Service) Start() { id, ch := s.bus.Subscribe(); s.subID = id; go s.run(ch); s.Reload() }
func (s *Service) Close() {
	close(s.stop)
	s.bus.Unsubscribe(s.subID)
	s.mu.Lock()
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250)
	}
	s.client = nil
	s.connected = false
	s.mu.Unlock()
}
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Enabled: s.enabled, Connected: s.connected, Broker: s.broker, TopicPrefix: s.prefix}
}
func (s *Service) Reload() {
	enabled := s.st.ConfigBool("mqttEnabled", false)
	broker := strings.TrimSpace(s.st.ConfigString("mqttBroker", ""))
	prefix := strings.Trim(strings.TrimSpace(s.st.ConfigString("mqttTopicPrefix", "wisp")), "/")
	if prefix == "" {
		prefix = "wisp"
	}
	user := s.st.ConfigString("mqttUsername", "")
	pass := s.st.ConfigString("mqttPassword", "")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		if s.client.IsConnected() {
			s.client.Disconnect(250)
		}
		s.client = nil
	}
	s.enabled = enabled
	s.connected = false
	s.broker = broker
	s.prefix = prefix
	if !enabled || broker == "" {
		return
	}
	opts := mqtt.NewClientOptions().AddBroker(broker).SetClientID(fmt.Sprintf("wisp-%d", time.Now().UnixNano())).SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(5 * time.Second)
	if user != "" {
		opts.SetUsername(user)
		opts.SetPassword(pass)
	}
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		s.mu.Lock()
		s.connected = true
		s.mu.Unlock()
		log.Printf("[mqtt] event=connected")
		s.publishStatus(c, "connected")
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, _ error) {
		s.mu.Lock()
		s.connected = false
		s.mu.Unlock()
		log.Printf("[mqtt] event=connection_lost")
	})
	c := mqtt.NewClient(opts)
	s.client = c
	go func() {
		tok := c.Connect()
		if tok.WaitTimeout(10*time.Second) && tok.Error() != nil {
			log.Printf("[mqtt] event=connect_failed")
		}
	}()
}
func (s *Service) topic(suffix string) string {
	s.mu.RLock()
	p := s.prefix
	s.mu.RUnlock()
	return p + "/" + suffix
}
func (s *Service) publishStatus(c mqtt.Client, state string) {
	b, _ := json.Marshal(map[string]any{"state": state, "timestamp": time.Now().Unix()})
	c.Publish(s.topic("status"), 1, true, b)
}
func (s *Service) publish(topic string, qos byte, retained bool, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.RLock()
	c := s.client
	ok := s.enabled && s.connected && c != nil
	s.mu.RUnlock()
	if ok {
		c.Publish(s.topic(topic), qos, retained, b)
	}
}
func (s *Service) run(ch <-chan events.Event) {
	for {
		select {
		case <-s.stop:
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			switch e.Type {
			case "message":
				if m, ok := e.Data.(model.Message); ok && m.FromMe == 0 && !m.Silent && !s.st.ChatMuted(m.ChatID) {
					senderName := ""
					if m.SenderName != nil {
						senderName = *m.SenderName
					}
					s.publish("message", 1, false, MessagePayload{
						ChatName:   s.st.ChatName(m.ChatID),
						SenderName: senderName,
						IsGroup:    s.st.ChatType(m.ChatID) == "group",
						Text:       m.Body,
					})
				}
			case "connection":
				s.publish("status", 1, true, e.Data)
			}
		}
	}
}
