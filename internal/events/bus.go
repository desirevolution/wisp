package events

import "sync"

type Event struct {
	Type string
	Data any
}
type Bus struct {
	mu       sync.RWMutex
	next     int
	subs     map[int]chan Event
	reliable map[int]*mailbox
}

// A reliable subscriber buffers bursts in memory without blocking publishers or
// losing events when an SSE subscriber is slow. Pending events are not durable.
type mailbox struct {
	mu    sync.Mutex
	queue []Event
	wake  chan struct{}
	stop  chan struct{}
	out   chan Event
	kind  string
}

func New() *Bus { return &Bus{subs: map[int]chan Event{}, reliable: map[int]*mailbox{}} }
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, c := range b.subs {
		select {
		case c <- e:
		default:
		}
	}
	for _, m := range b.reliable {
		if e.Type != m.kind {
			continue
		}
		m.mu.Lock()
		m.queue = append(m.queue, e)
		m.mu.Unlock()
		select {
		case m.wake <- struct{}{}:
		default:
		}
	}
}
func (b *Bus) Subscribe() (int, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	c := make(chan Event, 64)
	b.subs[b.next] = c
	return b.next, c
}
func (b *Bus) SubscribeReliable(kind string) (int, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	m := &mailbox{wake: make(chan struct{}, 1), stop: make(chan struct{}), out: make(chan Event), kind: kind}
	b.reliable[b.next] = m
	go m.run()
	return b.next, m.out
}
func (m *mailbox) run() {
	defer close(m.out)
	for {
		select {
		case <-m.stop:
			return
		default:
		}
		m.mu.Lock()
		if len(m.queue) == 0 {
			m.queue = nil
			m.mu.Unlock()
			select {
			case <-m.stop:
				return
			case <-m.wake:
			}
			continue
		}
		e := m.queue[0]
		m.queue[0] = Event{}
		m.queue = m.queue[1:]
		m.mu.Unlock()
		select {
		case <-m.stop:
			return
		case m.out <- e:
		}
	}
}
func (b *Bus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c := b.subs[id]; c != nil {
		delete(b.subs, id)
		close(c)
	}
	if m := b.reliable[id]; m != nil {
		delete(b.reliable, id)
		close(m.stop)
	}
}
