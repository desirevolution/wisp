package mqttsvc

import (
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/eclipse/paho.mqtt.golang/packets"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

// A local wire-level fixture exercises the actual pinned Paho client; it accepts
// MQTT CONNECT, publishes and keepalives, and can stop/restart or withhold PUBACK.
type wireBroker struct {
	listener    net.Listener
	mu          sync.Mutex
	clients     map[net.Conn]struct{}
	closed      bool
	wg          sync.WaitGroup
	messages    chan *packets.PublishPacket
	acknowledge atomic.Bool
	silent      atomic.Bool
}

func newWireBroker(t *testing.T, address string) *wireBroker {
	t.Helper()
	ln, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	b := &wireBroker{listener: ln, clients: map[net.Conn]struct{}{}, messages: make(chan *packets.PublishPacket, 1000)}
	b.acknowledge.Store(true)
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			b.mu.Lock()
			if b.closed {
				b.mu.Unlock()
				conn.Close()
				return
			}
			b.clients[conn] = struct{}{}
			b.wg.Add(1)
			b.mu.Unlock()
			go b.serve(conn)
		}
	}()
	t.Cleanup(b.close)
	return b
}
func (b *wireBroker) serve(conn net.Conn) {
	defer b.wg.Done()
	defer func() { conn.Close(); b.mu.Lock(); delete(b.clients, conn); b.mu.Unlock() }()
	for {
		packet, err := packets.ReadPacket(conn)
		if err != nil {
			return
		}
		if b.silent.Load() {
			continue
		}
		var response packets.ControlPacket
		switch p := packet.(type) {
		case *packets.ConnectPacket:
			response = packets.NewControlPacket(packets.Connack)
		case *packets.PublishPacket:
			b.messages <- p
			if p.Qos == 1 && b.acknowledge.Load() {
				ack := packets.NewControlPacket(packets.Puback).(*packets.PubackPacket)
				ack.MessageID = p.MessageID
				response = ack
			}
		case *packets.PingreqPacket:
			response = packets.NewControlPacket(packets.Pingresp)
		case *packets.DisconnectPacket:
			return
		}
		if response != nil {
			if err = response.Write(conn); err != nil {
				return
			}
		}
	}
}
func (b *wireBroker) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.listener.Close()
	for c := range b.clients {
		c.Close()
	}
	b.mu.Unlock()
	b.wg.Wait()
}
func await(t *testing.T, description string, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out: " + description)
}
func mqttService(t *testing.T, address string) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	st.SetConfigOverride("mqttEnabled", "true")
	st.SetConfigOverride("mqttBroker", "tcp://"+address)
	if err = st.OpenChat("c", "Contact", "private"); err != nil {
		t.Fatal(err)
	}
	s := New(st, events.New())
	s.newClient = func(opts *mqtt.ClientOptions) mqtt.Client {
		opts.SetConnectRetryInterval(50 * time.Millisecond).SetMaxReconnectInterval(100 * time.Millisecond).
			SetConnectTimeout(100 * time.Millisecond).SetKeepAlive(time.Second).SetPingTimeout(100 * time.Millisecond).SetWriteTimeout(100 * time.Millisecond)
		return mqtt.NewClient(opts)
	}
	t.Cleanup(s.Close)
	return s
}
func receiveText(t *testing.T, b *wireBroker, text string) {
	t.Helper()
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	for {
		select {
		case packet := <-b.messages:
			if packet.TopicName != "wisp/message" {
				continue
			}
			var payload MessagePayload
			if err := json.Unmarshal(packet.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Text == text {
				return
			}
		case <-timer.C:
			t.Fatalf("did not receive %q", text)
		}
	}
}
func TestRealMQTTBrokerRestartAndMessageBurst(t *testing.T) {
	first := newWireBroker(t, "127.0.0.1:0")
	address := first.listener.Addr().String()
	s := mqttService(t, address)
	s.Start()
	await(t, "initial connection", func() bool { return s.Status().Connected })
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "before"}})
	receiveText(t, first, "before")
	first.close()
	await(t, "disconnection", func() bool { return !s.Status().Connected })
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "during outage"}})
	await(t, "offline diagnostic", func() bool { return s.Status().SkippedOffline == 1 })
	second := newWireBroker(t, address)
	await(t, "automatic reconnect", func() bool { return s.Status().Connected })
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "after"}})
	receiveText(t, second, "after")
	// More than the old 64-event subscription buffer must all reach the broker.
	baseline := s.Status().Published
	for i := 0; i < 200; i++ {
		s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "burst"}})
	}
	count := 0
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	for count < 200 {
		select {
		case p := <-second.messages:
			if p.TopicName == "wisp/message" {
				count++
			}
		case <-timer.C:
			t.Fatalf("burst delivered %d/200", count)
		}
	}
	await(t, "publish acknowledgments", func() bool { return s.Status().Published >= baseline+200 })
	if s.Status().PublishErrors != 0 {
		t.Fatal(s.Status())
	}
}
func TestRealMQTTBrokerUnavailableAtStartup(t *testing.T) {
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	reservation.Close()
	s := mqttService(t, address)
	s.Start()
	await(t, "reported initial connection failure", func() bool { return s.Status().LastError != "" })
	broker := newWireBroker(t, address)
	await(t, "initial retry connects", func() bool { return s.Status().Connected })
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "late broker"}})
	receiveText(t, broker, "late broker")
}
func TestRealMQTTMissingPublishAckRecovers(t *testing.T) {
	broker := newWireBroker(t, "127.0.0.1:0")
	s := mqttService(t, broker.listener.Addr().String())
	s.publishTimeout = 150 * time.Millisecond
	s.Start()
	await(t, "initial status acknowledged", func() bool { return s.Status().Published > 0 })
	broker.acknowledge.Store(false)
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "unacknowledged"}})
	receiveText(t, broker, "unacknowledged")
	await(t, "publish timeout recorded", func() bool { return s.Status().PublishErrors > 0 })
	broker.acknowledge.Store(true)
	s.recoverStopped()
	await(t, "stopped client replaced", func() bool { return s.Status().Connected })
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "recovered"}})
	receiveText(t, broker, "recovered")
}

