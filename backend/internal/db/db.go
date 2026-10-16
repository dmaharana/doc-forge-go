package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"docforge/internal/config"
)

type DB struct {
	*sql.DB
}

func Connect(cfg *config.Config) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", cfg.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Optimize connection pool for SQLite
	db.SetMaxOpenConns(1) // Single writer for SQLite WAL prevents locks
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite db: %w", err)
	}

	database := &DB{db}
	if err := database.runMigrations(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	if err := database.seedAdmin(cfg); err != nil {
		return nil, fmt.Errorf("failed to seed admin user: %w", err)
	}

	return database, nil
}

func (db *DB) runMigrations() error {
	schema := `
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
		llm_config_id TEXT REFERENCES llm_configs(id),
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

	CREATE TABLE IF NOT EXISTS project_members (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
		user_email TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('owner', 'member')),
		status TEXT NOT NULL CHECK(status IN ('active', 'pending')),
		created_at DATETIME NOT NULL,
		UNIQUE(project_id, user_email)
	);
	CREATE INDEX IF NOT EXISTS idx_project_members_project ON project_members(project_id);
	CREATE INDEX IF NOT EXISTS idx_project_members_user ON project_members(user_id);
	CREATE INDEX IF NOT EXISTS idx_project_members_email ON project_members(user_email);
	`

	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Backfill existing project creators into project_members
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO project_members (id, project_id, user_id, user_email, role, status, created_at)
		SELECT 'pm_' || p.id, p.id, p.owner_id, COALESCE(u.email, 'admin@docforge.local'), 'owner', 'active', p.created_at
		FROM projects p
		LEFT JOIN users u ON p.owner_id = u.id
	`)

	// Ensure confluence_targets has new columns
	_ = db.addColumnIfNotExists("confluence_targets", "parent_page_title", "TEXT")
	_ = db.addColumnIfNotExists("confluence_targets", "updated_by", "TEXT")
	_ = db.addColumnIfNotExists("confluence_targets", "updated_at", "DATETIME")

	// Migration 003: Chunking, FTS5 Virtual Table, Generation Stages
	chunkingSchema := `
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
	`
	if _, err := db.Exec(chunkingSchema); err != nil {
		return fmt.Errorf("failed to apply chunking schema: %w", err)
	}

	_ = db.addColumnIfNotExists("generations", "system_map_md", "TEXT")
	_ = db.addColumnIfNotExists("generations", "completeness_scorecard_json", "TEXT")
	_ = db.addColumnIfNotExists("generations", "total_tokens", "INTEGER DEFAULT 0")
	_ = db.addColumnIfNotExists("projects", "llm_config_id", "TEXT REFERENCES llm_configs(id)")

	tokenUsageSchema := `
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
	`
	if _, err := db.Exec(tokenUsageSchema); err != nil {
		return fmt.Errorf("failed to apply token usage schema: %w", err)
	}

	return nil
}

func (db *DB) addColumnIfNotExists(table, column, colType string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	exists := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
			if name == column {
				exists = true
				break
			}
		}
	}

	if !exists {
		_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType))
		return err
	}
	return nil
}

func (db *DB) seedAdmin(cfg *config.Config) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	_, err = db.Exec(`
		INSERT INTO users (id, email, name, role, password_hash, is_active, created_at, updated_at)
		VALUES (?, ?, ?, 'admin', ?, 1, ?, ?)
	`, id, cfg.AdminEmail, cfg.AdminName, string(hash), now, now)

	if err != nil {
		return err
	}

	log.Printf("[DB] Seeded initial admin account: %s", cfg.AdminEmail)
	return nil
}
