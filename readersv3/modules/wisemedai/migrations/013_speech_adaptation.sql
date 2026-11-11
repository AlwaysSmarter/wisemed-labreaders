CREATE TABLE wai_speech_vocabularies (
 tenant_id uuid NOT NULL, template_id uuid NOT NULL, version bigint NOT NULL DEFAULT 1,
 PRIMARY KEY(tenant_id,template_id),
 FOREIGN KEY(tenant_id,template_id) REFERENCES wai_templates(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE wai_speech_terms (
 tenant_id uuid NOT NULL, template_id uuid NOT NULL, section_id text NOT NULL,
 term_key text NOT NULL, term bytea NOT NULL, source text NOT NULL CHECK(source IN ('manual','correction')),
 status text NOT NULL CHECK(status IN ('candidate','approved','rejected')),
 first_observed timestamptz NOT NULL DEFAULT now(), last_observed timestamptz NOT NULL DEFAULT now(),
 observations bigint NOT NULL DEFAULT 1, provenance uuid, author_id uuid REFERENCES wai_users(id),
 PRIMARY KEY(tenant_id,template_id,section_id,term_key),
 FOREIGN KEY(tenant_id,template_id) REFERENCES wai_templates(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE wai_speech_feedback (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, user_id uuid NOT NULL REFERENCES wai_users(id),
 template_id uuid NOT NULL, template_version uuid NOT NULL, section_id text NOT NULL,
 session_id uuid NOT NULL, accepted_revision integer NOT NULL, digest text NOT NULL,
 payload bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,session_id,accepted_revision,section_id),
 FOREIGN KEY(tenant_id,session_id) REFERENCES wai_sessions(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,template_id) REFERENCES wai_templates(tenant_id,id),
 FOREIGN KEY(tenant_id,template_version) REFERENCES wai_template_versions(tenant_id,id)
);
CREATE TABLE wai_speech_consent (
 tenant_id uuid NOT NULL, user_id uuid NOT NULL, enabled boolean NOT NULL DEFAULT false,
 notice_version text NOT NULL, activated_at timestamptz, withdrawn_at timestamptz,
 generation uuid NOT NULL,
 PRIMARY KEY(tenant_id,user_id),
 FOREIGN KEY(tenant_id,user_id) REFERENCES wai_memberships(tenant_id,user_id) ON DELETE CASCADE
);
CREATE TABLE wai_speech_consent_events (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, user_id uuid NOT NULL, enabled boolean NOT NULL,
 notice_version text NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,user_id) REFERENCES wai_memberships(tenant_id,user_id)
);
CREATE TABLE wai_speech_contributions (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, user_id uuid NOT NULL,
 source_session text NOT NULL, source_segment text NOT NULL, template_id uuid NOT NULL,
 template_version uuid NOT NULL, section_id text NOT NULL, consent_generation uuid NOT NULL,
 metadata bytea NOT NULL, digest text NOT NULL, audio_hash text NOT NULL,
 byte_length integer NOT NULL CHECK(byte_length BETWEEN 46 AND 960044),
 uploaded integer NOT NULL DEFAULT 0, state text NOT NULL DEFAULT 'uploading'
 CHECK(state IN ('uploading','pending_deidentification','needs_review','approved_for_dataset','rejected')),
 raw_path text, sanitized_path text, sanitized_transcript bytea, speaker_key bytea, reviewer_id uuid,
 review_payload bytea, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '7 days',
 UNIQUE(tenant_id,user_id,source_session,source_segment),
 FOREIGN KEY(tenant_id,user_id) REFERENCES wai_memberships(tenant_id,user_id),
 FOREIGN KEY(tenant_id,template_id) REFERENCES wai_templates(tenant_id,id),
 FOREIGN KEY(tenant_id,template_version) REFERENCES wai_template_versions(tenant_id,id)
);
CREATE TABLE wai_speech_upload_parts (
 contribution_id uuid NOT NULL REFERENCES wai_speech_contributions(id) ON DELETE CASCADE,
 offset_bytes integer NOT NULL, byte_length integer NOT NULL, digest text NOT NULL, path text NOT NULL,
 PRIMARY KEY(contribution_id,offset_bytes)
);
CREATE TABLE wai_speech_dataset_entries (
 dataset_id uuid NOT NULL, contribution_id uuid NOT NULL, revoked boolean NOT NULL DEFAULT false,
 exported_by uuid NOT NULL REFERENCES wai_users(id), exported_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(dataset_id,contribution_id)
);
CREATE INDEX wai_speech_contributions_retention ON wai_speech_contributions(expires_at);
CREATE TABLE wai_speech_grammars (
 tenant_id uuid NOT NULL, template_version uuid NOT NULL, section_id text NOT NULL,
 schema_version text NOT NULL, ir jsonb NOT NULL, sha256 text NOT NULL,
 PRIMARY KEY(tenant_id,template_version,section_id),
 FOREIGN KEY(tenant_id,template_version) REFERENCES wai_template_versions(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE wai_speech_term_observations (
 tenant_id uuid NOT NULL, template_id uuid NOT NULL, section_id text NOT NULL, term_key text NOT NULL,
 event_id uuid NOT NULL, source text NOT NULL, observed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,template_id,section_id,term_key,event_id),
 FOREIGN KEY(tenant_id,template_id,section_id,term_key) REFERENCES wai_speech_terms(tenant_id,template_id,section_id,term_key)
);
