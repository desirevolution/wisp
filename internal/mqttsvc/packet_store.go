package mqttsvc

import (
	"bytes"
	"log"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/eclipse/paho.mqtt.golang/packets"
)

// Paho's default MemoryStore returns shared packet pointers. During reconnect,
// resume changes a stored PublishPacket's DUP flag while the writer may already
// be serializing that same packet (including new publishes from OnConnect).
// Store encoded snapshots instead, and decode a fresh packet for every reader.
// Like the default store, this is in memory only and lasts for one client.
type snapshotPacketStore struct {
	mu     sync.Mutex
	data   map[string][]byte
	opened bool
}

var _ mqtt.Store = (*snapshotPacketStore)(nil)

func newSnapshotPacketStore() *snapshotPacketStore {
	return &snapshotPacketStore{data: make(map[string][]byte)}
}
func (s *snapshotPacketStore) Open()  { s.mu.Lock(); defer s.mu.Unlock(); s.opened = true }
func (s *snapshotPacketStore) Close() { s.mu.Lock(); defer s.mu.Unlock(); s.opened = false }
func (s *snapshotPacketStore) Put(key string, packet packets.ControlPacket) {
	// Paho calls Put before handing the packet to the outgoing writer.
	var encoded bytes.Buffer
	if err := packet.Write(&encoded); err != nil {
		log.Printf("[mqtt] event=packet_store_failed operation=encode error=%q", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opened {
		s.data[key] = encoded.Bytes()
	}
}
func (s *snapshotPacketStore) Get(key string) packets.ControlPacket {
	s.mu.Lock()
	if !s.opened {
		s.mu.Unlock()
		return nil
	}
	encoded := s.data[key]
	s.mu.Unlock()
	if encoded == nil {
		return nil
	}
	packet, err := packets.ReadPacket(bytes.NewReader(encoded))
	if err != nil {
		log.Printf("[mqtt] event=packet_store_failed operation=decode error=%q", err)
		return nil
	}
	return packet
}
func (s *snapshotPacketStore) All() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened {
		return nil
	}
	keys := make([]string, 0, len(s.data))
	for key := range s.data {
		keys = append(keys, key)
	}
	return keys
}
func (s *snapshotPacketStore) Del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opened {
		delete(s.data, key)
	}
}
func (s *snapshotPacketStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string][]byte)
}
