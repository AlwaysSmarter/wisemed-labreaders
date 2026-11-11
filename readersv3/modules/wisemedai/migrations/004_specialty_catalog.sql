CREATE TABLE IF NOT EXISTS wai_seed_catalog (
 tenant_id uuid NOT NULL REFERENCES wai_organizations(id),
 slug text NOT NULL,
 template_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,slug),
 FOREIGN KEY(tenant_id,template_id) REFERENCES wai_templates(tenant_id,id)
);
