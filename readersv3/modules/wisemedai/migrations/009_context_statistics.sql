ALTER TABLE wai_usage_records ADD COLUMN cached_input_tokens bigint NOT NULL DEFAULT 0 CHECK(cached_input_tokens>=0);
ALTER TABLE wai_usage_records ADD COLUMN request_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE wai_usage_records ADD COLUMN original_context_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE wai_usage_records ADD COLUMN sent_context_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE wai_usage_records ADD COLUMN latency_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE wai_usage_records ADD COLUMN context_strategy text NOT NULL DEFAULT 'legacy';
