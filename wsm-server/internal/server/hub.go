package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Identity fields are immutable after Register; activity/topics use the hub lock.
type ConnectionInfo struct {
	EquipmentID string    `json:"equipment_id,omitempty"`
	RemoteIP    string    `json:"remote_ip,omitempty"`
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	ClientType  string    `json:"client_type"`
	ClientID    string    `json:"client_id"`
	Subject     string    `json:"subject"`
	ReaderID    string    `json:"reader_id,omitempty"`
	Label       string    `json:"label,omitempty"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	QueueDepth  int       `json:"queue_depth"`
	Topics      []string  `json:"topics,omitempty"`
}
type Connection struct {
	ConnectionInfo
	conn      *websocket.Conn
	send      chan Envelope
	closed    chan struct{}
	closeOnce sync.Once
	cancel    context.CancelFunc
}
type HubStats struct {
	TotalAccepted uint64 `json:"total_accepted"`
	TotalClosed   uint64 `json:"total_closed"`
	TotalDropped  uint64 `json:"total_dropped"`
}
type Hub struct {
	mu          sync.RWMutex
	connections map[string]*Connection
	stats       map[string]HubStats
}

func NewHub() *Hub { return &Hub{connections: map[string]*Connection{}, stats: map[string]HubStats{}} }
func (h *Hub) NewConnection(ws *websocket.Conn, tenant string, queue int) *Connection {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		panic(err)
	}
	now := time.Now().UTC()
	return &Connection{ConnectionInfo: ConnectionInfo{ID: hex.EncodeToString(id), TenantID: tenant, ConnectedAt: now, LastSeenAt: now}, conn: ws, send: make(chan Envelope, queue), closed: make(chan struct{})}
}
func (h *Hub) Register(c *Connection, hello HelloPayload) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.connections[c.ID]; ok {
		return false
	}
	// One reader identity per tenant avoids ambiguous execution of 1:1 commands.
	for _, other := range h.connections {
		if c.TenantID == other.TenantID && hello.ClientType == "reader" && (other.ReaderID == hello.ReaderID || c.EquipmentID != "" && other.EquipmentID == c.EquipmentID) {
			return false
		}
	}
	c.ClientType = hello.ClientType
	c.ClientID = hello.ClientID
	c.ReaderID = hello.ReaderID
	c.Label = hello.Label
	h.connections[c.ID] = c
	st := h.stats[c.TenantID]
	st.TotalAccepted++
	h.stats[c.TenantID] = st
	return true
}
func (h *Hub) Remove(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.connections[id]; ok {
		delete(h.connections, id)
		st := h.stats[c.TenantID]
		st.TotalClosed++
		h.stats[c.TenantID] = st
		c.Close()
	}
}
func (h *Hub) Touch(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c := h.connections[id]; c != nil {
		c.LastSeenAt = time.Now().UTC()
	}
}
func (h *Hub) Snapshot(tenant string) []ConnectionInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := []ConnectionInfo{}
	for _, c := range h.connections {
		if c.TenantID == tenant {
			v := c.ConnectionInfo
			v.Topics = append([]string(nil), c.Topics...)
			v.QueueDepth = len(c.send)
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b ConnectionInfo) int { return a.ConnectedAt.Compare(b.ConnectedAt) })
	return out
}
func (h *Hub) Stats(tenant string) HubStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.stats[tenant]
}
func (h *Hub) Broadcast(tenant string, msg Envelope) int {
	return h.route(tenant, msg, func(*Connection) bool { return true })
}
func (h *Hub) Route(msg Envelope, sender *Connection) int {
	if sender == nil || msg.Target == nil {
		return 0
	}
	t := msg.Target
	return h.route(sender.TenantID, msg, func(c *Connection) bool {
		switch t.Mode {
		case "all":
			return true
		case "connection":
			return c.ID == t.ConnectionID
		case "connections":
			return slices.Contains(t.ConnectionIDs, c.ID)
		case "equipment":
			return c.EquipmentID != "" && c.EquipmentID == t.EquipmentID
		case "equipments":
			return c.EquipmentID != "" && slices.Contains(t.EquipmentIDs, c.EquipmentID)
		case "reader":
			return c.ReaderID != "" && c.ReaderID == t.ReaderID
		case "readers":
			return c.ReaderID != "" && slices.Contains(t.ReaderIDs, c.ReaderID)
		case "client_type":
			return c.ClientType == t.ClientType
		case "topic":
			return slices.Contains(c.Topics, t.Topic)
		case "self":
			return c.ID == sender.ID
		}
		return false
	})
}
func (h *Hub) route(tenant string, msg Envelope, match func(*Connection) bool) int {
	h.mu.RLock()
	targets := []*Connection{}
	for _, c := range h.connections {
		if c.TenantID == tenant && match(c) {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()
	sent := 0
	for _, c := range targets {
		if h.send(c, msg) {
			sent++
		}
	}
	return sent
}
func (h *Hub) Subscribe(id, topic string, max int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.connections[id]
	if c == nil || topic == "" {
		return false
	}
	if slices.Contains(c.Topics, topic) {
		return true
	}
	if len(c.Topics) >= max {
		return false
	}
	c.Topics = append(c.Topics, topic)
	return true
}
func (h *Hub) Unsubscribe(id, topic string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.connections[id]
	if c == nil {
		return false
	}
	c.Topics = slices.DeleteFunc(c.Topics, func(v string) bool { return v == topic })
	return true
}
func (h *Hub) send(c *Connection, msg Envelope) bool {
	select {
	case <-c.closed:
		return false
	default:
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	select {
	case c.send <- msg:
		return true
	default:
		h.mu.Lock()
		st := h.stats[c.TenantID]
		st.TotalDropped++
		h.stats[c.TenantID] = st
		h.mu.Unlock()
		// A slow peer must reconnect/resynchronize; never silently keep losing traffic.
		c.Close()
		return false
	}
}
func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.cancel != nil {
			c.cancel()
		}
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}
