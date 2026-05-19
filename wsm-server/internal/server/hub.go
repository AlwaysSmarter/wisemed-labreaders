package server

import (
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Connection struct {
	ID          string    `json:"id"`
	ClientType  string    `json:"client_type"`
	ClientID    string    `json:"client_id"`
	Subject     string    `json:"subject,omitempty"`
	Role        string    `json:"role,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	ReaderID    string    `json:"reader_id,omitempty"`
	Label       string    `json:"label,omitempty"`
	RemoteAddr  string    `json:"remote_addr"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	QueueDepth  int       `json:"queue_depth"`
	Topics      []string  `json:"topics,omitempty"`

	conn      *websocket.Conn
	send      chan Envelope
	closed    chan struct{}
	closeOnce sync.Once
}

type Hub struct {
	mu          sync.RWMutex
	connections map[string]*Connection
	topics      map[string]map[string]*Connection
	seq         uint64
	stats       HubStats
}

type HubStats struct {
	TotalAccepted uint64 `json:"total_accepted"`
	TotalClosed   uint64 `json:"total_closed"`
	TotalDropped  uint64 `json:"total_dropped"`
}

func NewHub() *Hub {
	return &Hub{connections: map[string]*Connection{}, topics: map[string]map[string]*Connection{}}
}

func (h *Hub) NewConnection(ws *websocket.Conn, remoteAddr string, sendQueueSize int) *Connection {
	now := time.Now().UTC()
	id := atomic.AddUint64(&h.seq, 1)
	return &Connection{
		ID:          "conn-" + itoa(int(id)),
		RemoteAddr:  remoteAddr,
		ConnectedAt: now,
		LastSeenAt:  now,
		conn:        ws,
		send:        make(chan Envelope, sendQueueSize),
		closed:      make(chan struct{}),
	}
}

func (h *Hub) Register(conn *Connection, hello HelloPayload) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn.ClientType = hello.ClientType
	conn.ClientID = hello.ClientID
	conn.UserID = hello.UserID
	conn.ReaderID = hello.ReaderID
	conn.Label = hello.Label
	conn.LastSeenAt = time.Now().UTC()
	h.connections[conn.ID] = conn
	h.stats.TotalAccepted++
}

func (h *Hub) Remove(connectionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.connections[connectionID]; ok {
		h.removeConnectionTopicsLocked(conn)
		conn.Close()
		delete(h.connections, connectionID)
		h.stats.TotalClosed++
	}
}

func (h *Hub) Touch(connectionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.connections[connectionID]; ok {
		conn.LastSeenAt = time.Now().UTC()
	}
}

func (h *Hub) Snapshot() []Connection {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make([]Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		out = append(out, Connection{
			ID:          conn.ID,
			ClientType:  conn.ClientType,
			ClientID:    conn.ClientID,
			Subject:     conn.Subject,
			Role:        conn.Role,
			UserID:      conn.UserID,
			ReaderID:    conn.ReaderID,
			Label:       conn.Label,
			RemoteAddr:  conn.RemoteAddr,
			ConnectedAt: conn.ConnectedAt,
			LastSeenAt:  conn.LastSeenAt,
			QueueDepth:  len(conn.send),
			Topics:      append([]string(nil), conn.Topics...),
		})
	}
	slices.SortFunc(out, func(a, b Connection) int {
		switch {
		case a.ConnectedAt.Before(b.ConnectedAt):
			return -1
		case a.ConnectedAt.After(b.ConnectedAt):
			return 1
		default:
			return 0
		}
	})
	return out
}

func (h *Hub) Stats() HubStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.stats
}

func (h *Hub) SendToConnection(connectionID string, msg Envelope) bool {
	h.mu.RLock()
	conn, ok := h.connections[connectionID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	return h.send(conn, msg)
}

func (h *Hub) Broadcast(msg Envelope) int {
	h.mu.RLock()
	targets := make([]*Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		targets = append(targets, conn)
	}
	h.mu.RUnlock()

	sent := 0
	for _, conn := range targets {
		if h.send(conn, msg) {
			sent++
		}
	}
	return sent
}

func (h *Hub) Route(msg Envelope, sender *Connection) int {
	if msg.Broadcast || msg.Target == nil || msg.Target.Mode == "all" {
		return h.Broadcast(msg)
	}

	switch msg.Target.Mode {
	case "connection":
		if h.SendToConnection(msg.Target.ConnectionID, msg) {
			return 1
		}
	case "reader":
		return h.sendWhere(msg, func(conn *Connection) bool {
			return conn.ReaderID != "" && conn.ReaderID == msg.Target.ReaderID
		})
	case "topic":
		return h.sendToTopic(msg.Target.Topic, msg)
	case "client_type":
		return h.sendWhere(msg, func(conn *Connection) bool {
			return conn.ClientType == msg.Target.ClientType
		})
	case "self":
		if sender != nil && h.SendToConnection(sender.ID, msg) {
			return 1
		}
	}

	return 0
}

func (h *Hub) Subscribe(connectionID string, topic string) bool {
	if topic == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	conn, ok := h.connections[connectionID]
	if !ok {
		return false
	}
	if h.topics[topic] == nil {
		h.topics[topic] = map[string]*Connection{}
	}
	h.topics[topic][connectionID] = conn
	if !slices.Contains(conn.Topics, topic) {
		conn.Topics = append(conn.Topics, topic)
	}
	return true
}

func (h *Hub) Unsubscribe(connectionID string, topic string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn, ok := h.connections[connectionID]
	if !ok {
		return false
	}
	if subscribers, ok := h.topics[topic]; ok {
		delete(subscribers, connectionID)
		if len(subscribers) == 0 {
			delete(h.topics, topic)
		}
	}
	filtered := make([]string, 0, len(conn.Topics))
	for _, item := range conn.Topics {
		if item != topic {
			filtered = append(filtered, item)
		}
	}
	conn.Topics = filtered
	return true
}

func (h *Hub) sendToTopic(topic string, msg Envelope) int {
	if topic == "" {
		return 0
	}
	h.mu.RLock()
	subscribers := h.topics[topic]
	targets := make([]*Connection, 0, len(subscribers))
	for _, conn := range subscribers {
		targets = append(targets, conn)
	}
	h.mu.RUnlock()

	sent := 0
	for _, conn := range targets {
		if h.send(conn, msg) {
			sent++
		}
	}
	return sent
}

func (h *Hub) removeConnectionTopicsLocked(conn *Connection) {
	for _, topic := range conn.Topics {
		if subscribers, ok := h.topics[topic]; ok {
			delete(subscribers, conn.ID)
			if len(subscribers) == 0 {
				delete(h.topics, topic)
			}
		}
	}
	conn.Topics = nil
}

func (h *Hub) sendWhere(msg Envelope, predicate func(*Connection) bool) int {
	h.mu.RLock()
	targets := make([]*Connection, 0)
	for _, conn := range h.connections {
		if predicate(conn) {
			targets = append(targets, conn)
		}
	}
	h.mu.RUnlock()

	sent := 0
	for _, conn := range targets {
		if h.send(conn, msg) {
			sent++
		}
	}
	return sent
}

func (h *Hub) send(conn *Connection, msg Envelope) bool {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	select {
	case conn.send <- msg:
		return true
	default:
		h.mu.Lock()
		h.stats.TotalDropped++
		h.mu.Unlock()
		log.Printf("dropping message for %s because send queue is full", conn.ID)
		return false
	}
}

func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}
