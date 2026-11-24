package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Observers are transient, bounded, and never store traffic on disk.
type trafficObserver struct {
	tenant   string
	filters  []string
	messages chan []byte
	overflow chan struct{}
}

var trafficTypes = []string{"command", "reply", "event", "presence"}

func (h *Hub) observe(tenant string, filters []string) (*trafficObserver, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.observers == nil {
		h.observers = map[*trafficObserver]bool{}
	}
	n := 0
	for o := range h.observers {
		if o.tenant == tenant {
			n++
		}
	}
	if n >= 8 {
		return nil, false
	}
	o := &trafficObserver{tenant, filters, make(chan []byte, 32), make(chan struct{})}
	h.observers[o] = true
	return o, true
}
func (h *Hub) unobserve(o *trafficObserver) { h.mu.Lock(); delete(h.observers, o); h.mu.Unlock() }
func (h *Hub) publish(tenant string, msg Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.observers) == 0 {
		return
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if len(data) > 65536 {
		msg.Payload = map[string]interface{}{"truncated": true, "bytes": len(data), "sender_client_id": msg.Payload["sender_client_id"], "sender_client_type": msg.Payload["sender_client_type"], "sender_reader_id": msg.Payload["sender_reader_id"]}
		data, _ = json.Marshal(msg)
	}
	for o := range h.observers {
		if o.tenant != tenant {
			continue
		}
		match := slices.Contains(o.filters, msg.Type) || slices.Contains(o.filters, "all")
		if msg.Target != nil && msg.Target.Mode == "topic" {
			match = match || slices.Contains(o.filters, "topic:"+msg.Target.Topic)
		}
		if !match {
			continue
		}
		select {
		case <-o.overflow:
			continue
		default:
		}
		select {
		case o.messages <- data:
		default:
			close(o.overflow)
		}
	}
}
func (s *Server) adminClients(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]interface{}{"clients": s.hub.Snapshot(sess.TenantID)})
}
func (s *Server) adminClientAction(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := s.requireAdmin(w, r, true)
	if !ok {
		return
	}
	var in struct {
		ID      string `json:"id"`
		Action  string `json:"action"`
		Message string `json:"message"`
	}
	if !decodeAdmin(w, r, &in) {
		return
	}
	if !slices.Contains([]string{"kick", "reconnect", "message"}, in.Action) || (in.Action == "message" && (strings.TrimSpace(in.Message) == "" || len(in.Message) > 4096)) {
		adminError(w, 400, "invalid action/message (maximum 4096 bytes)")
		return
	}
	s.hub.mu.RLock()
	c := s.hub.connections[in.ID]
	if c != nil && c.TenantID != sess.TenantID {
		c = nil
	}
	s.hub.mu.RUnlock()
	if c == nil {
		adminError(w, 404, "client not connected")
		return
	}
	if in.Action == "kick" {
		c.Close()
		writeJSON(w, 200, map[string]interface{}{"closed": true})
		return
	}
	if in.Action == "reconnect" && c.ClientType != "reader" && !isSIUIServiceClientType(c.ClientType) {
		adminError(w, 400, "reconnect requires a reader/utility supporting ws.reconnect; use kick to close other clients")
		return
	}
	id, _ := randomAdminToken()
	command := "debug.message"
	if in.Action == "reconnect" {
		command = "ws.reconnect"
	}
	sender := &Connection{ConnectionInfo: ConnectionInfo{ID: "admin-console", TenantID: sess.TenantID}}
	msg := Envelope{Type: "command", RequestID: id, ConnectionID: sender.ID, Target: &Target{Mode: "connection", ConnectionID: in.ID}, Payload: map[string]interface{}{"command": command, "args": map[string]interface{}{"message": in.Message}, "sender_connection_id": sender.ID, "sender_client_id": "admin-console", "sender_client_type": "service", "sender_subject": sess.Username, "sender_tenant_id": sess.TenantID, "sender_scopes": []string{"route:command", "devices:debug"}}}
	n := s.hub.Route(msg, sender)
	if n == 0 {
		adminError(w, 409, "client disconnected or queue full")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"request_id": id, "delivery": "queued"})
}
func (s *Server) adminMessageCatalog(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	filters := append([]string{"all"}, trafficTypes...)
	for _, c := range s.hub.Snapshot(sess.TenantID) {
		for _, topic := range c.Topics {
			f := "topic:" + topic
			if !slices.Contains(filters, f) {
				filters = append(filters, f)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]interface{}{"messages": filters})
}
func (s *Server) adminMessageStream(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := s.requireAdmin(w, r, false)
	if !ok {
		return
	}
	filters := strings.Split(r.URL.Query().Get("filters"), ",")
	if len(filters) > 32 {
		adminError(w, 400, "too many filters")
		return
	}
	for _, f := range filters {
		if f != "all" && !slices.Contains(trafficTypes, f) && !(strings.HasPrefix(f, "topic:") && len(f) > 6 && len(f) <= 134) {
			adminError(w, 400, "invalid message filter")
			return
		}
	}
	o, ok := s.hub.observe(sess.TenantID, filters)
	if !ok {
		adminError(w, 429, "too many live feeds")
		return
	}
	defer s.hub.unobserve(o)
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(event string, data []byte) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		if err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send("ready", []byte(`{}`)) {
		return
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-o.overflow:
			send("gap", []byte(`{}`))
			return
		case data := <-o.messages:
			current, valid := s.findAdminSession(r)
			if !valid || current.TenantID != sess.TenantID {
				send("expired", []byte(`{}`))
				return
			}
			if !send("traffic", data) {
				return
			}
		case <-tick.C:
			if _, valid := s.findAdminSession(r); !valid {
				send("expired", []byte(`{}`))
				return
			}
			if !send("heartbeat", []byte(`{}`)) {
				return
			}
		}
	}
}
