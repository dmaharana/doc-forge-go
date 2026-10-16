-- Migration: Source Chunking, SQLite FTS5 Retrieval Index & Generation Stages

CREATE TABLE IF NOT EXISTS source_chunks (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    file_path TEXT,
    symbol_name TEXT,
    chunk_type TEXT NOT NULL,
    line_start INTEGER DEFAULT 0,
    line_end INTEGER DEFAULT 0,
    search_header TEXT,
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_source_chunks_source ON source_chunks(source_id);
CREATE INDEX IF NOT EXISTS idx_source_chunks_project ON source_chunks(project_id);
CREATE INDEX IF NOT EXISTS idx_source_chunks_type ON source_chunks(chunk_type);

CREATE VIRTUAL TABLE IF NOT EXISTS source_chunks_fts USING fts5(
    chunk_id UNINDEXED,
    project_id UNINDEXED,
    source_id UNINDEXED,
    file_path,
    symbol_name,
    chunk_type,
    search_header,
    content,
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS trg_source_chunks_insert AFTER INSERT ON source_chunks BEGIN
    INSERT INTO source_chunks_fts(chunk_id, project_id, source_id, file_path, symbol_name, chunk_type, search_header, content)
    VALUES (new.id, new.project_id, new.source_id, COALESCE(new.file_path, ''), COALESCE(new.symbol_name, ''), new.chunk_type, COALESCE(new.search_header, ''), new.content);
END;

CREATE TRIGGER IF NOT EXISTS trg_source_chunks_delete AFTER DELETE ON source_chunks BEGIN
    DELETE FROM source_chunks_fts WHERE chunk_id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS trg_source_chunks_update AFTER UPDATE ON source_chunks BEGIN
    DELETE FROM source_chunks_fts WHERE chunk_id = old.id;
    INSERT INTO source_chunks_fts(chunk_id, project_id, source_id, file_path, symbol_name, chunk_type, search_header, content)
    VALUES (new.id, new.project_id, new.source_id, COALESCE(new.file_path, ''), COALESCE(new.symbol_name, ''), new.chunk_type, COALESCE(new.search_header, ''), new.content);
END;

CREATE TABLE IF NOT EXISTS generation_stages (
    id TEXT PRIMARY KEY,
    generation_id TEXT NOT NULL REFERENCES generations(id) ON DELETE CASCADE,
    stage_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'processing', 'completed', 'failed', 'skipped')),
    input_context TEXT,
    output_payload TEXT,
    tokens_used INTEGER DEFAULT 0,
    duration_ms INTEGER DEFAULT 0,
    error_message TEXT,
    created_at DATETIME NOT NULL,
    completed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_gen_stages_gen ON generation_stages(generation_id);

ALTER TABLE generations ADD COLUMN system_map_md TEXT;
ALTER TABLE generations ADD COLUMN completeness_scorecard_json TEXT;
