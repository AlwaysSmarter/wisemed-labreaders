package siuidata

const drugSchema = `
ALTER TABLE siui_data_imports DROP CONSTRAINT IF EXISTS siui_data_imports_catalogue_type_check;
ALTER TABLE siui_data_imports ADD CONSTRAINT siui_data_imports_catalogue_type_check CHECK(catalogue_type IN ('CLIN','PARA','PHM'));

CREATE OR REPLACE FUNCTION siui_data_drug_fold(text) RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT lower(translate($1,'ÀÁÂÃÄÅÇÈÉÊËÌÍÎÏÑÒÓÔÕÖÙÚÛÜÝàáâãäåçèéêëìíîïñòóôõöùúûüýÿĀāĂăĄąĆćĈĉĊċČčĎďĒēĔĕĖėĘęĚěĜĝĞğĠġĢģĤĥĨĩĪīĬĭĮįİĴĵĶķĹĺĻļĽľŃńŅņŇňŌōŎŏŐőŔŕŖŗŘřŚśŜŝŞşŠšŢţŤťŨũŪūŬŭŮůŰűŲųŴŵŶŷŸŹźŻżŽžƠơƯưǍǎǏǐǑǒǓǔǕǖǗǘǙǚǛǜǞǟǠǡǦǧǨǩǪǫǬǭǰǴǵǸǹǺǻȀȁȂȃȄȅȆȇȈȉȊȋȌȍȎȏȐȑȒȓȔȕȖȗȘșȚțȞȟȦȧȨȩȪȫȬȭȮȯȰȱȲȳ','aaaaaaceeeeiiiinooooouuuuyaaaaaaceeeeiiiinooooouuuuyyaaaaaaccccccccddeeeeeeeeeegggggggghhiiiiiiiiijjkkllllllnnnnnnoooooorrrrrrssssssssttttuuuuuuuuuuuuwwyyyzzzzzzoouuaaiioouuuuuuuuuuaaaaggkkoooojggnnaaaaaaeeeeiiiioooorrrruuuusstthhaaeeooooooooyy')) $$;
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS drug_count bigint NOT NULL DEFAULT 0;
ALTER TABLE siui_data_imports ADD COLUMN IF NOT EXISTS drug_projection_version integer NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS siui_data_drugs(
 import_id uuid NOT NULL REFERENCES siui_data_imports(id) ON DELETE CASCADE,code text NOT NULL,
 name text NOT NULL,name_normalized text NOT NULL,search_text text NOT NULL,attributes jsonb NOT NULL,document jsonb NOT NULL,
 active_substance text NOT NULL,concentration text NOT NULL,pharmaceutical_form text NOT NULL,presentation_mode text NOT NULL,
 company text NOT NULL,country text NOT NULL,atc text NOT NULL,PRIMARY KEY(import_id,code)
);
CREATE INDEX IF NOT EXISTS siui_data_drug_search ON siui_data_drugs USING gin(search_text gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_drug_name ON siui_data_drugs USING gin(name_normalized gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_drug_substance ON siui_data_drugs USING gin(active_substance gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_drug_attributes ON siui_data_drugs USING gin(attributes jsonb_path_ops);
CREATE INDEX IF NOT EXISTS siui_data_drug_form ON siui_data_drugs(import_id,pharmaceutical_form,concentration);
CREATE INDEX IF NOT EXISTS siui_data_drug_atc ON siui_data_drugs(import_id,atc);
CREATE INDEX IF NOT EXISTS siui_data_drug_dci_atc ON siui_data_drugs(import_id,active_substance,atc);
CREATE TABLE IF NOT EXISTS siui_data_drug_terms(
 import_id uuid NOT NULL,drug_code text NOT NULL,ordinal bigint NOT NULL,source_kind text NOT NULL,attributes jsonb NOT NULL,document jsonb NOT NULL,
 copayment_list text NOT NULL,nhp_code text NOT NULL,disease_code text NOT NULL,disease_name text NOT NULL,
 approval text NOT NULL,max_price numeric,reference_price numeric,max_price_ut numeric,
 PRIMARY KEY(import_id,drug_code,ordinal),FOREIGN KEY(import_id,drug_code) REFERENCES siui_data_drugs(import_id,code) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_drug ON siui_data_drug_terms(import_id,drug_code);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_list ON siui_data_drug_terms(import_id,copayment_list,drug_code);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_program ON siui_data_drug_terms(import_id,nhp_code,drug_code);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_disease ON siui_data_drug_terms(import_id,disease_code,drug_code);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_disease_name ON siui_data_drug_terms USING gin(disease_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_max_price ON siui_data_drug_terms(import_id,max_price);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_ref_price ON siui_data_drug_terms(import_id,reference_price);
CREATE INDEX IF NOT EXISTS siui_data_drug_term_attributes ON siui_data_drug_terms USING gin(attributes jsonb_path_ops);
CREATE INDEX IF NOT EXISTS siui_data_drug_raw_relation ON siui_data_records(import_id,kind,(attributes->>'drug')) WHERE active AND kind='CopaymentListDrug';
CREATE INDEX IF NOT EXISTS siui_data_drug_raw_substance ON siui_data_records(import_id,kind,(attributes->>'activeSubstance')) WHERE active AND kind IN ('ActSubstICD10','CopaymentListProtocolTherapeutic');
INSERT INTO siui_data_schema_version(version) VALUES(2) ON CONFLICT DO NOTHING;
`
