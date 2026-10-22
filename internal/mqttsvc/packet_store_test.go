package mqttsvc

import (
	"bytes"
	"github.com/eclipse/paho.mqtt.golang/packets"
	"sync"
	"testing"
)

func TestReconnectPacketsDoNotShareMutableFields(t *testing.T) {
	store := newSnapshotPacketStore()
	store.Open()
	defer store.Close()
	original := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	original.Qos = 1
	original.MessageID = 42
	original.TopicName = "wisp/message"
	original.Payload = []byte("image")
	store.Put("o.42", original)
	// Simulate mutations on the outgoing original and on multiple resume copies.
	original.Dup = true
	original.Payload[0] = 'X'
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				p := store.Get("o.42").(*packets.PublishPacket)
				if p.Dup || string(p.Payload) != "image" || p.MessageID != 42 {
					t.Errorf("stored packet mutated: %+v", p)
					return
				}
				p.Dup = true
				p.Payload[0] = 'Y'
				var wire bytes.Buffer
				if err := p.Write(&wire); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