type callbackClient struct {
	mqtt.Client
	open         atomic.Bool
	opts         *mqtt.ClientOptions
	publications atomic.Int64
	disconnects  atomic.Int64
}

func (c *callbackClient) IsConnectionOpen() bool { return c.open.Load() }
func (c *callbackClient) IsConnected() bool      { return c.open.Load() }
func (c *callbackClient) Connect() mqtt.Token    { c.open.Store(true); return completedToken{} }
func (c *callbackClient) Disconnect(uint)        { c.disconnects.Add(1); c.open.Store(false) }
func (c *callbackClient) Publish(string, byte, bool, interface{}) mqtt.Token {
	c.publications.Add(1)
	return completedToken{}
}
func TestDelayedLossCallbackCannotStopFuturePublishing(t *testing.T) {
	s := mqttService(t, "127.0.0.1:1")
	var clients []*callbackClient
	s.newClient = func(opts *mqtt.ClientOptions) mqtt.Client {
		c := &callbackClient{opts: opts}
		clients = append(clients, c)
		return c
	}
	s.Reload()
	first := clients[0]
	first.opts.OnConnect(first)
	// Reproduce Paho's possible callback order: connect completed, then the
	// goroutine for the preceding connection loss finally runs.
	first.opts.OnConnectionLost(first, errors.New("old connection lost"))
	if !s.Status().Connected {
		t.Fatal("late callback overrode live transport", s.Status())
	}
	s.publish("message", 1, false, MessagePayload{Text: "still works"})
	if first.publications.Load() != 2 {
		t.Fatal("new message was blocked")
	}
	s.Reload()
	second := clients[1]
	second.opts.OnConnect(second)
	first.opts.OnConnectionLost(first, errors.New("stale client error"))
	first.opts.OnConnect(first)
	if !s.Status().Connected || strings.Contains(s.Status().LastError, "stale") {
		t.Fatal(s.Status())
	}
	s.Close()
	first.opts.OnConnect(first)
	second.opts.OnConnect(second)
	if s.Status().Connected {
		t.Fatal("callback resurrected closed client")
	}
}

func TestRealMQTTSilentConnectionDetectedByKeepalive(t *testing.T) {
	broker := newWireBroker(t, "127.0.0.1:0")
	s := mqttService(t, broker.listener.Addr().String())
	s.Start()
	await(t, "initial status acknowledged", func() bool { return s.Status().Published > 0 })
	broker.silent.Store(true)
	await(t, "keepalive detects an unresponsive peer", func() bool { return s.Status().LastDisconnectedAt != "" })
	broker.silent.Store(false)
	await(t, "automatic reconnect after keepalive loss", func() bool { return s.Status().Connected })
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ChatID: "c", Body: "after silent loss"}})
	receiveText(t, broker, "after silent loss")
}
func TestCloseDuringInitialRetryAndSecretRedaction(t *testing.T) {
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	reservation.Close()
	s := mqttService(t, address)
	s.Start()
	await(t, "initial retry", func() bool { return s.Status().LastError != "" })
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close blocked during retry")
	}
	if s.Status().Connected {
		t.Fatal(s.Status())
	}
	if reason := redactError(errors.New("tcp://user:password@host: connection refused"), []string{"password"}); strings.Contains(reason, "password") {
		t.Fatal(reason)
	}
}
