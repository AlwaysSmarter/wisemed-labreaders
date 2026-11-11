ALTER TABLE wai_usage_records ADD COLUMN encrypted_prompt bytea, ADD COLUMN encrypted_output bytea;
CREATE INDEX wai_usage_prompt_session ON wai_usage_records(tenant_id, session_id) WHERE encrypted_prompt IS NOT NULL;
CREATE INDEX wai_usage_prompt_date ON wai_usage_records(created_at) WHERE encrypted_prompt IS NOT NULL;

CREATE FUNCTION wai_clear_session_prompts() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE wai_usage_records SET encrypted_prompt = NULL, encrypted_output = NULL
    WHERE tenant_id = OLD.tenant_id AND session_id = OLD.id AND encrypted_prompt IS NOT NULL;
    RETURN OLD;
END;
$$;

CREATE TRIGGER wai_clear_session_prompts AFTER DELETE ON wai_sessions
FOR EACH ROW EXECUTE FUNCTION wai_clear_session_prompts();
