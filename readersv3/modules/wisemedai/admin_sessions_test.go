package wisemedai

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestPostgresAdministrativeSessionHistoryScopesAndResults(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	password, _ := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	login := func(email string) (string, map[string]any) {
		code, out := request(t, m, "", "POST", "auth/login", map[string]any{"email": email, "password": string(password)})
		if code != 200 {
			t.Fatal(code, out)
		}
		return out["token"].(string), out["profile"].(map[string]any)
	}
	admin, profile := login(m.cfg.BootstrapEmail)
	tenant, actor := profile["tenant_id"].(string), profile["user_id"].(string)
	otherTenant := uuid.NewString()
	if _, e := m.db.Exec(ctx, `INSERT INTO wai_organizations(id,name)VALUES($1,'Alt cabinet sintetic')`, otherTenant); e != nil {
		t.Fatal(e)
	}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.seed(ctx, tx, otherTenant); e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	user := func(email, role, organization string) (string, string) {
		id := uuid.NewString()
		if _, e := m.db.Exec(ctx, `INSERT INTO wai_users(id,email,password_hash,name) SELECT $1,$2,password_hash,$2 FROM wai_users WHERE id=$3`, id, email, actor); e != nil {
			t.Fatal(e)
		}
		if _, e := m.db.Exec(ctx, `INSERT INTO wai_memberships(id,tenant_id,user_id,role)VALUES($1,$2,$3,$4)`, uuid.NewString(), organization, id, role); e != nil {
			t.Fatal(e)
		}
		token, _ := login(email)
		return id, token
	}
	doctor, doctorToken := user("doctor-history@example.invalid", "doctor", tenant)
	_, organizationAdmin := user("org-history@example.invalid", "admin", tenant)
	otherDoctor, _ := user("other-history@example.invalid", "doctor", otherTenant)
	create := func(owner, organization, status string, withResult bool) string {
		id := uuid.NewString()
		var version string
		if e := m.db.QueryRow(ctx, `SELECT id FROM wai_template_versions WHERE tenant_id=$1 ORDER BY version LIMIT 1`, organization).Scan(&version); e != nil {
			t.Fatal(e)
		}
		context, e := m.box.seal(map[string]any{"patient_ref": "SYNTHETIC"}, organization+":"+id)
		if e != nil {
			t.Fatal(e)
		}
		var result []byte
		if withResult {
			result, e = m.box.seal(Result{SessionID: id, HTML: "<p>Rezultat sintetic</p>", Data: map[string]any{"observație": "sintetică"}}, organization+":"+id)
			if e != nil {
				t.Fatal(e)
			}
		}
		if _, e := m.db.Exec(ctx, `INSERT INTO wai_sessions(id,tenant_id,user_id,template_version,status,context,result)VALUES($1,$2,$3,$4,$5,$6,$7)`, id, organization, owner, version, status, context, result); e != nil {
			t.Fatal(e)
		}
		chunk := uuid.NewString()
		text, e := m.box.seal("dictare sintetică", organization+":"+id+":"+chunk)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := m.db.Exec(ctx, `INSERT INTO wai_session_chunks(id,tenant_id,session_id,sequence,revision,is_final,timestamp,text,digest)VALUES($1,$2,$3,1,1,true,now(),$4,$5)`, chunk, organization, id, text, digest("synthetic")); e != nil {
			t.Fatal(e)
		}
		return id
	}
	create(actor, tenant, "recording", false)
	doctorSession := create(doctor, tenant, "recording", false)
	foreign := create(otherDoctor, otherTenant, "approved", true)
	for i := 0; i < 50; i++ {
		create(doctor, tenant, "recording", false)
	}
	code, list := request(t, m, admin, "GET", "admin/sessions", nil)
	if code != 200 || list["total"] != float64(53) || len(list["items"].([]any)) != 50 || list["instance_scope"] != true {
		t.Fatal(code, list)
	}
	code, list = request(t, m, admin, "GET", "admin/sessions?offset=50", nil)
	if code != 200 || len(list["items"].([]any)) != 3 {
		t.Fatal(code, list)
	}
	code, list = request(t, m, admin, "GET", "admin/sessions?user_id="+otherDoctor+"&status=approved", nil)
	if code != 200 || list["total"] != float64(1) || list["items"].([]any)[0].(map[string]any)["has_result"] != true {
		t.Fatal(code, list)
	}
	code, detail := request(t, m, admin, "GET", "admin/sessions/"+foreign, nil)
	if code != 200 || detail["owner"].(map[string]any)["user_id"] != otherDoctor || detail["session"].(map[string]any)["result"].(map[string]any)["rendered_html"] != "<p>Rezultat sintetic</p>" {
		t.Fatal(code, detail)
	}
	if detail["session"].(map[string]any)["chunks"].([]any)[0].(map[string]any)["text"] != "dictare sintetică" {
		t.Fatal("encrypted transcript not recovered", detail)
	}
	var reads int
	if e := m.db.QueryRow(ctx, `SELECT count(*) FROM wai_audit_events WHERE action='session.admin.read' AND user_id=$1 AND entity_id=$2`, actor, foreign).Scan(&reads); e != nil || reads != 1 {
		t.Fatal("missing administrative read audit", e, reads)
	}
	code, list = request(t, m, organizationAdmin, "GET", "admin/sessions", nil)
	if code != 200 || list["total"] != float64(52) || list["instance_scope"] != false {
		t.Fatal(code, list)
	}
	for _, path := range []string{"admin/sessions/" + foreign, "sessions/" + doctorSession} {
		if code, out := request(t, m, organizationAdmin, "GET", path, nil); code != 404 {
			t.Fatal("scope escaped", path, code, out)
		}
	}
	if code, out := request(t, m, organizationAdmin, "GET", "admin/sessions/"+doctorSession, nil); code != 200 {
		t.Fatal(code, out)
	}
	for _, path := range []string{"admin/sessions", "admin/sessions/" + doctorSession} {
		if code, out := request(t, m, doctorToken, "GET", path, nil); code != 403 {
			t.Fatal("doctor gained admin history", code, out)
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		if code, out := request(t, m, admin, method, "admin/sessions/"+foreign, map[string]any{}); code != 405 {
			t.Fatal("administrative mutation allowed", method, code, out)
		}
	}
	if code, out := request(t, m, admin, "GET", "sessions/"+foreign, nil); code != 404 {
		t.Fatal("normal ownership changed", code, out)
	}
	for _, query := range []string{"?status=bad", "?user_id=bad", "?offset=-1"} {
		if code, out := request(t, m, admin, "GET", "admin/sessions"+query, nil); code != 422 {
			t.Fatal("invalid filter accepted", code, out)
		}
	}
}
