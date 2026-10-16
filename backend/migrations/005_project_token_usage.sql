-- Migration: Project Token Usage Ledger for Chargeback and Auditing

CREATE TABLE IF NOT EXISTS project_token_usage (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    generation_id TEXT REFERENCES generations(id) ON DELETE CASCADE,
    document_id TEXT REFERENCES documents(id) ON DELETE SET NULL,
    stage_id TEXT REFERENCES generation_stages(id) ON DELETE SET NULL,
    operation TEXT NOT NULL,
    model_used TEXT NOT NULL,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_token_usage_project ON project_token_usage(project_id);
CREATE INDEX IF NOT EXISTS idx_token_usage_gen ON project_token_usage(generation_id);
CREATE INDEX IF NOT EXISTS idx_token_usage_op ON project_token_usage(operation);

ALTER TABLE generations ADD COLUMN total_tokens INTEGER DEFAULT 0;
