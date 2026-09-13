package realtime

import "sync"

// Event represents one realtime message published to subscribers.
type Event struct {
	Type string
	Data any
}

// Broker manages in-process realtime subscriptions.
//
// The first CONNECT realtime implementation is intentionally
// process-local. PostgreSQL remains the durable source of truth.
// A distributed broker can replace this implementation later
// without changing the HTTP streaming contract.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(
			map[string]map[chan Event]struct{},
		),
	}
}

// Subscribe registers a subscriber for a topic.
//
// The returned function must be called when the subscriber
// disconnects so the broker does not retain stale channels.
func (b *Broker) Subscribe(
	topic string,
) (<-chan Event, func()) {
	ch := make(chan Event, 16)

	b.mu.Lock()

	if b.subscribers[topic] == nil {
		b.subscribers[topic] =
			make(map[chan Event]struct{})
	}

	b.subscribers[topic][ch] = struct{}{}

	b.mu.Unlock()

	var once sync.Once

	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			subscribers := b.subscribers[topic]
			if subscribers == nil {
				return
			}

			delete(subscribers, ch)

			if len(subscribers) == 0 {
				delete(b.subscribers, topic)
			}
		})
	}

	return ch, unsubscribe
}

// Publish sends an event to the current subscribers of a topic.
//
// Publication is deliberately non-blocking. A slow or disconnected
// client must never block the driver's operational request path.
func (b *Broker) Publish(
	topic string,
	event Event,
) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers[topic] {
		select {
		case ch <- event:
		default:
			// Drop the realtime notification for a slow subscriber.
			//
			// The durable state remains available from PostgreSQL,
			// so clients can recover through the normal read API.
		}
	}
}
