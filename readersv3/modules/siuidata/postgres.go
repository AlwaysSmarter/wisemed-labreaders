package siuidata

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = `
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE IF NOT EXISTS siui_data_imports (
 id uuid PRIMARY KEY, catalogue_type text NOT NULL CHECK(catalogue_type IN ('CLIN','PARA')),
 status text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz,
 as_of date NOT NULL, source_sha256 text NOT NULL DEFAULT '', record_count bigint NOT NULL DEFAULT 0,
 service_count bigint NOT NULL DEFAULT 0, error text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS siui_data_imports_type ON siui_data_imports(catalogue_type,created_at DESC);
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS source_date date;
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS source_filename text NOT NULL DEFAULT '';
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'upload';
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS statistics jsonb NOT NULL DEFAULT '{}';
CREATE TABLE IF NOT EXISTS siui_data_active (
 catalogue_type text PRIMARY KEY, import_id uuid NOT NULL REFERENCES siui_data_imports(id)
);
CREATE TABLE IF NOT EXISTS siui_data_records (
 import_id uuid NOT NULL REFERENCES siui_data_imports(id) ON DELETE CASCADE,
 ordinal bigint NOT NULL, kind text NOT NULL, container text NOT NULL, code text NOT NULL,
 name_normalized text NOT NULL, attributes jsonb NOT NULL, payload jsonb NOT NULL,
 valid_from date, valid_to date, active boolean NOT NULL, PRIMARY KEY(import_id,ordinal)
);
CREATE INDEX IF NOT EXISTS siui_data_record_lookup ON siui_data_records(import_id,kind,code);
CREATE UNIQUE INDEX IF NOT EXISTS siui_data_entity_unique ON siui_data_records(import_id,kind,code)
 WHERE active AND kind IN ('ClinicService','ClinicServiceGroup','Speciality','MedSrvPack','LaboratoriesService');
CREATE INDEX IF NOT EXISTS siui_data_record_relation ON siui_data_records(import_id,kind,(attributes->>'service'),(attributes->>'serviceGroup')) WHERE active;
CREATE INDEX IF NOT EXISTS siui_data_record_name ON siui_data_records USING gin(name_normalized gin_trgm_ops) WHERE active;
CREATE INDEX IF NOT EXISTS siui_data_record_attributes ON siui_data_records USING gin(attributes jsonb_path_ops) WHERE active;
CREATE TABLE IF NOT EXISTS siui_data_services (
 import_id uuid NOT NULL REFERENCES siui_data_imports(id) ON DELETE CASCADE, code text NOT NULL,
 name text NOT NULL, search_text text NOT NULL, attributes jsonb NOT NULL, document jsonb NOT NULL,
 specialty_codes text[] NOT NULL, specialty_names text NOT NULL, package_codes text[] NOT NULL,
 PRIMARY KEY(import_id,code)
);
CREATE INDEX IF NOT EXISTS siui_data_service_text ON siui_data_services USING gin(search_text gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_service_attributes ON siui_data_services USING gin(attributes jsonb_path_ops);
CREATE INDEX IF NOT EXISTS siui_data_service_specialties ON siui_data_services USING gin(specialty_codes);
CREATE INDEX IF NOT EXISTS siui_data_service_packages ON siui_data_services USING gin(package_codes);
CREATE TABLE IF NOT EXISTS siui_data_schema_version(version integer PRIMARY KEY);
INSERT INTO siui_data_schema_version(version) VALUES(1) ON CONFLICT DO NOTHING;
`

func connectPostgres(ctx context.Context, c settings) (*pgxpool.Pool, error) {
	if c.PGDatabase == "" || c.PGUser == "" {
		return nil, errors.New("configure PostgreSQL database and user in SIUI Data setup")
	}
	u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(c.PGHost, strconv.Itoa(c.PGPort)), Path: "/" + c.PGDatabase, User: url.UserPassword(c.PGUser, c.PGPassword)}
	q := url.Values{"sslmode": {c.PGSSLMode}, "connect_timeout": {"5"}, "application_name": {"wisemed-siui-data"}}
	if c.PGRootCert != "" {
		q.Set("sslrootcert", c.PGRootCert)
	}
	u.RawQuery = q.Encode()
	cfg, e := pgxpool.ParseConfig(u.String())
	if e != nil {
		return nil, errors.New("invalid PostgreSQL connection settings")
	}
	cfg.MaxConns = int32(c.PGMaxConns)
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000"
	cfg.MaxConnLifetime = 30 * time.Minute
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return nil, errors.New("cannot initialize PostgreSQL pool")
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, errors.New("PostgreSQL unavailable; check host, database, credentials and TLS")
	}
	e = ensurePostgresSchema(ctx, p)
	if e != nil {
		p.Close()
		return nil, fmt.Errorf("initialize SIUI PostgreSQL schema (pg_trgm and table permissions required): %w", e)
	}
	// A crashed process loses its session lock. Only then may an unfinished job
	// be marked failed; jobs owned by another live instance remain untouched.
	conn, e := p.Acquire(ctx)
	if e != nil {
		p.Close()
		return nil, errors.New("PostgreSQL unavailable")
	}
	for _, entry := range []struct {
		typ string
		key int32
	}{{"CLIN", 1}, {"PARA", 2}, {"PHM", 3}, {"PHYS", 4}} {
		var locked bool
		if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(1936288117,$1)`, entry.key).Scan(&locked); e != nil {
			break
		}
		if !locked {
			continue
		}
		_, e = conn.Exec(ctx, `UPDATE siui_data_imports SET status='failed',finished_at=now(),error='worker interrupted before activation' WHERE catalogue_type=$1 AND status IN ('queued','parsing','downloading')`, entry.typ)
		var released bool
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		unlockErr := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock(1936288117,$1)`, entry.key).Scan(&released)
		if unlockErr != nil || !released {
			conn.Conn().Close(unlockCtx)
		}
		cancel()
		if e != nil || unlockErr != nil {
			break
		}
	}
	conn.Release()
	if e != nil {
		p.Close()
		return nil, errors.New("cannot recover interrupted SIUI imports")
	}
	return p, nil
}

