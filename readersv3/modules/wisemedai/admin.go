package wisemedai

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (m *Module) profile(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method == "GET" {
		respond(w, 200, p)
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		Name            string `json:"name"`
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 180 {
		fail(w, 400, "invalid_name")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	if in.Password != "" {
		if len(in.Password) < 12 || len(in.Password) > 72 {
			fail(w, 400, "password_requires_12_to_72_bytes")
			return
		}
		var hash string
		if tx.QueryRow(r.Context(), `SELECT password_hash FROM wai_users WHERE id=$1`, p.User).Scan(&hash) != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.CurrentPassword)) != nil {
			fail(w, 403, "current_password_required")
			return
		}
		b, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if e != nil {
			fail(w, 400, "invalid_password")
			return
		}
		if _, e = tx.Exec(r.Context(), `UPDATE wai_users SET password_hash=$1 WHERE id=$2`, string(b), p.User); e != nil {
			fail(w, 503, "database_error")
			return
		}
		_, _ = tx.Exec(r.Context(), `DELETE FROM wai_auth_sessions WHERE user_id=$1`, p.User)
	}
	if _, e = tx.Exec(r.Context(), `UPDATE wai_users SET name=$1 WHERE id=$2`, in.Name, p.User); e == nil {
		e = audit(r.Context(), tx, p, "profile.update", p.User)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	p.Name = in.Name
	respond(w, 200, p)
}
func (m *Module) users(w http.ResponseWriter, r *http.Request, p principal) {
	if p.Role != "admin" {
		fail(w, 403, "admin_required")
		return
	}
	if r.Method == "GET" {
		rows, e := m.db.Query(r.Context(), `SELECT u.id,u.email,u.name,m.role,coalesce(m.external_subject,''),m.disabled FROM wai_users u JOIN wai_memberships m ON m.user_id=u.id WHERE m.tenant_id=$1 ORDER BY u.email`, p.Tenant)
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, email, name, role, subject string
			var disabled bool
			if rows.Scan(&id, &email, &name, &role, &subject, &disabled) != nil {
				fail(w, 500, "database_error")
				return
			}
			out = append(out, map[string]any{"id": id, "email": email, "name": name, "role": role, "external_subject": subject, "disabled": disabled})
		}
		respond(w, 200, out)
		return
	}
	if r.Method != "POST" && r.Method != "PUT" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		UserType *int   `json:"user_type"`
		ID       string `json:"id"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
		External string `json:"external_subject"`
		Disabled bool   `json:"disabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.UserType != nil {
		if *in.UserType < -1 || *in.UserType > 1 {
			fail(w, 400, "invalid_user_type")
			return
		}
		if *in.UserType == -1 && !p.InstanceAdmin {
			fail(w, 403, "super_admin_required")
			return
		}
		if *in.UserType <= 0 {
			in.Role = "admin"
		} else {
			in.Role = "doctor"
		}
		if r.Method == "PUT" {
			fail(w, 400, "use_billing_role_endpoint_for_existing_users")
			return
		}
	}
	if (in.Role != "doctor" && in.Role != "admin" && in.Role != "viewer") || in.Name == "" || !strings.Contains(in.Email, "@") || len(in.Email) > 254 {
		fail(w, 400, "invalid_user")
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	if r.Method == "POST" {
		if len(in.Password) < 12 || len(in.Password) > 72 {
			fail(w, 400, "password_requires_12_to_72_bytes")
			return
		}
		in.ID = uuid.NewString()
		hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if e != nil {
			fail(w, 400, "invalid_password")
			return
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO wai_users(id,email,name,password_hash,instance_admin)VALUES($1,$2,$3,$4,$5)`, in.ID, strings.ToLower(in.Email), in.Name, string(hash), in.UserType != nil && *in.UserType == -1)
		if e == nil {
			_, e = tx.Exec(r.Context(), `INSERT INTO wai_memberships(id,tenant_id,user_id,role,external_subject)VALUES($1,$2,$3,$4,NULLIF($5,''))`, uuid.NewString(), p.Tenant, in.ID, in.Role, in.External)
		}
	} else {
		if in.ID == p.User && (in.Disabled || in.Role != "admin") {
			fail(w, 409, "cannot_remove_own_admin_access")
			return
		}
		var exists bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM wai_memberships WHERE tenant_id=$1 AND user_id::text=$2)`, p.Tenant, in.ID).Scan(&exists)
		if e != nil || !exists {
			fail(w, 404, "user_not_found")
			return
		}
		var targetInstance bool
		var oldName string
		if e = tx.QueryRow(r.Context(), `SELECT instance_admin,name FROM wai_users WHERE id=$1`, in.ID).Scan(&targetInstance, &oldName); e != nil {
			fail(w, 404, "user_not_found")
			return
		}
		if targetInstance && !p.InstanceAdmin {
			fail(w, 403, "cannot_modify_instance_admin")
			return
		}
		if in.Name != oldName && !p.InstanceAdmin && in.ID != p.User {
			fail(w, 403, "global_identity_name_requires_owner")
			return
		}
		if in.Password != "" {
			if !p.InstanceAdmin && in.ID != p.User {
				var shared bool
				if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM wai_memberships WHERE user_id=$1 AND tenant_id<>$2)`, in.ID, p.Tenant).Scan(&shared); e != nil {
					fail(w, 503, "database_error")
					return
				}
				if shared {
					fail(w, 403, "shared_user_password_requires_instance_admin")
					return
				}
			}
			if len(in.Password) < 12 || len(in.Password) > 72 {
				fail(w, 400, "password_requires_12_to_72_bytes")
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
			if err != nil {
				fail(w, 400, "invalid_password")
				return
			}
			_, e = tx.Exec(r.Context(), `UPDATE wai_users SET name=$1,password_hash=$2 WHERE id=$3`, in.Name, string(hash), in.ID)
			if e == nil {
				_, e = tx.Exec(r.Context(), `DELETE FROM wai_auth_sessions WHERE user_id=$1`, in.ID)
			}
		} else {
			_, e = tx.Exec(r.Context(), `UPDATE wai_users SET name=$1 WHERE id=$2`, in.Name, in.ID)
		}
		if e == nil {
			_, e = tx.Exec(r.Context(), `UPDATE wai_memberships SET role=$1,external_subject=NULLIF($2,''),disabled=$5 WHERE user_id=$3 AND tenant_id=$4`, in.Role, in.External, in.ID, p.Tenant, in.Disabled)
		}
	}
	if e == nil {
		e = audit(r.Context(), tx, p, "user.save", in.ID)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 409, "user_conflict")
		return
	}
	respond(w, 200, map[string]any{"id": in.ID, "ok": true})
}
func (m *Module) settings(w http.ResponseWriter, r *http.Request, p principal) {
	if !p.InstanceAdmin {
		fail(w, 403, "instance_admin_required")
		return
	}
	if r.Method == "PUT" {
		m.saveSettings(w, r, p)
		return
	}
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	respond(w, 200, map[string]any{"provider": m.cfg.Provider, "model": m.cfg.Model, "openai_key_configured": m.openAIKey() != "", "openai_key_env": m.cfg.OpenAIKeyEnv, "monthly_budget_usd": m.cfg.MonthlyBudget, "input_usd_per_million": m.cfg.InputPrice, "output_usd_per_million": m.cfg.OutputPrice, "retention_days": m.cfg.RetentionDays, "secure_cookies": m.cfg.SecureCookies, "database_configured": m.cfg.DatabaseURL != "", "encryption": "AES-256-GCM", "wisemed_login_enabled": m.cfg.WiseMEDLogin, "wsm_identity_enabled": m.cfg.WSMIdentity, "runtime_settings_url": "/settings/reader", "wsm_settings_url": "/settings/wss", "updates_url": "/settings/reader"})
}
func (m *Module) dashboard(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var sessions, review, approved, templates int
	e := m.db.QueryRow(r.Context(), `SELECT count(*),count(*) FILTER(WHERE status='review'),count(*) FILTER(WHERE status='approved') FROM wai_sessions WHERE tenant_id=$1 AND user_id=$2`, p.Tenant, p.User).Scan(&sessions, &review, &approved)
	if e == nil {
		e = m.db.QueryRow(r.Context(), `SELECT count(*) FROM wai_templates t JOIN wai_template_versions v ON v.id=t.current_version WHERE `+visibleTemplateSQL, p.Tenant, p.User, p.InstanceAdmin).Scan(&templates)
	}
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]any{"sessions": sessions, "review": review, "approved": approved, "templates": templates, "provider": m.cfg.Provider, "retention_days": m.cfg.RetentionDays, "organization_id": p.Tenant})
}
func (m *Module) auditHTTP(w http.ResponseWriter, r *http.Request, p principal) {
	if p.Role != "admin" {
		fail(w, 403, "admin_required")
		return
	}
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	rows, e := m.db.Query(r.Context(), `SELECT id,user_id,action,entity_id,created_at FROM wai_audit_events WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 250`, p.Tenant)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, user, action, entity string
		var at time.Time
		if rows.Scan(&id, &user, &action, &entity, &at) != nil {
			fail(w, 500, "database_error")
			return
		}
		out = append(out, map[string]any{"id": id, "user_id": user, "action": action, "entity_id": entity, "created_at": at})
	}
	respond(w, 200, out)
}
func (m *Module) usage(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Method != "GET" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var input, output int64
	var cost, reserved float64
	var uncertain int64
	e := m.db.QueryRow(r.Context(), `SELECT coalesce(sum(input_tokens),0),coalesce(sum(output_tokens),0),coalesce(sum(cost_usd),0),coalesce(sum(cost_usd) FILTER(WHERE status IN ('reserved','uncertain')),0),count(*) FILTER(WHERE status IN ('reserved','uncertain')) FROM wai_usage_records WHERE tenant_id=$1 AND created_at>=date_trunc('month',now())`, p.Tenant).Scan(&input, &output, &cost, &reserved, &uncertain)
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	respond(w, 200, map[string]any{"input_tokens": input, "output_tokens": output, "cost_usd": cost, "reserved_usd": reserved, "uncertain_requests": uncertain, "budget_usd": m.cfg.MonthlyBudget, "prices_operator_configured": true})
}

var resourceTables = map[string]string{"integrations": "wai_integration_configs", "connector-mappings": "wai_connector_mappings", "memory/preferences": "wai_memory_preferences", "memory/examples": "wai_memory_examples", "memory/feedback": "wai_memory_feedback"}

func (m *Module) resources(w http.ResponseWriter, r *http.Request, p principal, path string) {
	if path == "specialty-packs" || path == "specialties" || path == "organizations" {
		if r.Method != "GET" {
			fail(w, 405, "method_not_allowed")
			return
		}
		query := `SELECT id,name,payload FROM wai_specialty_packs WHERE tenant_id=$1`
		if path == "specialties" {
			query = `SELECT id,name,'{}'::jsonb FROM wai_specialties WHERE tenant_id=$1`
		}
		if path == "organizations" {
			query = `SELECT id,name,'{}'::jsonb FROM wai_organizations WHERE id=$1`
		}
		rows, e := m.db.Query(r.Context(), query, p.Tenant)
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, name string
			var b []byte
			if rows.Scan(&id, &name, &b) != nil {
				fail(w, 500, "database_error")
				return
			}
			var payload any
			_ = json.Unmarshal(b, &payload)
			out = append(out, map[string]any{"id": id, "name": name, "payload": payload})
		}
		respond(w, 200, out)
		return
	}
	table, ok := resourceTables[path]
	if !ok {
		fail(w, 404, "not_found")
		return
	}
	admin := path == "integrations" || path == "connector-mappings"
	if admin && p.Role != "admin" {
		fail(w, 403, "admin_required")
		return
	}
	if r.Method == "GET" {
		rows, e := m.db.Query(r.Context(), `SELECT id,name,payload,created_at FROM `+table+` WHERE tenant_id=$1 AND ($2 OR user_id=$3) ORDER BY created_at DESC LIMIT 200`, p.Tenant, admin, p.User)
		if e != nil {
			fail(w, 503, "database_error")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, name string
			var b []byte
			var at time.Time
			if rows.Scan(&id, &name, &b, &at) != nil {
				fail(w, 500, "database_error")
				return
			}
			var payload map[string]any
			if m.box.open(b, p.Tenant+":"+id, &payload) != nil {
				fail(w, 500, "decryption_failed")
				return
			}
			out = append(out, map[string]any{"id": id, "name": name, "payload": redact(payload), "created_at": at})
		}
		respond(w, 200, out)
		return
	}
	if r.Method != "POST" && r.Method != "DELETE" {
		fail(w, 405, "method_not_allowed")
		return
	}
	var in struct {
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Payload   map[string]any `json:"payload"`
		Confirmed bool           `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	tx, e := m.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "database_error")
		return
	}
	defer tx.Rollback(r.Context())
	if r.Method == "DELETE" {
		_, e = tx.Exec(r.Context(), `DELETE FROM `+table+` WHERE id::text=$1 AND tenant_id=$2 AND ($3 OR user_id=$4)`, in.ID, p.Tenant, admin, p.User)
	} else {
		if !in.Confirmed || len(in.Name) < 2 || len(in.Name) > 180 {
			fail(w, 400, "explicit_confirmation_and_name_required")
			return
		}
		if path == "integrations" {
			if e = validateConnector(in.Payload, m.cfg.AllowedHosts); e != nil {
				fail(w, 400, e.Error())
				return
			}
		}
		in.ID = uuid.NewString()
		b, err := m.box.seal(in.Payload, p.Tenant+":"+in.ID)
		if err != nil {
			fail(w, 500, "encryption_error")
			return
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO `+table+`(id,tenant_id,user_id,name,payload)VALUES($1,$2,$3,$4,$5)`, in.ID, p.Tenant, p.User, in.Name, b)
	}
	if e == nil {
		e = audit(r.Context(), tx, p, path+".save", in.ID)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 409, "resource_conflict")
		return
	}
	respond(w, 200, map[string]any{"id": in.ID, "ok": true})
}
func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		o := map[string]any{}
		for k, v := range x {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "api_key") || strings.Contains(lower, "authorization") {
				o[k+"_configured"] = v != "" && v != nil
			} else {
				o[k] = redact(v)
			}
		}
		return o
	case []any:
		o := []any{}
		for _, v := range x {
			o = append(o, redact(v))
		}
		return o
	default:
		return v
	}
}
