CREATE TABLE wai_template_contexts (
 template_version uuid PRIMARY KEY REFERENCES wai_template_versions(id) ON DELETE CASCADE,
 tenant_id uuid NOT NULL REFERENCES wai_organizations(id),
 created_by uuid NOT NULL REFERENCES wai_users(id),
 payload bytea NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wai_template_context_tenant ON wai_template_contexts(tenant_id);
