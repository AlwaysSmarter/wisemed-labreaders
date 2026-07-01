package analyzeractivity

import (
	"strings"
	"sync"
	"time"
)

type Snapshot struct {
	Connected           bool      `json:"connected"`
	ConnectedSessions   int       `json:"connected_sessions"`
	LastPacketAt        time.Time `json:"last_packet_at"`
	LastTransport       string    `json:"last_transport"`
	LastDirection       string    `json:"last_direction"`
	LastConnectionEvent time.Time `json:"last_connection_event"`
}

type Tracker struct {
	mu      sync.Mutex
	clients int
	last    Snapshot
}

func New() *Tracker {
	return &Tracker{}
}

func (t *Tracker) Connected(delta int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clients += delta
	if t.clients < 0 {
		t.clients = 0
	}
	t.last.ConnectedSessions = t.clients
	t.last.Connected = t.clients > 0
	t.last.LastConnectionEvent = time.Now()
}

func (t *Tracker) Packet(direction, transport string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.last.LastPacketAt = now
	t.last.LastTransport = strings.TrimSpace(strings.ToLower(transport))
	t.last.LastDirection = strings.TrimSpace(strings.ToLower(direction))
	if t.clients > 0 {
		t.last.Connected = true
	}
}

func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.last
	out.ConnectedSessions = t.clients
	out.Connected = t.clients > 0
	if !out.Connected && !out.LastPacketAt.IsZero() && time.Since(out.LastPacketAt) <= 5*time.Second {
		out.Connected = true
	}
	return out
}
