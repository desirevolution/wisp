package events

import (
	"testing"
	"time"
)

func TestReliableBurst(t *testing.T) {
	b := New()
	id, ch := b.SubscribeReliable("message")
	defer b.Unsubscribe(id)
	for i := 0; i < 1000; i++ {
		b.Publish(Event{Type: "message", Data: i})
		b.Publish(Event{Type: "media", Data: i})
	}
	for i := 0; i < 1000; i++ {
		select {
		case e := <-ch:
			if e.Type != "message" || e.Data.(int) != i {
				t.Fatalf("out of order: %#v, want %d", e, i)
			}
		case <-time.After(time.Second):
			t.Fatalf("lost event %d", i)
		}
	}
}
func TestUnsubscribeWhileBacklogged(t *testing.T) {
	b := New()
	id, ch := b.SubscribeReliable("message")
	for i := 0; i < 100; i++ {
		b.Publish(Event{Type: "message", Data: i})
	}
	b.Unsubscribe(id)
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not stop")
	}
	b.Publish(Event{Type: "message"})
}
