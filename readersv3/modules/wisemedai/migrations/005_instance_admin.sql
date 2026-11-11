ALTER TABLE wai_users ADD COLUMN IF NOT EXISTS instance_admin boolean NOT NULL DEFAULT false;
UPDATE wai_users SET instance_admin=true WHERE id=(SELECT id FROM wai_users ORDER BY created_at,id LIMIT 1);
ALTER TABLE wai_memberships ADD COLUMN IF NOT EXISTS disabled boolean NOT NULL DEFAULT false;
