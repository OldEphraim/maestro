package sse_test

import (
	"sync"
	"testing"
	"time"

	"github.com/oldephraim/maestro/backend/internal/sse"
)

func TestSubscribePublishReceive(t *testing.T) {
	b := sse.NewBroadcaster()

	ch := b.Subscribe("client-1")

	event := sse.Event{Type: "test", ExecutionID: "exec-1", Payload: "hello"}
	b.Publish(event)

	select {
	case received := <-ch:
		if received.Type != "test" {
			t.Fatalf("expected type 'test', got %q", received.Type)
		}
		if received.ExecutionID != "exec-1" {
			t.Fatalf("expected execution_id 'exec-1', got %q", received.ExecutionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestUnsubscribeCleansUp(t *testing.T) {
	b := sse.NewBroadcaster()

	ch := b.Subscribe("client-2")
	b.Unsubscribe("client-2")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}

	// Publishing after unsubscribe should not panic
	b.Publish(sse.Event{Type: "after-unsub"})
}

func TestMultipleSubscribers(t *testing.T) {
	b := sse.NewBroadcaster()

	ch1 := b.Subscribe("client-a")
	ch2 := b.Subscribe("client-b")

	b.Publish(sse.Event{Type: "broadcast"})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		select {
		case e := <-ch1:
			if e.Type != "broadcast" {
				t.Errorf("client-a: expected 'broadcast', got %q", e.Type)
			}
		case <-time.After(time.Second):
			t.Error("client-a: timeout")
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case e := <-ch2:
			if e.Type != "broadcast" {
				t.Errorf("client-b: expected 'broadcast', got %q", e.Type)
			}
		case <-time.After(time.Second):
			t.Error("client-b: timeout")
		}
	}()

	wg.Wait()

	b.Unsubscribe("client-a")
	b.Unsubscribe("client-b")
}

func TestSlowConsumerSkipped(t *testing.T) {
	b := sse.NewBroadcaster()

	ch := b.Subscribe("slow-client")

	// Fill the buffer (capacity 64)
	for i := 0; i < 70; i++ {
		b.Publish(sse.Event{Type: "flood"})
	}

	// Should still be able to read buffered events
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
	if count != 64 {
		t.Fatalf("expected 64 buffered events, got %d", count)
	}

	b.Unsubscribe("slow-client")
}
