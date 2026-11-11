CREATE TABLE wai_subscriptions (
 id uuid PRIMARY KEY, user_id uuid NOT NULL UNIQUE REFERENCES wai_users(id),
 plan_version uuid NOT NULL REFERENCES wai_billing_plan_versions(id),
 started_at timestamptz NOT NULL, created_by uuid NOT NULL REFERENCES wai_users(id),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','cancelled'))
);
CREATE TABLE wai_billing_periods (
 id uuid PRIMARY KEY, subscription_id uuid NOT NULL REFERENCES wai_subscriptions(id),
 starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL CHECK(ends_at>starts_at),
 UNIQUE(subscription_id,starts_at)
);
CREATE TABLE wai_billing_invoices (
 id uuid PRIMARY KEY, user_id uuid NOT NULL REFERENCES wai_users(id),
 period_id uuid NOT NULL REFERENCES wai_billing_periods(id),
 kind text NOT NULL CHECK(kind IN ('subscription','usage')),
 currency text NOT NULL CHECK(currency IN ('USD','EUR','RON')),
 amount_minor bigint NOT NULL CHECK(amount_minor>0),
 issued_at timestamptz NOT NULL, due_at timestamptz NOT NULL,
 reference text NOT NULL UNIQUE
);
CREATE UNIQUE INDEX wai_invoice_subscription_period ON wai_billing_invoices(period_id) WHERE kind='subscription';
CREATE INDEX wai_invoice_owner ON wai_billing_invoices(user_id,issued_at);
CREATE TABLE wai_billing_payments (
 id uuid PRIMARY KEY, invoice_id uuid NOT NULL REFERENCES wai_billing_invoices(id),
 amount_minor bigint NOT NULL CHECK(amount_minor>0),
 reference text NOT NULL UNIQUE CHECK(length(reference) BETWEEN 3 AND 200),
 recorded_by uuid NOT NULL REFERENCES wai_users(id), recorded_at timestamptz NOT NULL DEFAULT now(),
 method text NOT NULL DEFAULT 'manual_verified' CHECK(method='manual_verified')
);
CREATE INDEX wai_payments_invoice ON wai_billing_payments(invoice_id);
-- Usage is bound to its commercial period at reservation; no retroactive attribution.
ALTER TABLE wai_usage_records ADD COLUMN billing_period_id uuid REFERENCES wai_billing_periods(id);
CREATE INDEX wai_usage_billing_period ON wai_usage_records(billing_period_id);
