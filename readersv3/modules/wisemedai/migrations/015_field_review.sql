ALTER TABLE wai_session_chunks ADD COLUMN IF NOT EXISTS target_field text NOT NULL DEFAULT '';
