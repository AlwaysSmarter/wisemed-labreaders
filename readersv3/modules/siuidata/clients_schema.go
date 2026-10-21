package siuidata

// Confidential provider data never enters public catalogue tables.
const clientSchema = `
CREATE TABLE IF NOT EXISTS siui_client_snapshots(
 id uuid PRIMARY KEY,cui text NOT NULL,category text NOT NULL,provider_code text NOT NULL,
 provider_name text NOT NULL,provider jsonb NOT NULL,effective_at timestamptz NOT NULL,
 imported_at timestamptz NOT NULL DEFAULT now(),source_filename text NOT NULL,sha256 text NOT NULL,
 statistics jsonb NOT NULL,UNIQUE(cui,category,provider_code,effective_at)
);
CREATE INDEX IF NOT EXISTS siui_client_snapshot_lookup ON siui_client_snapshots(cui,category,provider_code,effective_at DESC);
CREATE INDEX IF NOT EXISTS siui_client_snapshot_name ON siui_client_snapshots USING gin((siui_data_drug_fold(provider_name)) gin_trgm_ops);
CREATE TABLE IF NOT EXISTS siui_client_contracts(
 snapshot_id uuid NOT NULL REFERENCES siui_client_snapshots(id) ON DELETE CASCADE,
 ordinal integer NOT NULL,category text NOT NULL,contract_no text NOT NULL,
 valid_from date NOT NULL,valid_to date,document jsonb NOT NULL,PRIMARY KEY(snapshot_id,ordinal),
 CHECK(valid_to IS NULL OR valid_to>=valid_from)
);
CREATE INDEX IF NOT EXISTS siui_client_contract_period ON siui_client_contracts(snapshot_id,category,valid_from,valid_to);
CREATE TABLE IF NOT EXISTS siui_client_employees(
 snapshot_id uuid NOT NULL REFERENCES siui_client_snapshots(id) ON DELETE CASCADE,
 ordinal integer NOT NULL,stencil text NOT NULL,name text NOT NULL,valid_from date NOT NULL,valid_to date,
 attributes jsonb NOT NULL,PRIMARY KEY(snapshot_id,ordinal),CHECK(valid_to IS NULL OR valid_to>=valid_from)
);
CREATE TABLE IF NOT EXISTS siui_client_specialties(
 snapshot_id uuid NOT NULL,employee_ordinal integer NOT NULL,ordinal integer NOT NULL,
 code text NOT NULL,valid_from date NOT NULL,valid_to date,attributes jsonb NOT NULL,
 PRIMARY KEY(snapshot_id,employee_ordinal,ordinal),FOREIGN KEY(snapshot_id,employee_ordinal) REFERENCES siui_client_employees(snapshot_id,ordinal) ON DELETE CASCADE,
 CHECK(valid_to IS NULL OR valid_to>=valid_from)
);
CREATE INDEX IF NOT EXISTS siui_client_specialty_snapshot ON siui_client_specialties(snapshot_id,code);
INSERT INTO siui_data_schema_version(version) VALUES(4) ON CONFLICT DO NOTHING;
`
