CREATE TABLE IF NOT EXISTS wai_billing_plans (
 id uuid PRIMARY KEY, code text NOT NULL UNIQUE, current_version uuid, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS wai_billing_plan_versions (
 id uuid PRIMARY KEY, plan_id uuid NOT NULL REFERENCES wai_billing_plans(id), version integer NOT NULL CHECK(version>0),
 name text NOT NULL, description text NOT NULL DEFAULT '', currency text NOT NULL CHECK(currency IN ('USD','EUR','RON')),
 monthly_price_minor bigint NOT NULL CHECK(monthly_price_minor>=0),
 max_specialties integer NOT NULL CHECK(max_specialties BETWEEN 1 AND 10000),
 max_templates integer NOT NULL CHECK(max_templates BETWEEN 0 AND 1000000),
 max_fields_per_template integer NOT NULL DEFAULT 150 CHECK(max_fields_per_template BETWEEN 1 AND 150),
 included_ai_tokens bigint NOT NULL CHECK(included_ai_tokens>=0),
 extra_specialty_price_minor bigint NOT NULL DEFAULT 0 CHECK(extra_specialty_price_minor>=0),
 extra_template_price_minor bigint NOT NULL DEFAULT 0 CHECK(extra_template_price_minor>=0),
 extra_million_tokens_price_minor bigint NOT NULL DEFAULT 0 CHECK(extra_million_tokens_price_minor>=0),
 credit_limit_minor bigint NOT NULL DEFAULT 0 CHECK(credit_limit_minor>=0),
 extras_enabled boolean NOT NULL DEFAULT false, active boolean NOT NULL DEFAULT false,
 created_by uuid REFERENCES wai_users(id), created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(plan_id,version)
);
ALTER TABLE wai_billing_plans ADD CONSTRAINT wai_plan_current_version_fk FOREIGN KEY(current_version) REFERENCES wai_billing_plan_versions(id);
CREATE TABLE IF NOT EXISTS wai_plan_assignments (
 user_id uuid PRIMARY KEY REFERENCES wai_users(id), plan_version uuid NOT NULL REFERENCES wai_billing_plan_versions(id),
 assigned_by uuid NOT NULL REFERENCES wai_users(id), assigned_at timestamptz NOT NULL DEFAULT now(),
 credit_limit_minor bigint CHECK(credit_limit_minor>=0), note text NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS wai_user_specialties (
 user_id uuid NOT NULL REFERENCES wai_users(id), specialty text NOT NULL, activated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,specialty)
);
ALTER TABLE wai_templates ADD COLUMN IF NOT EXISTS owner_id uuid REFERENCES wai_users(id);
CREATE INDEX IF NOT EXISTS wai_templates_owner ON wai_templates(owner_id) WHERE owner_id IS NOT NULL;
UPDATE wai_templates t SET owner_id=(
 SELECT a.user_id FROM wai_audit_events a JOIN wai_template_versions v ON v.id::text=a.entity_id AND v.tenant_id=a.tenant_id
 WHERE a.action='template.publish' AND v.template_id=t.id AND a.user_id IS NOT NULL ORDER BY a.created_at LIMIT 1
) WHERE t.owner_id IS NULL AND NOT EXISTS(SELECT 1 FROM wai_seed_catalog c WHERE c.template_id=t.id);
ALTER TABLE wai_usage_records ADD COLUMN IF NOT EXISTS session_id uuid;
ALTER TABLE wai_usage_records ADD COLUMN IF NOT EXISTS template_version uuid;
ALTER TABLE wai_usage_records ADD COLUMN IF NOT EXISTS operation text NOT NULL DEFAULT 'legacy';
ALTER TABLE wai_usage_records ADD COLUMN IF NOT EXISTS reserved_tokens bigint NOT NULL DEFAULT 0 CHECK(reserved_tokens>=0);
ALTER TABLE wai_usage_records ADD COLUMN IF NOT EXISTS plan_version uuid REFERENCES wai_billing_plan_versions(id);
CREATE INDEX IF NOT EXISTS wai_usage_user_date ON wai_usage_records(user_id,created_at);
-- The initial tariffs are deliberately unset. Paid packages are drafts until configured.
INSERT INTO wai_billing_plans(id,code) VALUES
 ('ba110000-0000-4000-8000-000000000001','free'),('ba110000-0000-4000-8000-000000000002','pro'),('ba110000-0000-4000-8000-000000000003','premium');
INSERT INTO wai_billing_plan_versions(id,plan_id,version,name,description,currency,monthly_price_minor,max_specialties,max_templates,included_ai_tokens,active) VALUES
 ('ba110001-0000-4000-8000-000000000001','ba110000-0000-4000-8000-000000000001',1,'Free','Pachet implicit. Limite inițiale configurabile.','USD',0,1,3,50000,true),
 ('ba110001-0000-4000-8000-000000000002','ba110000-0000-4000-8000-000000000002',1,'Pro','Schiță comercială: configurați prețul și limitele înainte de ofertare.','USD',0,2,25,500000,false),
 ('ba110001-0000-4000-8000-000000000003','ba110000-0000-4000-8000-000000000003',1,'Premium','Schiță comercială: configurați prețul și limitele înainte de ofertare.','USD',0,10,100,2000000,false);
UPDATE wai_billing_plans p SET current_version=v.id FROM wai_billing_plan_versions v WHERE v.plan_id=p.id;
