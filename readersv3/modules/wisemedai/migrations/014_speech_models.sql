CREATE TABLE wai_speech_model_versions (
 model_id text NOT NULL, version text NOT NULL, manifest jsonb NOT NULL,
 manifest_hash text NOT NULL, published boolean NOT NULL DEFAULT false,
 created_by uuid REFERENCES wai_users(id), created_at timestamptz NOT NULL DEFAULT now(),
 published_at timestamptz, checked_at timestamptz,
 PRIMARY KEY(model_id,version)
);
CREATE UNIQUE INDEX wai_speech_model_current ON wai_speech_model_versions(model_id) WHERE published;
CREATE TABLE wai_speech_model_blobs (
 sha256 text PRIMARY KEY, byte_length bigint NOT NULL, verified_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wai_speech_model_jobs (
 id uuid PRIMARY KEY, model_id text NOT NULL, version text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('mirror','check')),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','ready','failed')),
 completed_files integer NOT NULL DEFAULT 0, total_files integer NOT NULL DEFAULT 0,
 error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(model_id,version) REFERENCES wai_speech_model_versions(model_id,version)
);
CREATE UNIQUE INDEX wai_speech_model_job_active ON wai_speech_model_jobs(model_id,version,kind) WHERE state IN ('queued','running');
