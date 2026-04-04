package sse

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type        string `json:"type"`
	ExecutionID string `json:"execution_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Payload     any    `json:"payload,omitempty"`
}

func (e Event) JSON() []byte {
	b, _ := json.Marshal(e)
	return b
}

type Broadcaster struct {
	mu      sync.RWMutex
	clients map[string]chan Event
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[string]chan Event),
	}
}

func (b *Broadcaster) Subscribe(clientID string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, 64)
	b.clients[clientID] = ch
	return ch
}

func (b *Broadcaster) Unsubscribe(clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.clients[clientID]; ok {
		close(ch)
		delete(b.clients, clientID)
	}
}

func (b *Broadcaster) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Skip slow consumers
		}
	}
}
