-- DocForge Initial SQLite Schema

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('admin', 'user')),
    password_hash TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS llm_configs (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    api_key_encrypted TEXT NOT NULL,
    base_url TEXT,
    default_model TEXT NOT NULL,
    max_tokens INTEGER NOT NULL DEFAULT 4096,
    temperature REAL NOT NULL DEFAULT 0.2,
    is_active INTEGER NOT NULL DEFAULT 1,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    owner_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS sources (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('upload', 'remote')),
    file_path TEXT,
    original_filename TEXT,
    mime_type TEXT,
    size_bytes INTEGER,
    remote_url TEXT,
    remote_type TEXT,
    extracted_text TEXT,
    metadata_json TEXT,
    status TEXT NOT NULL DEFAULT 'ready',
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS atlassian_connections (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token_encrypted TEXT NOT NULL,
    refresh_token_encrypted TEXT,
    token_expiry DATETIME,
    cloud_id TEXT,
    site_url TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS confluence_targets (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    connection_id TEXT NOT NULL REFERENCES atlassian_connections(id) ON DELETE CASCADE,
    space_key TEXT NOT NULL,
    space_name TEXT NOT NULL,
    parent_page_id TEXT,
    is_default INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS generations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    requested_by TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK(status IN ('queued', 'processing', 'completed', 'failed')),
    template_set TEXT NOT NULL,
    llm_config_id TEXT REFERENCES llm_configs(id),
    model_used TEXT,
    error_message TEXT,
    notification_sent_at DATETIME,
    created_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME
);

CREATE TABLE IF NOT EXISTS generation_sources (
    generation_id TEXT NOT NULL REFERENCES generations(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    PRIMARY KEY (generation_id, source_id)
);

CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY,
    generation_id TEXT REFERENCES generations(id) ON DELETE SET NULL,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    template_type TEXT NOT NULL,
    content_md TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('draft', 'in_review', 'approved')),
    current_version_id TEXT,
    confluence_page_id TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS document_versions (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    content_md TEXT NOT NULL,
    change_summary TEXT,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS document_feedback (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id),
    section_anchor TEXT,
    category TEXT NOT NULL CHECK(category IN ('correction', 'missing_info', 'additional_fact', 'clarification', 'style', 'other')),
    content TEXT NOT NULL,
    is_blocking INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK(status IN ('open', 'addressed', 'dismissed')) DEFAULT 'open',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS notification_preferences (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    channel TEXT NOT NULL CHECK(channel IN ('email', 'teams')),
    enabled INTEGER NOT NULL DEFAULT 1,
    teams_webhook_url_encrypted TEXT,
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sources_project ON sources(project_id);
CREATE INDEX IF NOT EXISTS idx_generations_project ON generations(project_id);
CREATE INDEX IF NOT EXISTS idx_generations_status ON generations(status);
CREATE INDEX IF NOT EXISTS idx_documents_project ON documents(project_id);
CREATE INDEX IF NOT EXISTS idx_doc_versions_doc ON document_versions(document_id);
CREATE INDEX IF NOT EXISTS idx_feedback_doc ON document_feedback(document_id);
