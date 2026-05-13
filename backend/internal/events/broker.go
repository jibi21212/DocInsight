package events

import (
	"encoding/json"
	"sync"
)

// Event represents a server-sent event.
type Event struct {
	Type string      `json:"type"` // e.g. "document.completed", "document.failed"
	Data interface{} `json:"data"`
}

// Broker manages SSE subscribers and publishes events to all of them.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]chan Event
}

// NewBroker creates a new event broker.
func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[string]chan Event),
	}
}

// Subscribe registers a new client and returns a channel to receive events.
func (b *Broker) Subscribe(clientID string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 16)
	b.subscribers[clientID] = ch
	return ch
}

// Unsubscribe removes a client and closes its channel.
func (b *Broker) Unsubscribe(clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[clientID]; ok {
		close(ch)
		delete(b.subscribers, clientID)
	}
}

// Publish sends an event to all subscribers. Non-blocking — drops events
// for slow consumers.
func (b *Broker) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Slow consumer, drop the event
		}
	}
}

// FormatSSE encodes an Event as an SSE-compatible string.
func FormatSSE(event Event) (string, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	return "data: " + string(data) + "\n\n", nil
}
