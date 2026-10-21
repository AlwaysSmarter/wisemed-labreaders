package siuidata

const physicianSchema = `
ALTER TABLE siui_data_imports DROP CONSTRAINT IF EXISTS siui_data_imports_catalogue_type_check;
ALTER TABLE siui_data_imports ADD CONSTRAINT siui_data_imports_catalogue_type_check CHECK(catalogue_type IN ('CLIN','PARA','PHM','PHYS'));
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS physician_count bigint NOT NULL DEFAULT 0;
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS source_dates jsonb NOT NULL DEFAULT '{}';
ALTER TABLE siui_data_records ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT '';
DROP INDEX IF EXISTS siui_data_entity_unique;
CREATE UNIQUE INDEX siui_data_entity_unique ON siui_data_records(import_id,source,kind,code)
 WHERE active AND kind IN ('ClinicService','ClinicServiceGroup','Speciality','MedSrvPack','LaboratoriesService');
CREATE UNIQUE INDEX IF NOT EXISTS siui_data_physician_entity_unique ON siui_data_records(import_id,source,kind,code)
 WHERE active AND source<>'' AND kind IN ('Physician','InsuranceHouse','InsuranceHouseType');
CREATE INDEX IF NOT EXISTS siui_data_physician_raw ON siui_data_records(import_id,source,kind,code) WHERE active;
CREATE TABLE IF NOT EXISTS siui_data_physicians(
 import_id uuid NOT NULL REFERENCES siui_data_imports(id) ON DELETE CASCADE,
 source text NOT NULL,stencil text NOT NULL,name text NOT NULL,name_normalized text NOT NULL,
 attributes jsonb NOT NULL,document jsonb NOT NULL,PRIMARY KEY(import_id,source,stencil)
);
CREATE INDEX IF NOT EXISTS siui_data_physician_name ON siui_data_physicians USING gin(name_normalized gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_physician_search ON siui_data_physicians USING gin((siui_data_drug_fold(name||' '||stencil)) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_physician_stencil ON siui_data_physicians(stencil,import_id);
CREATE TABLE IF NOT EXISTS siui_data_physician_relations(
 import_id uuid NOT NULL,source text NOT NULL,stencil text NOT NULL,ordinal bigint NOT NULL,
 attributes jsonb NOT NULL,document jsonb NOT NULL,specialty_code text NOT NULL,specialty_name text NOT NULL,
 contract_type text NOT NULL,contract_no text NOT NULL,physician_type text NOT NULL,insurance_house text NOT NULL,insurance_house_name text NOT NULL,
 PRIMARY KEY(import_id,source,ordinal),FOREIGN KEY(import_id,source,stencil) REFERENCES siui_data_physicians(import_id,source,stencil) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_stencil ON siui_data_physician_relations(import_id,source,stencil);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_specialty ON siui_data_physician_relations(import_id,specialty_code,stencil);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_house ON siui_data_physician_relations(import_id,insurance_house,stencil);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_contract_no ON siui_data_physician_relations(import_id,contract_no,stencil);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_type ON siui_data_physician_relations(import_id,physician_type,stencil);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_contract ON siui_data_physician_relations(import_id,contract_type,stencil);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_specialty_name ON siui_data_physician_relations USING gin(specialty_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_physician_relation_attributes ON siui_data_physician_relations USING gin(attributes jsonb_path_ops);
INSERT INTO siui_data_schema_version(version) VALUES(3) ON CONFLICT DO NOTHING;
`
