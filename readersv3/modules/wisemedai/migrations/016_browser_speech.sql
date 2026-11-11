CREATE TABLE wai_browser_speech (
 token_hash text PRIMARY KEY, login_hash text NOT NULL REFERENCES wai_auth_sessions(token_hash) ON DELETE CASCADE,
 tenant_id uuid NOT NULL, user_id uuid NOT NULL, session_id uuid, template_version uuid NOT NULL,
 section_id text NOT NULL DEFAULT '', target_field text NOT NULL DEFAULT '', base_sequence integer NOT NULL,
 partial bytea, stop_requested boolean NOT NULL DEFAULT false, finished boolean NOT NULL DEFAULT false,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '2 hours', created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,user_id) REFERENCES wai_memberships(tenant_id,user_id),
 FOREIGN KEY(tenant_id,session_id) REFERENCES wai_sessions(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,template_version) REFERENCES wai_template_versions(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE wai_browser_speech_events (
 token_hash text NOT NULL REFERENCES wai_browser_speech(token_hash) ON DELETE CASCADE,
 ordinal integer NOT NULL, event_id uuid NOT NULL, digest text NOT NULL, payload bytea NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(token_hash,ordinal), UNIQUE(token_hash,event_id)
);