const latestSchemaVersion = 4

func readSchemaVersion(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (int, error) {
	var exists bool
	if e := q.QueryRow(ctx, `SELECT to_regclass('siui_data_schema_version') IS NOT NULL`).Scan(&exists); e != nil || !exists {
		return 0, e
	}
	var version int
	if e := q.QueryRow(ctx, `SELECT COALESCE(max(version),0) FROM siui_data_schema_version`).Scan(&version); e != nil {
		return 0, e
	}
	if version > latestSchemaVersion {
		return 0, errors.New("PostgreSQL SIUI schema is newer than this application")
	}
	return version, nil
}

func ensurePostgresSchema(ctx context.Context, p *pgxpool.Pool) error {
	version, e := readSchemaVersion(ctx, p)
	if e != nil || version == latestSchemaVersion {
		return e
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1936288117,0)`); e != nil {
		return e
	}
	// Another instance may have migrated while this transaction waited.
	version, e = readSchemaVersion(ctx, tx)
	if e != nil {
		return e
	}
	// Completed migrations must not be replayed: older versions temporarily
	// restrict catalogue_type and cannot validate newer imports such as PHYS.
	for i, migration := range []string{schema, drugSchema, physicianSchema, clientSchema} {
		if i+1 <= version {
			continue
		}
		if _, e = tx.Exec(ctx, migration); e != nil {
			return fmt.Errorf("migrate SIUI schema to version %d: %w", i+1, e)
		}
	}
	return tx.Commit(ctx)
}

func (m *Module) database(ctx context.Context) (*pgxpool.Pool, error) {
	m.dbMu.Lock()
	defer m.dbMu.Unlock()
	if m.db != nil {
		return m.db, nil
	}
	m.cfgMu.RLock()
	c := m.cfg
	m.cfgMu.RUnlock()
	if c.PGRootCert != "" {
		c.PGRootCert = m.rt.ResolvePath(c.PGRootCert)
	}
	var e error
	m.db, e = connectPostgres(ctx, c)
	return m.db, e
}

// Rendering is set based in PostgreSQL. No full catalogue or relationship map is
// held in Go, and each resulting service is already assembled for readers.
func buildServices(ctx context.Context, tx pgx.Tx, id, typ string) (int64, error) {
	model := models[typ]
	if model.Service == "" {
		return 0, nil
	}
	for _, check := range []struct {
		query, message string
		args           []any
	}{
		{`SELECT EXISTS(SELECT 1 FROM siui_data_records s WHERE s.import_id=$1 AND s.active AND s.kind=$2 AND COALESCE(s.attributes->>'serviceGroup','')<>'' AND NOT EXISTS(SELECT 1 FROM siui_data_records g WHERE g.import_id=s.import_id AND g.active AND g.kind=$3 AND g.code=s.attributes->>'serviceGroup'))`, "active service references missing group", []any{id, model.Service, model.Group}},
		{`SELECT EXISTS(SELECT 1 FROM siui_data_records a JOIN siui_data_records g ON g.import_id=a.import_id AND g.active AND g.kind=$3 AND g.code=a.attributes->>'serviceGroup' WHERE a.import_id=$1 AND a.active AND a.kind=$2 AND NOT EXISTS(SELECT 1 FROM siui_data_records p WHERE p.import_id=a.import_id AND p.active AND p.kind='Speciality' AND p.code=a.attributes->>'specialty'))`, "active group association references missing specialty", []any{id, model.GroupAssociation, model.Group}},
		{`SELECT EXISTS(SELECT 1 FROM siui_data_records a JOIN siui_data_records s ON s.import_id=a.import_id AND s.active AND s.kind=$3 AND s.code=a.attributes->>'service' WHERE a.import_id=$1 AND a.active AND a.kind=$2 AND NOT EXISTS(SELECT 1 FROM siui_data_records p WHERE p.import_id=a.import_id AND p.active AND p.kind='MedSrvPack' AND p.code=a.attributes->>'medSrvPack'))`, "active service association references missing package", []any{id, model.PackageAssociation, model.Service}},
		{`SELECT EXISTS(SELECT 1 FROM siui_data_records WHERE import_id=$1 AND active AND kind=$2 GROUP BY attributes->>'service',attributes->>'medSrvPack' HAVING count(*)>1)`, "overlapping active package association", []any{id, model.PackageAssociation}},
		{`SELECT EXISTS(SELECT 1 FROM siui_data_records WHERE import_id=$1 AND active AND kind=$2 GROUP BY attributes->>'serviceGroup',attributes->>'specialty' HAVING count(*)>1)`, "overlapping active specialty association", []any{id, model.GroupAssociation}},
	} {
		var invalid bool
		if e := tx.QueryRow(ctx, check.query, check.args...).Scan(&invalid); e != nil {
			return 0, e
		}
		if invalid {
			return 0, errors.New(check.message)
		}
	}
	result, e := tx.Exec(ctx, `
WITH specialties AS (
 SELECT a.attributes->>'serviceGroup' AS group_code,
 jsonb_agg(p.attributes || jsonb_build_object('association',a.attributes,'associationValidFrom',a.attributes->>'validFrom','associationValidTo',a.attributes->>'validTo') || (SELECT jsonb_object_agg('association.'||key,value) FROM jsonb_each(a.attributes)) ORDER BY p.code) AS details,
 array_agg(p.code ORDER BY p.code) AS codes, string_agg(p.name_normalized,' ') AS names
 FROM siui_data_records a JOIN siui_data_records p ON p.import_id=a.import_id AND p.active AND p.kind='Speciality' AND p.code=a.attributes->>'specialty'
 WHERE a.import_id=$1 AND a.active AND a.kind=$4 GROUP BY a.attributes->>'serviceGroup'
), packages AS (
 SELECT a.attributes->>'service' AS service_code,
 jsonb_agg(jsonb_build_object('attributes',a.attributes,'package',p.attributes) ORDER BY p.code) AS details,
 array_agg(p.code ORDER BY p.code) AS codes
 FROM siui_data_records a JOIN siui_data_records p ON p.import_id=a.import_id AND p.active AND p.kind='MedSrvPack' AND p.code=a.attributes->>'medSrvPack'
 WHERE a.import_id=$1 AND a.active AND a.kind=$5 GROUP BY a.attributes->>'service'
)
INSERT INTO siui_data_services(import_id,code,name,search_text,attributes,document,specialty_codes,specialty_names,package_codes)
SELECT s.import_id,s.code,s.attributes->>'name',
 lower(s.code)||' '||s.name_normalized||' '||COALESCE(g.name_normalized,'')||' '||COALESCE(sp.names,''), s.attributes,
 jsonb_build_object('code',s.code,'name',s.attributes->>'name','kind',s.kind,'type',$6::text,
 'attributes',s.attributes,'group',g.attributes,'specialties',COALESCE(sp.details,'[]'::jsonb),
 'packages',COALESCE(pk.details,'[]'::jsonb),
 'isExam',s.attributes->>'isExam' IN ('true','1'),'opRoom',s.attributes->>'opRoom' IN ('true','1'),
 'isConnectedService',s.attributes->>'isConnectedService' IN ('true','1'),
 'flags',jsonb_strip_nulls(jsonb_build_object(
 'isExam',s.attributes->>'isExam','opRoom',s.attributes->>'opRoom','isConnectedService',s.attributes->>'isConnectedService',
 'highLevel',s.attributes->>'highLevel','recByPC',s.attributes->>'recByPC','formula',s.attributes->>'formula','inClinic',s.attributes->>'inClinic'))),
 COALESCE(sp.codes,ARRAY[]::text[]),COALESCE(sp.names,''),COALESCE(pk.codes,ARRAY[]::text[])
FROM siui_data_records s
LEFT JOIN siui_data_records g ON g.import_id=s.import_id AND g.active AND g.kind=$3 AND g.code=s.attributes->>'serviceGroup'
LEFT JOIN specialties sp ON sp.group_code=s.attributes->>'serviceGroup'
LEFT JOIN packages pk ON pk.service_code=s.code
WHERE s.import_id=$1 AND s.active AND s.kind=$2`, id, model.Service, model.Group, model.GroupAssociation, model.PackageAssociation, typ)
	if e != nil {
		return 0, e
	}
	if result.RowsAffected() == 0 {
		return 0, errors.New("catalogue contains no active services of the requested type")
	}
	return result.RowsAffected(), nil
}
