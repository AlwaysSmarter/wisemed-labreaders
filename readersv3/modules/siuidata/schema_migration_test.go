package siuidata

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestClientSchemaUpgradePreservesPhysicianImports(t *testing.T) {
	cfg := testPostgresSettings(t)
	m, s := newTestModule(t, cfg)
	id := upload(t, s, "PHYS", bytes.NewReader(physicianArchive(t)))
	awaitImport(t, s, id, "ready")
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	// Recreate a v3 installation with active PHYS data, including dictionary
	// codes shared by CNAS and OPSNAJ. Replaying v1/v2 would break this data.
	if _, e = db.Exec(context.Background(), `DROP TABLE siui_client_specialties; DROP TABLE siui_client_employees; DROP TABLE siui_client_contracts; DROP TABLE siui_client_snapshots; DELETE FROM siui_data_schema_version WHERE version=4`); e != nil {
		t.Fatal(e)
	}
	_, replica := newTestModule(t, cfg)
	status, data := apiCall(t, replica, "GET", prefix+"physicians", "", nil)
	if status != 200 || !bytes.Contains(data, []byte(`"totalCount":4`)) {
		t.Fatalf("v3 to v4 lost PHYS access: %d %s", status, data)
	}
	var activeID, constraint string
	if e = db.QueryRow(context.Background(), `SELECT import_id::text FROM siui_data_active WHERE catalogue_type='PHYS'`).Scan(&activeID); e != nil || activeID != id {
		t.Fatalf("active generation changed: %s %v", activeID, e)
	}
	if e = db.QueryRow(context.Background(), `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='siui_data_imports'::regclass AND conname='siui_data_imports_catalogue_type_check'`).Scan(&constraint); e != nil || !strings.Contains(constraint, "PHYS") {
		t.Fatalf("lost PHYS constraint: %s %v", constraint, e)
	}
	version, e := readSchemaVersion(context.Background(), db)
	if e != nil || version != latestSchemaVersion {
		t.Fatalf("schema version: %d %v", version, e)
	}
	var clients int
	if e = db.QueryRow(context.Background(), `SELECT count(*) FROM siui_client_snapshots`).Scan(&clients); e != nil {
		t.Fatalf("client schema missing: %v", e)
	}
	// Reconnecting to an already-upgraded database must remain a no-op.
	if e = ensurePostgresSchema(context.Background(), db); e != nil {
		t.Fatal(e)
	}
}
