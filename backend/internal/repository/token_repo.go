package repository

import (
	"database/sql"
	"time"

	"github.com/google/uuid"

	"docforge/internal/models"
)

// RecordTokenUsage records an LLM token consumption event for a project
func (r *Repository) RecordTokenUsage(usage *models.TokenUsage) error {
	if usage == nil {
		return nil
	}
	if usage.ID == "" {
		usage.ID = uuid.New().String()
	}
	if usage.CreatedAt.IsZero() {
		usage.CreatedAt = time.Now().UTC()
	}
	if usage.TotalTokens == 0 && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	query := `
		INSERT INTO project_token_usage (
			id, project_id, generation_id, document_id, stage_id,
			operation, model_used, input_tokens, output_tokens, total_tokens, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		usage.ID,
		usage.ProjectID,
		usage.GenerationID,
		usage.DocumentID,
		usage.StageID,
		usage.Operation,
		usage.ModelUsed,
		usage.InputTokens,
		usage.OutputTokens,
		usage.TotalTokens,
		usage.CreatedAt,
	)
	return err
}

// GetProjectTokenSummary calculates aggregate token totals and breakdowns for chargeback
func (r *Repository) GetProjectTokenSummary(projectID string) (*models.ProjectTokenSummary, error) {
	summary := &models.ProjectTokenSummary{
		ProjectID:   projectID,
		ByModel:     make(map[string]int),
		ByOperation: make(map[string]int),
		RecentUsage: make([]models.TokenUsage, 0),
	}

	// 1. Overall totals
	totalQuery := `
		SELECT 
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COUNT(*)
		FROM project_token_usage
		WHERE project_id = ?
	`
	err := r.db.QueryRow(totalQuery, projectID).Scan(
		&summary.TotalTokens,
		&summary.TotalInputTokens,
		&summary.TotalOutputTokens,
		&summary.EventsCount,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// 2. Breakdown by model
	modelQuery := `
		SELECT model_used, COALESCE(SUM(total_tokens), 0)
		FROM project_token_usage
		WHERE project_id = ?
		GROUP BY model_used
	`
	modelRows, err := r.db.Query(modelQuery, projectID)
	if err == nil {
		defer modelRows.Close()
		for modelRows.Next() {
			var m string
			var tokens int
			if err := modelRows.Scan(&m, &tokens); err == nil && m != "" {
				summary.ByModel[m] = tokens
			}
		}
	}

	// 3. Breakdown by operation
	opQuery := `
		SELECT operation, COALESCE(SUM(total_tokens), 0)
		FROM project_token_usage
		WHERE project_id = ?
		GROUP BY operation
	`
	opRows, err := r.db.Query(opQuery, projectID)
	if err == nil {
		defer opRows.Close()
		for opRows.Next() {
			var op string
			var tokens int
			if err := opRows.Scan(&op, &tokens); err == nil && op != "" {
				summary.ByOperation[op] = tokens
			}
		}
	}

	// 4. Recent usage entries (limit 25)
	recentList, err := r.ListProjectTokenUsage(projectID, 25)
	if err == nil {
		summary.RecentUsage = recentList
	}

	return summary, nil
}

// ListProjectTokenUsage returns recent token ledger entries for a project
func (r *Repository) ListProjectTokenUsage(projectID string, limit int) ([]models.TokenUsage, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, project_id, generation_id, document_id, stage_id, operation, model_used, input_tokens, output_tokens, total_tokens, created_at
		FROM project_token_usage
		WHERE project_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := r.db.Query(query, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]models.TokenUsage, 0)
	for rows.Next() {
		var u models.TokenUsage
		var genID, docID, stageID sql.NullString
		if err := rows.Scan(
			&u.ID,
			&u.ProjectID,
			&genID,
			&docID,
			&stageID,
			&u.Operation,
			&u.ModelUsed,
			&u.InputTokens,
			&u.OutputTokens,
			&u.TotalTokens,
			&u.CreatedAt,
		); err != nil {
			return nil, err
		}
		if genID.Valid {
			u.GenerationID = &genID.String
		}
		if docID.Valid {
			u.DocumentID = &docID.String
		}
		if stageID.Valid {
			u.StageID = &stageID.String
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// UpdateGenerationTotalTokens computes and records total tokens used by a generation
func (r *Repository) UpdateGenerationTotalTokens(generationID string) error {
	query := `
		UPDATE generations
		SET total_tokens = (
			SELECT COALESCE(SUM(total_tokens), 0)
			FROM project_token_usage
			WHERE generation_id = ?
		)
		WHERE id = ?
	`
	_, err := r.db.Exec(query, generationID, generationID)
	return err
}
