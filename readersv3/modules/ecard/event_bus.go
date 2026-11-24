package ecard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

type eventSubscription struct {
	Owner, Job, Connection string
	Hardware               bool
	Expires                time.Time
}

// Durable event IDs are replay cursors. Live WSM delivery is best-effort;
// disconnected clients recover from /events or the retained operation report.
type eventBus struct {
	mu            sync.Mutex
	subscriptions map[string]eventSubscription
}

func (m *Module) subscribeEvents(w http.ResponseWriter, r *http.Request, owner string) {
	p, remote := apibridge.PrincipalFrom(r.Context())
	if !remote || p.ConnectionID == "" {
		fail(w, 400, errors.New("WSM subscription requires an attested connection; local clients use GET events"))
		return
	}
	var in struct {
		SubscriptionID string `json:"subscription_id,omitempty"`
		Job            string `json:"job_id,omitempty"`
		Hardware       bool   `json:"hardware,omitempty"`
	}
	if r.Method != "POST" && r.Method != "DELETE" {
		fail(w, 405, errors.New("use POST or DELETE"))
		return
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err)
		return
	}
	if in.Job != "" {
		if _, err := m.store.job(in.Job, owner); err != nil {
			fail(w, 404, errors.New("job not found"))
			return
		}
	}
	m.bus.mu.Lock()
	defer m.bus.mu.Unlock()
	if r.Method == "DELETE" {
		s, ok := m.bus.subscriptions[in.SubscriptionID]
		if !ok || s.Owner != owner || s.Connection != p.ConnectionID {
			fail(w, 404, errors.New("subscription not found"))
			return
		}
		delete(m.bus.subscriptions, in.SubscriptionID)
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if m.bus.subscriptions == nil {
		m.bus.subscriptions = map[string]eventSubscription{}
	}
	now := time.Now()
	count := 0
	for id, s := range m.bus.subscriptions {
		if !s.Expires.After(now) {
			delete(m.bus.subscriptions, id)
		} else if s.Owner == owner {
			count++
		}
	}
	if count >= 32 && in.SubscriptionID == "" {
		fail(w, 429, errors.New("too many event subscriptions; remove or renew an existing subscription"))
		return
	}
	id := uuid.NewString()
	if in.SubscriptionID != "" {
		s, ok := m.bus.subscriptions[in.SubscriptionID]
		if !ok || s.Owner != owner || s.Connection != p.ConnectionID {
			fail(w, 404, errors.New("subscription not found"))
			return
		}
		id = in.SubscriptionID
	}
	expires := now.Add(30 * time.Minute)
	m.bus.subscriptions[id] = eventSubscription{Owner: owner, Job: in.Job, Hardware: in.Hardware, Connection: p.ConnectionID, Expires: expires}
	respond(w, 200, map[string]interface{}{"subscription_id": id, "expires_at": expires.UTC(), "event": "ecard.event", "replay_path": "/api/ecard/events", "report_path_template": "/api/ecard/jobs/{id}/report"})
}

// The existing utility logging setting controls both local live logs and WSM.
func (m *Module) verboseEvents() bool {
	if m.rt == nil {
		return false
	}
	level, _ := strconv.Atoi(fmt.Sprint(m.rt.ModuleSettings("logging")["verbose_level"]))
	return level >= 5
}

func minimalLiveEvent(event map[string]interface{}) bool {
	if event["source"] == "pcsc" {
		job, _ := event["job"].(string)
		return job == ""
	}
	switch event["event"] {
	case "pin_required", "operation_finished":
		return true
	}
	return false
}

func (m *Module) publishEvent(owner string, event map[string]interface{}) {
	if m.rt == nil {
		return
	}
	verbose := m.verboseEvents()
	if !verbose && !minimalLiveEvent(event) {
		return
	}
	svc, ok := m.rt.Service("wisemed-ws-client")
	if !ok {
		return
	}
	publisher, ok := svc.(interface {
		PublishEventToConnection(string, map[string]interface{}) error
	})
	if !ok {
		return
	}
	job, _ := event["job"].(string)
	m.bus.mu.Lock()
	connections := map[string]bool{}
	now := time.Now()
	for id, s := range m.bus.subscriptions {
		if !s.Expires.After(now) {
			delete(m.bus.subscriptions, id)
			continue
		}
		if owner == "" {
			if s.Hardware {
				connections[s.Connection] = true
			}
		} else if s.Owner == owner && (s.Job == "" || s.Job == job) {
			connections[s.Connection] = true
		}
	}
	m.bus.mu.Unlock()
	for connection := range connections {
		payload := map[string]interface{}{"event": "ecard.event", "type": event["type"], "subtype": event["subtype"], "instance_id": m.rt.ReaderID(), "data": event}
		if job != "" {
			payload["report_path"] = "/api/ecard/jobs/" + job + "/report"
		}
		if event["event"] == "pin_required" && job != "" {
			// Fetch the current, single-use challenge via the authenticated API;
			// never persist it in the event history or replay an expired prompt.
			payload["status_path"] = "/api/ecard/jobs/" + job
			payload["reply_path"] = "/api/ecard/jobs/" + job + "/pin"
			payload["reply_method"] = "POST"
		}
		if verbose && event["event"] == "operation_finished" {
			if j, err := m.store.job(job, owner); err == nil {
				if report, err := m.operationReport(owner, j); err == nil {
					report = compactOperationReport(report)
					if raw, err := json.Marshal(report); err == nil && len(raw) < 3<<20 {
						payload["operation_report"] = report
					} else {
						payload["report_delivery"] = "chunked"
						payload["report_parts_path"] = "/api/ecard/jobs/" + job + "/report?part=0"
					}
				}
			}
		}
		if err := publisher.PublishEventToConnection(connection, payload); err != nil {
			m.rt.Logf("ecard: WSM event delivery unavailable; recover via event cursor/report")
		}
	}
}
