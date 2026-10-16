package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"docforge/internal/models"
)

func (r *Repository) SaveSourceChunks(chunks []models.SourceChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO source_chunks (id, source_id, project_id, chunk_index, file_path, symbol_name, chunk_type, line_start, line_end, search_header, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, c := range chunks {
		id := c.ID
		if id == "" {
			id = uuid.New().String()
		}
		createdAt := c.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		_, err := stmt.Exec(id, c.SourceID, c.ProjectID, c.ChunkIndex, c.FilePath, c.SymbolName, c.ChunkType, c.LineStart, c.LineEnd, c.SearchHeader, c.Content, createdAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) DeleteSourceChunksBySource(sourceID string) error {
	_, err := r.db.Exec("DELETE FROM source_chunks WHERE source_id = ?", sourceID)
	return err
}

func (r *Repository) ListSourceChunksBySource(sourceID string) ([]models.SourceChunk, error) {
	rows, err := r.db.Query(`
		SELECT id, source_id, project_id, chunk_index, file_path, symbol_name, chunk_type, line_start, line_end, search_header, content, created_at
		FROM source_chunks
		WHERE source_id = ?
		ORDER BY chunk_index ASC
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChunks(rows)
}

func (r *Repository) ListSourceChunksByProject(projectID string) ([]models.SourceChunk, error) {
	rows, err := r.db.Query(`
		SELECT id, source_id, project_id, chunk_index, file_path, symbol_name, chunk_type, line_start, line_end, search_header, content, created_at
		FROM source_chunks
		WHERE project_id = ?
		ORDER BY file_path ASC, chunk_index ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChunks(rows)
}

func (r *Repository) SearchSourceChunks(projectID, query string, chunkTypes []string, limit int) ([]models.SourceChunk, error) {
	if limit <= 0 {
		limit = 10
	}
	cleanQuery := sanitizeFTS5Query(query)
	if cleanQuery == "" {
		return r.listRecentChunks(projectID, chunkTypes, limit)
	}

	var typeFilter string
	var args []any
	args = append(args, cleanQuery, projectID)

	if len(chunkTypes) > 0 {
		placeholders := make([]string, len(chunkTypes))
		for i, ct := range chunkTypes {
			placeholders[i] = "?"
			args = append(args, ct)
		}
		typeFilter = fmt.Sprintf("AND c.chunk_type IN (%s)", strings.Join(placeholders, ","))
	}
	args = append(args, limit)

	sqlQuery := fmt.Sprintf(`
		SELECT c.id, c.source_id, c.project_id, c.chunk_index, c.file_path, c.symbol_name, c.chunk_type, c.line_start, c.line_end, c.search_header, c.content, c.created_at
		FROM source_chunks c
		JOIN source_chunks_fts fts ON c.id = fts.chunk_id
		WHERE source_chunks_fts MATCH ?
		  AND c.project_id = ?
		  %s
		ORDER BY bm25(source_chunks_fts) ASC
		LIMIT ?
	`, typeFilter)

	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return r.fallbackLikeSearch(projectID, query, chunkTypes, limit)
	}
	defer rows.Close()

	chunks, err := scanChunks(rows)
	if err != nil || len(chunks) == 0 {
		return r.fallbackLikeSearch(projectID, query, chunkTypes, limit)
	}
	return chunks, nil
}

func (r *Repository) listRecentChunks(projectID string, chunkTypes []string, limit int) ([]models.SourceChunk, error) {
	var typeFilter string
	var args []any
	args = append(args, projectID)

	if len(chunkTypes) > 0 {
		placeholders := make([]string, len(chunkTypes))
		for i, ct := range chunkTypes {
			placeholders[i] = "?"
			args = append(args, ct)
		}
		typeFilter = fmt.Sprintf("AND chunk_type IN (%s)", strings.Join(placeholders, ","))
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT id, source_id, project_id, chunk_index, file_path, symbol_name, chunk_type, line_start, line_end, search_header, content, created_at
		FROM source_chunks
		WHERE project_id = ?
		  %s
		ORDER BY chunk_index ASC
		LIMIT ?
	`, typeFilter)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChunks(rows)
}

func (r *Repository) fallbackLikeSearch(projectID, query string, chunkTypes []string, limit int) ([]models.SourceChunk, error) {
	likePattern := "%" + query + "%"
	var typeFilter string
	var args []any
	args = append(args, projectID, likePattern, likePattern, likePattern)

	if len(chunkTypes) > 0 {
		placeholders := make([]string, len(chunkTypes))
		for i, ct := range chunkTypes {
			placeholders[i] = "?"
			args = append(args, ct)
		}
		typeFilter = fmt.Sprintf("AND chunk_type IN (%s)", strings.Join(placeholders, ","))
	}
	args = append(args, limit)

	sqlQuery := fmt.Sprintf(`
		SELECT id, source_id, project_id, chunk_index, file_path, symbol_name, chunk_type, line_start, line_end, search_header, content, created_at
		FROM source_chunks
		WHERE project_id = ?
		  AND (content LIKE ? OR symbol_name LIKE ? OR file_path LIKE ?)
		  %s
		LIMIT ?
	`, typeFilter)

	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChunks(rows)
}

func (r *Repository) ListProjectCodeSymbols(projectID string) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT symbol_name
		FROM source_chunks
		WHERE project_id = ? AND symbol_name IS NOT NULL AND symbol_name != ''
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil && s != "" {
			symbols = append(symbols, s)
		}
	}
	return symbols, rows.Err()
}

func scanChunks(rows *sql.Rows) ([]models.SourceChunk, error) {
	var list []models.SourceChunk
	for rows.Next() {
		var c models.SourceChunk
		var fp, sym, hdr sql.NullString
		if err := rows.Scan(&c.ID, &c.SourceID, &c.ProjectID, &c.ChunkIndex, &fp, &sym, &c.ChunkType, &c.LineStart, &c.LineEnd, &hdr, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		if fp.Valid {
			c.FilePath = fp.String
		}
		if sym.Valid {
			c.SymbolName = sym.String
		}
		if hdr.Valid {
			c.SearchHeader = hdr.String
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func sanitizeFTS5Query(q string) string {
	var words []string
	for _, token := range strings.Fields(q) {
		token = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return -1
		}, token)
		if len(token) > 1 {
			words = append(words, token+"*")
		}
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, " OR ")
}
