package wisemedai

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"io/fs"
	"os"
	"strings"
)

var errInvalidDatabase = errors.New("WiseMED AI: PostgreSQL unavailable; configure WISEMED_AI_DATABASE_URL")

// Migrations are serialized across processes and never touch existing WiseMED tables.
func (m *Module) migrate(ctx context.Context) error {
	tx, e := m.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7741921103)`); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS wai_schema_migrations(version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); e != nil {
		return e
	}
	names, e := fs.Glob(migrations, "migrations/*.sql")
	if e != nil {
		return e
	}
	for _, n := range names {
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_schema_migrations WHERE version=$1)`, n).Scan(&exists); e != nil {
			return e
		}
		if exists {
			continue
		}
		b, _ := migrations.ReadFile(n)
		if _, e = tx.Exec(ctx, string(b)); e != nil {
			return fmt.Errorf("migration %s: %w", n, e)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO wai_schema_migrations(version)VALUES($1)`, n); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (m *Module) bootstrap(ctx context.Context) error {
	tx, e := m.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7741921104)`); e != nil {
		return e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM wai_users`).Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	raw, e := secretFile(m.rt.ResolvePath(m.cfg.BootstrapPasswordFile), 18)
	if e != nil {
		return e
	}
	// A generated secret is only stored in the protected file; never logged.
	password := fmt.Sprintf("%x", raw)
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tenant, user := uuid.NewString(), uuid.NewString()
	if _, e = tx.Exec(ctx, `INSERT INTO wai_organizations(id,name) VALUES($1,'Cabinet demonstrativ')`, tenant); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO wai_users(id,email,password_hash,name,instance_admin) VALUES($1,$2,$3,'Administrator',true)`, user, strings.ToLower(m.cfg.BootstrapEmail), string(hash)); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO wai_memberships(id,tenant_id,user_id,role)VALUES($1,$2,$3,'admin')`, uuid.NewString(), tenant, user); e != nil {
		return e
	}
	if e = m.seed(ctx, tx, tenant); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func audit(ctx context.Context, tx pgx.Tx, p principal, action, id string) error {
	_, e := tx.Exec(ctx, `INSERT INTO wai_audit_events(id,tenant_id,user_id,action,entity_id)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), p.Tenant, p.User, action, id)
	return e
}
func (m *Module) retain(ctx context.Context) {
	_, _ = m.db.Exec(ctx, `DELETE FROM wai_browser_speech WHERE expires_at<now()`)
	m.retainSpeech(ctx)
	_, e := m.db.Exec(ctx, `DELETE FROM wai_sessions WHERE updated_at < now()-make_interval(days=>$1)`, m.cfg.RetentionDays)
	if e != nil {
		m.rt.Logf("wisemed-ai retention failed")
	}
	if _, e = m.db.Exec(ctx, `UPDATE wai_usage_records SET encrypted_prompt=NULL, encrypted_output=NULL WHERE encrypted_prompt IS NOT NULL AND created_at < now()-make_interval(days=>$1)`, m.cfg.RetentionDays); e != nil {
		m.rt.Logf("wisemed-ai prompt retention failed")
	}
	_, _ = m.db.Exec(ctx, `DELETE FROM wai_auth_sessions WHERE expires_at<now(); DELETE FROM wai_pairing_tokens WHERE expires_at<now()`)
}
func envKey(name string) bool { return os.Getenv(name) != "" }
