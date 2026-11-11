CREATE TABLE wai_template_addons (
 template_id uuid PRIMARY KEY REFERENCES wai_templates(id),
 user_id uuid NOT NULL REFERENCES wai_users(id),
 request_id uuid NOT NULL,
 plan_version uuid NOT NULL REFERENCES wai_billing_plan_versions(id),
 currency text NOT NULL CHECK(currency IN ('USD','EUR','RON')),
 monthly_minor bigint NOT NULL CHECK(monthly_minor>0),
 started_at timestamptz NOT NULL,
 ends_at timestamptz,
 UNIQUE(user_id,request_id)
);
CREATE INDEX wai_template_addons_user ON wai_template_addons(user_id);
ALTER TABLE wai_billing_invoices ALTER COLUMN period_id DROP NOT NULL;
ALTER TABLE wai_billing_invoices DROP CONSTRAINT wai_billing_invoices_kind_check;
ALTER TABLE wai_billing_invoices ADD COLUMN template_addon uuid REFERENCES wai_template_addons(template_id);
ALTER TABLE wai_billing_invoices ADD COLUMN addon_month timestamptz;
ALTER TABLE wai_billing_invoices ADD CHECK (
 (kind IN ('subscription','usage') AND period_id IS NOT NULL AND template_addon IS NULL AND addon_month IS NULL)
 OR (kind='template_addon' AND period_id IS NULL AND template_addon IS NOT NULL AND addon_month IS NOT NULL)
);
CREATE UNIQUE INDEX wai_invoice_template_month ON wai_billing_invoices(template_addon,addon_month);
