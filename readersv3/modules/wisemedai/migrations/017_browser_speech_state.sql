ALTER TABLE wai_browser_speech ADD COLUMN capture_state text NOT NULL DEFAULT 'waiting';
ALTER TABLE wai_browser_speech ADD COLUMN capture_state_updated_at timestamptz NOT NULL DEFAULT now();
