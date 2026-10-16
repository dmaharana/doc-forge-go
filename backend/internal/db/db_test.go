package db

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFTS5Support(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE VIRTUAL TABLE test_fts USING fts5(
			chunk_id UNINDEXED,
			project_id UNINDEXED,
			file_path,
			symbol_name,
			chunk_type,
			search_header,
			content
		);
		INSERT INTO test_fts (chunk_id, project_id, file_path, symbol_name, chunk_type, search_header, content)
		VALUES ('c1', 'p1', 'internal/auth.go', 'LoginUser', 'code_symbol', '[Auth Handler]', 'func LoginUser handles authentication with JWT tokens');
	`)
	if err != nil {
		t.Fatalf("failed to create or insert into FTS5 table: %v", err)
	}

	var symbol string
	err = db.QueryRow("SELECT symbol_name FROM test_fts WHERE test_fts MATCH 'authentication'").Scan(&symbol)
	if err != nil {
		t.Fatalf("failed to query FTS5 table: %v", err)
	}

	if symbol != "LoginUser" {
		t.Fatalf("expected symbol 'LoginUser', got '%s'", symbol)
	}
}
