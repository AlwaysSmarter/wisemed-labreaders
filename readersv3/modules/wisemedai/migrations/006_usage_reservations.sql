ALTER TABLE wai_usage_records ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'completed' CHECK(status IN ('reserved','completed','uncertain','rejected'));
