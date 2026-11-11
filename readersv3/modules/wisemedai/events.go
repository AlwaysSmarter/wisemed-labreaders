package wisemedai

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

func (m *Module) notify(ctx context.Context, s Session, event string) {
	p, ok := apibridge.PrincipalFrom(ctx)
	if !ok || !m.cfg.WSMIdentity || p.ConnectionID == "" {
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
	// Payload carries no patient data, transcript, clinical result or API token.
	if e := publisher.PublishEventToConnection(p.ConnectionID, map[string]interface{}{"event": "wisemed-ai." + event, "instance_id": m.rt.ReaderID(), "session_id": s.ID, "revision": s.Revision, "status": s.Status, "result_path": "/api/wisemed-ai/v1/sessions/" + s.ID + "/result"}); e != nil {
		m.rt.Logf("wisemed-ai WSM event delivery unavailable")
	}
}
func applyContext(t Template, context map[string]any, p principal) Template {
	if raw, ok := context["presentation"]; ok {
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &t.Presentation)
	}
	t.HTML = strings.ReplaceAll(t.HTML, "{{patient_ref}}", html.EscapeString(fmt.Sprint(context["patient_ref"])))
	t.HTML = strings.ReplaceAll(t.HTML, "{{doctor_name}}", html.EscapeString(p.Name))
	return t
}
