package events

import (
	"testing"
	"time"
)

func TestBroker_SubscribeAndPublish(t *testing.T) {
	b := NewBroker()
	ch := b.Subscribe("client1")

	b.Publish(Event{Type: "test.event", Data: map[string]string{"key": "value"}})

	select {
	case event := <-ch:
		if event.Type != "test.event" {
			t.Errorf("event type = %q, want 'test.event'", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	b := NewBroker()
	ch1 := b.Subscribe("client1")
	ch2 := b.Subscribe("client2")

	b.Publish(Event{Type: "broadcast", Data: nil})

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case event := <-ch:
			if event.Type != "broadcast" {
				t.Errorf("event type = %q, want 'broadcast'", event.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
}

func TestBroker_Unsubscribe(t *testing.T) {
	b := NewBroker()
	ch := b.Subscribe("client1")
	b.Unsubscribe("client1")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected closed channel after unsubscribe")
	}

	// Publishing after unsubscribe should not panic
	b.Publish(Event{Type: "after.unsub", Data: nil})
}

func TestBroker_UnsubscribeNonexistent(t *testing.T) {
	b := NewBroker()
	// Should not panic
	b.Unsubscribe("nobody")
}

func TestBroker_SlowConsumerDrops(t *testing.T) {
	b := NewBroker()
	ch := b.Subscribe("slow")

	// Fill the channel buffer (capacity is 16)
	for i := 0; i < 20; i++ {
		b.Publish(Event{Type: "flood", Data: i})
	}

	// Should have received up to buffer capacity
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != 16 {
		t.Errorf("expected 16 events (buffer size), got %d", count)
	}
}

func TestFormatSSE(t *testing.T) {
	event := Event{Type: "test", Data: map[string]string{"msg": "hello"}}
	result, err := FormatSSE(event)
	if err != nil {
		t.Fatalf("FormatSSE: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty SSE output")
	}
	if result[len(result)-2:] != "\n\n" {
		t.Error("SSE message should end with double newline")
	}
	if result[:6] != "data: " {
		t.Error("SSE message should start with 'data: '")
	}
}
