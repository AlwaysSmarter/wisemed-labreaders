-- pgvector is optional on shared installations. The Docker database provisions it.
-- No extension is installed into an existing WiseMED database automatically.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector') THEN
  EXECUTE 'ALTER TABLE wai_memory_examples ADD COLUMN IF NOT EXISTS embedding vector(1536)';
 END IF;
END $$;
