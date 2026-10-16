package repository

import (
	"database/sql"
	"time"

	"github.com/google/uuid"

	"docforge/internal/models"
)

func (r *Repository) CreateGenerationStage(stage *models.GenerationStage) error {
	if stage.ID == "" {
		stage.ID = uuid.New().String()
	}
	if stage.CreatedAt.IsZero() {
		stage.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO generation_stages (id, generation_id, stage_name, status, input_context, output_payload, tokens_used, duration_ms, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, stage.ID, stage.GenerationID, stage.StageName, stage.Status, stage.InputContext, stage.OutputPayload, stage.TokensUsed, stage.DurationMs, stage.ErrorMessage, stage.CreatedAt)
	return err
}

func (r *Repository) UpdateGenerationStage(stage *models.GenerationStage) error {
	now := time.Now().UTC()
	stage.CompletedAt = &now

	query := `
		UPDATE generation_stages
		SET status = ?, output_payload = ?, tokens_used = ?, duration_ms = ?, error_message = ?, completed_at = ?
		WHERE id = ?
	`
	_, err := r.db.Exec(query, stage.Status, stage.OutputPayload, stage.TokensUsed, stage.DurationMs, stage.ErrorMessage, stage.CompletedAt, stage.ID)
	return err
}

func (r *Repository) ListGenerationStages(generationID string) ([]models.GenerationStage, error) {
	query := `
		SELECT id, generation_id, stage_name, status, input_context, output_payload, tokens_used, duration_ms, error_message, created_at, completed_at
		FROM generation_stages
		WHERE generation_id = ?
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(query, generationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.GenerationStage
	for rows.Next() {
		var s models.GenerationStage
		var inCtx, outPayload, errMsg sql.NullString
		var completedAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.GenerationID, &s.StageName, &s.Status, &inCtx, &outPayload, &s.TokensUsed, &s.DurationMs, &errMsg, &s.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		if inCtx.Valid {
			s.InputContext = inCtx.String
		}
		if outPayload.Valid {
			s.OutputPayload = outPayload.String
		}
		if errMsg.Valid {
			s.ErrorMessage = errMsg.String
		}
		if completedAt.Valid {
			s.CompletedAt = &completedAt.Time
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *Repository) UpdateGenerationSystemMap(generationID, systemMapMD string) error {
	_, err := r.db.Exec("UPDATE generations SET system_map_md = ? WHERE id = ?", systemMapMD, generationID)
	return err
}

func (r *Repository) UpdateGenerationScorecard(generationID, scorecardJSON string) error {
	_, err := r.db.Exec("UPDATE generations SET completeness_scorecard_json = ? WHERE id = ?", scorecardJSON, generationID)
	return err
}
