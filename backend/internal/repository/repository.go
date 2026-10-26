package repository

import (
	"database/sql"
	"time"

	"github.com/google/uuid"

	"docforge/internal/db"
	"docforge/internal/models"
)

type Repository struct {
	db *db.DB
}

func New(database *db.DB) *Repository {
	return &Repository{db: database}
}

// ----------------- USERS -----------------

func (r *Repository) CreateUser(u *models.User) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	query := `
		INSERT INTO users (id, email, name, role, password_hash, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, u.ID, u.Email, u.Name, u.Role, u.PasswordHash, u.IsActive, u.CreatedAt, u.UpdatedAt)
	return err
}

func (r *Repository) GetUserByID(id string) (*models.User, error) {
	query := `SELECT id, email, name, role, password_hash, is_active, created_at, updated_at FROM users WHERE id = ?`
	row := r.db.QueryRow(query, id)

	var u models.User
	var isActive int
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &isActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *Repository) GetUserByEmail(email string) (*models.User, error) {
	query := `SELECT id, email, name, role, password_hash, is_active, created_at, updated_at FROM users WHERE email = ?`
	row := r.db.QueryRow(query, email)

	var u models.User
	var isActive int
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &isActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *Repository) ListUsers() ([]models.User, error) {
	query := `SELECT id, email, name, role, is_active, created_at, updated_at FROM users ORDER BY created_at DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		var isActive int
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &isActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.IsActive = isActive == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repository) UpdateUser(id, name, role string, isActive bool) error {
	query := `UPDATE users SET name = ?, role = ?, is_active = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, name, role, isActive, time.Now().UTC(), id)
	return err
}

// ----------------- LLM CONFIGS -----------------

func (r *Repository) SaveLLMConfig(cfg *models.LLMConfig) error {
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}
	cfg.UpdatedAt = time.Now().UTC()

	// If this config is set active, deactivate others
	if cfg.IsActive {
		_, _ = r.db.Exec("UPDATE llm_configs SET is_active = 0")
	}

	query := `
		INSERT INTO llm_configs (id, provider, api_key_encrypted, base_url, default_model, max_tokens, temperature, is_active, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider = excluded.provider,
			api_key_encrypted = excluded.api_key_encrypted,
			base_url = excluded.base_url,
			default_model = excluded.default_model,
			max_tokens = excluded.max_tokens,
			temperature = excluded.temperature,
			is_active = excluded.is_active,
			updated_at = excluded.updated_at
	`
	_, err := r.db.Exec(query, cfg.ID, cfg.Provider, cfg.APIKeyEncrypted, cfg.BaseURL, cfg.DefaultModel, cfg.MaxTokens, cfg.Temperature, cfg.IsActive, cfg.UpdatedAt)
	return err
}

func (r *Repository) GetActiveLLMConfig() (*models.LLMConfig, error) {
	query := `SELECT id, provider, api_key_encrypted, base_url, default_model, max_tokens, temperature, is_active, updated_at FROM llm_configs WHERE is_active = 1 LIMIT 1`
	row := r.db.QueryRow(query)

	var c models.LLMConfig
	var isActive int
	var baseURL sql.NullString
	err := row.Scan(&c.ID, &c.Provider, &c.APIKeyEncrypted, &baseURL, &c.DefaultModel, &c.MaxTokens, &c.Temperature, &isActive, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if baseURL.Valid {
		c.BaseURL = baseURL.String
	}
	c.IsActive = isActive == 1
	return &c, nil
}

func (r *Repository) ListLLMConfigs() ([]models.LLMConfig, error) {
	query := `SELECT id, provider, api_key_encrypted, base_url, default_model, max_tokens, temperature, is_active, updated_at FROM llm_configs ORDER BY updated_at DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]models.LLMConfig, 0)
	for rows.Next() {
		var c models.LLMConfig
		var isActive int
		var baseURL sql.NullString
		if err := rows.Scan(&c.ID, &c.Provider, &c.APIKeyEncrypted, &baseURL, &c.DefaultModel, &c.MaxTokens, &c.Temperature, &isActive, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if baseURL.Valid {
			c.BaseURL = baseURL.String
		}
		c.IsActive = isActive == 1
		list = append(list, c)
	}
	return list, rows.Err()
}

// ----------------- PROJECTS -----------------

func (r *Repository) CreateProject(p *models.Project) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = "active"
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO projects (id, name, description, owner_id, status, llm_config_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.Exec(query, p.ID, p.Name, p.Description, p.OwnerID, p.Status, p.LLMConfigID, p.CreatedAt, p.UpdatedAt); err != nil {
		return err
	}

	var email string
	_ = tx.QueryRow("SELECT email FROM users WHERE id = ?", p.OwnerID).Scan(&email)
	if email == "" {
		email = "admin@docforge.local"
	}

	memberID := "pm_" + p.ID
	pmQuery := `INSERT INTO project_members (id, project_id, user_id, user_email, role, status, created_at) VALUES (?, ?, ?, ?, 'owner', 'active', ?)`
	if _, err := tx.Exec(pmQuery, memberID, p.ID, p.OwnerID, email, now); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Repository) GetProjectByID(id string) (*models.Project, error) {
	query := `
		SELECT p.id, p.name, p.description, p.owner_id, u.name, p.status, p.llm_config_id, COALESCE(lc.provider, ''), COALESCE(lc.default_model, ''),
		       COALESCE((SELECT SUM(total_tokens) FROM project_token_usage WHERE project_id = p.id), 0) as total_tokens,
		       p.created_at, p.updated_at
		FROM projects p
		JOIN users u ON p.owner_id = u.id
		LEFT JOIN llm_configs lc ON p.llm_config_id = lc.id
		WHERE p.id = ?
	`
	row := r.db.QueryRow(query, id)

	var p models.Project
	var desc sql.NullString
	var llmCfgID sql.NullString
	err := row.Scan(&p.ID, &p.Name, &desc, &p.OwnerID, &p.OwnerName, &p.Status, &llmCfgID, &p.LLMProvider, &p.LLMModel, &p.TotalTokens, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		p.Description = desc.String
	}
	if llmCfgID.Valid {
		p.LLMConfigID = &llmCfgID.String
	}
	return &p, nil
}

func (r *Repository) ListProjects(userID string, isAdmin bool) ([]models.Project, error) {
	var query string
	var args []any
	if isAdmin {
		query = `
			SELECT p.id, p.name, p.description, p.owner_id, u.name, p.status, p.llm_config_id, COALESCE(lc.provider, ''), COALESCE(lc.default_model, ''), COALESCE(pm.role, 'admin') as user_role,
			       COALESCE((SELECT SUM(total_tokens) FROM project_token_usage WHERE project_id = p.id), 0) as total_tokens,
			       p.created_at, p.updated_at
			FROM projects p
			JOIN users u ON p.owner_id = u.id
			LEFT JOIN llm_configs lc ON p.llm_config_id = lc.id
			LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.user_id = ? AND pm.status = 'active'
			ORDER BY p.updated_at DESC
		`
		args = append(args, userID)
	} else {
		query = `
			SELECT p.id, p.name, p.description, p.owner_id, u.name, p.status, p.llm_config_id, COALESCE(lc.provider, ''), COALESCE(lc.default_model, ''), pm.role as user_role,
			       COALESCE((SELECT SUM(total_tokens) FROM project_token_usage WHERE project_id = p.id), 0) as total_tokens,
			       p.created_at, p.updated_at
			FROM projects p
			JOIN users u ON p.owner_id = u.id
			LEFT JOIN llm_configs lc ON p.llm_config_id = lc.id
			JOIN project_members pm ON p.id = pm.project_id AND pm.user_id = ? AND pm.status = 'active'
			ORDER BY p.updated_at DESC
		`
		args = append(args, userID)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]models.Project, 0)
	for rows.Next() {
		var p models.Project
		var desc sql.NullString
		var llmCfgID sql.NullString
		var userRole sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &desc, &p.OwnerID, &p.OwnerName, &p.Status, &llmCfgID, &p.LLMProvider, &p.LLMModel, &userRole, &p.TotalTokens, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			p.Description = desc.String
		}
		if llmCfgID.Valid {
			p.LLMConfigID = &llmCfgID.String
		}
		if userRole.Valid {
			p.UserRole = userRole.String
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (r *Repository) UpdateProject(id, name, description, status string, llmConfigID *string) error {
	query := `UPDATE projects SET name = ?, description = ?, status = ?, llm_config_id = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, name, description, status, llmConfigID, time.Now().UTC(), id)
	return err
}

func (r *Repository) DeleteProject(id string) error {
	_, err := r.db.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

// ----------------- SOURCES -----------------

func (r *Repository) CreateSource(s *models.Source) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	s.CreatedAt = time.Now().UTC()
	if s.Status == "" {
		s.Status = "ready"
	}

	query := `
		INSERT INTO sources (id, project_id, name, type, file_path, original_filename, mime_type, size_bytes, remote_url, remote_type, extracted_text, metadata_json, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, s.ID, s.ProjectID, s.Name, s.Type, s.FilePath, s.OriginalFilename, s.MimeType, s.SizeBytes, s.RemoteURL, s.RemoteType, s.ExtractedText, s.MetadataJSON, s.Status, s.CreatedAt)
	return err
}

func (r *Repository) GetSourceByID(id string) (*models.Source, error) {
	query := `SELECT id, project_id, name, type, file_path, original_filename, mime_type, size_bytes, remote_url, remote_type, extracted_text, metadata_json, status, created_at FROM sources WHERE id = ?`
	row := r.db.QueryRow(query, id)

	var s models.Source
	var fp, of, mt, ru, rt, et, mj sql.NullString
	var sb sql.NullInt64
	err := row.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Type, &fp, &of, &mt, &sb, &ru, &rt, &et, &mj, &s.Status, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	if fp.Valid {
		s.FilePath = fp.String
	}
	if of.Valid {
		s.OriginalFilename = of.String
	}
	if mt.Valid {
		s.MimeType = mt.String
	}
	if sb.Valid {
		s.SizeBytes = sb.Int64
	}
	if ru.Valid {
		s.RemoteURL = ru.String
	}
	if rt.Valid {
		s.RemoteType = rt.String
	}
	if et.Valid {
		s.ExtractedText = et.String
	}
	if mj.Valid {
		s.MetadataJSON = mj.String
	}
	return &s, nil
}

func (r *Repository) ListSourcesByProject(projectID string) ([]models.Source, error) {
	query := `SELECT id, project_id, name, type, file_path, original_filename, mime_type, size_bytes, remote_url, remote_type, extracted_text, metadata_json, status, created_at FROM sources WHERE project_id = ? ORDER BY created_at DESC`
	rows, err := r.db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]models.Source, 0)
	for rows.Next() {
		var s models.Source
		var fp, of, mt, ru, rt, et, mj sql.NullString
		var sb sql.NullInt64
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Type, &fp, &of, &mt, &sb, &ru, &rt, &et, &mj, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		if fp.Valid {
			s.FilePath = fp.String
		}
		if of.Valid {
			s.OriginalFilename = of.String
		}
		if mt.Valid {
			s.MimeType = mt.String
		}
		if sb.Valid {
			s.SizeBytes = sb.Int64
		}
		if ru.Valid {
			s.RemoteURL = ru.String
		}
		if rt.Valid {
			s.RemoteType = rt.String
		}
		if et.Valid {
			s.ExtractedText = et.String
		}
		if mj.Valid {
			s.MetadataJSON = mj.String
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *Repository) DeleteSource(id string) error {
	_, err := r.db.Exec("DELETE FROM sources WHERE id = ?", id)
	return err
}

// ----------------- GENERATIONS -----------------

func (r *Repository) CreateGeneration(g *models.Generation, sourceIDs []string) error {
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	g.CreatedAt = time.Now().UTC()
	g.Status = models.GenerationQueued

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO generations (id, project_id, requested_by, status, template_set, llm_config_id, model_used, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.Exec(query, g.ID, g.ProjectID, g.RequestedBy, g.Status, g.TemplateSet, g.LLMConfigID, g.ModelUsed, g.ErrorMessage, g.CreatedAt)
	if err != nil {
		return err
	}

	for _, sID := range sourceIDs {
		_, err = tx.Exec("INSERT INTO generation_sources (generation_id, source_id) VALUES (?, ?)", g.ID, sID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *Repository) GetGenerationByID(id string) (*models.Generation, error) {
	query := `
		SELECT g.id, g.project_id, g.requested_by, u.name, g.status, g.template_set, g.llm_config_id, g.model_used, g.error_message, g.notification_sent_at, g.system_map_md, g.completeness_scorecard_json,
		       COALESCE((SELECT SUM(total_tokens) FROM project_token_usage WHERE generation_id = g.id), 0) as total_tokens,
		       g.created_at, g.started_at, g.completed_at
		FROM generations g
		JOIN users u ON g.requested_by = u.id
		WHERE g.id = ?
	`
	row := r.db.QueryRow(query, id)

	var g models.Generation
	var llmCfg, model, errMsg, sysMap, scorecard sql.NullString
	var notifSent, started, completed sql.NullTime
	err := row.Scan(&g.ID, &g.ProjectID, &g.RequestedBy, &g.RequesterName, &g.Status, &g.TemplateSet, &llmCfg, &model, &errMsg, &notifSent, &sysMap, &scorecard, &g.TotalTokens, &g.CreatedAt, &started, &completed)
	if err != nil {
		return nil, err
	}
	if llmCfg.Valid {
		g.LLMConfigID = &llmCfg.String
	}
	if model.Valid {
		g.ModelUsed = model.String
	}
	if errMsg.Valid {
		g.ErrorMessage = errMsg.String
	}
	if notifSent.Valid {
		g.NotificationSentAt = &notifSent.Time
	}
	if sysMap.Valid {
		g.SystemMapMD = &sysMap.String
	}
	if scorecard.Valid {
		g.CompletenessScorecardJSON = &scorecard.String
	}
	if started.Valid {
		g.StartedAt = &started.Time
	}
	if completed.Valid {
		g.CompletedAt = &completed.Time
	}

	// Fetch source IDs
	sRows, err := r.db.Query("SELECT source_id FROM generation_sources WHERE generation_id = ?", g.ID)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var sID string
			if err := sRows.Scan(&sID); err == nil {
				g.SourceIDs = append(g.SourceIDs, sID)
			}
		}
	}

	stages, _ := r.ListGenerationStages(g.ID)
	g.Stages = stages

	return &g, nil
}

func (r *Repository) ListGenerationsByProject(projectID string) ([]models.Generation, error) {
	query := `
		SELECT g.id, g.project_id, g.requested_by, u.name, g.status, g.template_set, g.llm_config_id, g.model_used, g.error_message, g.notification_sent_at,
		       COALESCE((SELECT SUM(total_tokens) FROM project_token_usage WHERE generation_id = g.id), 0) as total_tokens,
		       g.created_at, g.started_at, g.completed_at
		FROM generations g
		JOIN users u ON g.requested_by = u.id
		WHERE g.project_id = ?
		ORDER BY g.created_at DESC
	`
	rows, err := r.db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]models.Generation, 0)
	for rows.Next() {
		var g models.Generation
		var llmCfg, model, errMsg sql.NullString
		var notifSent, started, completed sql.NullTime
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.RequestedBy, &g.RequesterName, &g.Status, &g.TemplateSet, &llmCfg, &model, &errMsg, &notifSent, &g.TotalTokens, &g.CreatedAt, &started, &completed); err != nil {
			return nil, err
		}
		if llmCfg.Valid {
			g.LLMConfigID = &llmCfg.String
		}
		if model.Valid {
			g.ModelUsed = model.String
		}
		if errMsg.Valid {
			g.ErrorMessage = errMsg.String
		}
		if notifSent.Valid {
			g.NotificationSentAt = &notifSent.Time
		}
		if started.Valid {
			g.StartedAt = &started.Time
		}
		if completed.Valid {
			g.CompletedAt = &completed.Time
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (r *Repository) ClaimNextQueuedGeneration() (*models.Generation, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := `SELECT id FROM generations WHERE status = 'queued' ORDER BY created_at ASC LIMIT 1`
	var id string
	if err := tx.QueryRow(query).Scan(&id); err != nil {
		return nil, err // sql.ErrNoRows if empty
	}

	now := time.Now().UTC()
	_, err = tx.Exec("UPDATE generations SET status = 'processing', started_at = ? WHERE id = ?", now, id)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetGenerationByID(id)
}

func (r *Repository) UpdateGenerationStatus(id string, status models.GenerationStatus, modelUsed, errMsg string) error {
	now := time.Now().UTC()
	var query string
	var args []any

	if status == models.GenerationCompleted || status == models.GenerationFailed {
		query = `UPDATE generations SET status = ?, model_used = ?, error_message = ?, completed_at = ? WHERE id = ?`
		args = []any{status, modelUsed, errMsg, now, id}
	} else {
		query = `UPDATE generations SET status = ?, model_used = ?, error_message = ? WHERE id = ?`
		args = []any{status, modelUsed, errMsg, id}
	}

	_, err := r.db.Exec(query, args...)
	return err
}

func (r *Repository) MarkGenerationNotificationSent(id string) error {
	_, err := r.db.Exec("UPDATE generations SET notification_sent_at = ? WHERE id = ?", time.Now().UTC(), id)
	return err
}

// ----------------- DOCUMENTS -----------------

func (r *Repository) CreateDocument(doc *models.Document, userID string) error {
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	doc.CreatedAt = now
	doc.UpdatedAt = now
	if doc.Status == "" {
		doc.Status = models.DocStatusDraft
	}

	versionID := uuid.New().String()
	doc.CurrentVersionID = &versionID

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	queryDoc := `
		INSERT INTO documents (id, generation_id, project_id, title, template_type, content_md, status, current_version_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.Exec(queryDoc, doc.ID, doc.GenerationID, doc.ProjectID, doc.Title, doc.TemplateType, doc.ContentMD, doc.Status, doc.CurrentVersionID, doc.CreatedAt, doc.UpdatedAt)
	if err != nil {
		return err
	}

	queryVer := `
		INSERT INTO document_versions (id, document_id, version_number, content_md, change_summary, created_by, created_at)
		VALUES (?, ?, 1, ?, 'Initial generated draft', ?, ?)
	`
	_, err = tx.Exec(queryVer, versionID, doc.ID, doc.ContentMD, userID, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Repository) GetDocumentByID(id string) (*models.Document, error) {
	query := `
		SELECT d.id, d.generation_id, d.project_id, d.title, d.template_type, d.content_md, d.status, d.current_version_id, d.confluence_page_id, d.confluence_page_url, d.created_at, d.updated_at, COALESCE(NULLIF(ct.site_url, ''), ac.site_url, '') AS site_url, ct.space_key
		FROM documents d
		LEFT JOIN confluence_targets ct ON ct.project_id = d.project_id
		LEFT JOIN atlassian_connections ac ON ac.id = ct.connection_id
		WHERE d.id = ?
	`
	row := r.db.QueryRow(query, id)

	var d models.Document
	var genID, curVerID, confPageID, confPageURL, ctSiteURL, ctSpaceKey sql.NullString
	err := row.Scan(&d.ID, &genID, &d.ProjectID, &d.Title, &d.TemplateType, &d.ContentMD, &d.Status, &curVerID, &confPageID, &confPageURL, &d.CreatedAt, &d.UpdatedAt, &ctSiteURL, &ctSpaceKey)
	if err != nil {
		return nil, err
	}
	if genID.Valid {
		d.GenerationID = &genID.String
	}
	if curVerID.Valid {
		d.CurrentVersionID = &curVerID.String
	}
	if confPageID.Valid {
		d.ConfluencePageID = &confPageID.String
	}
	if confPageURL.Valid && confPageURL.String != "" {
		d.ConfluencePageURL = &confPageURL.String
	} else if confPageID.Valid && confPageID.String != "" && ctSiteURL.Valid && ctSiteURL.String != "" {
		computedURL := models.FormatConfluencePageURL(ctSiteURL.String, ctSpaceKey.String, confPageID.String, "")
		d.ConfluencePageURL = &computedURL
	}

	// Fetch versions
	vRows, err := r.db.Query(`
		SELECT v.id, v.document_id, v.version_number, v.content_md, v.change_summary, v.created_by, u.name, v.created_at
		FROM document_versions v
		JOIN users u ON v.created_by = u.id
		WHERE v.document_id = ?
		ORDER BY v.version_number DESC
	`, d.ID)
	if err == nil {
		for vRows.Next() {
			var v models.DocumentVersion
			var cs sql.NullString
			if err := vRows.Scan(&v.ID, &v.DocumentID, &v.VersionNumber, &v.ContentMD, &cs, &v.CreatedBy, &v.AuthorName, &v.CreatedAt); err == nil {
				if cs.Valid {
					v.ChangeSummary = cs.String
				}
				d.Versions = append(d.Versions, v)
			}
		}
		vRows.Close()
	}

	// Fetch feedback
	fbRows, err := r.db.Query(`
		SELECT f.id, f.document_id, f.user_id, u.name, u.email, f.section_anchor, f.category, f.content, f.is_blocking, f.status, f.created_at, f.updated_at
		FROM document_feedback f
		JOIN users u ON f.user_id = u.id
		WHERE f.document_id = ?
		ORDER BY f.created_at DESC
	`, d.ID)
	if err == nil {
		for fbRows.Next() {
			var f models.DocumentFeedback
			var secAnchor sql.NullString
			var isBlocking int
			if err := fbRows.Scan(&f.ID, &f.DocumentID, &f.UserID, &f.AuthorName, &f.AuthorEmail, &secAnchor, &f.Category, &f.Content, &isBlocking, &f.Status, &f.CreatedAt, &f.UpdatedAt); err == nil {
				if secAnchor.Valid {
					f.SectionAnchor = secAnchor.String
				}
				f.IsBlocking = isBlocking == 1
				d.Feedback = append(d.Feedback, f)
			}
		}
		fbRows.Close()
	}

	if d.Versions == nil {
		d.Versions = make([]models.DocumentVersion, 0)
	}
	if d.Feedback == nil {
		d.Feedback = make([]models.DocumentFeedback, 0)
	}

	return &d, nil
}

func (r *Repository) ListDocumentsByProject(projectID string) ([]models.Document, error) {
	query := `
		SELECT d.id, d.generation_id, d.project_id, d.title, d.template_type, d.content_md, d.status, d.current_version_id, d.confluence_page_id, d.confluence_page_url, d.created_at, d.updated_at, COALESCE(NULLIF(ct.site_url, ''), ac.site_url, '') AS site_url, ct.space_key
		FROM documents d
		LEFT JOIN confluence_targets ct ON ct.project_id = d.project_id
		LEFT JOIN atlassian_connections ac ON ac.id = ct.connection_id
		WHERE d.project_id = ?
		ORDER BY d.updated_at DESC
	`
	rows, err := r.db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]models.Document, 0)
	for rows.Next() {
		var d models.Document
		var genID, curVerID, confPageID, confPageURL, ctSiteURL, ctSpaceKey sql.NullString
		if err := rows.Scan(&d.ID, &genID, &d.ProjectID, &d.Title, &d.TemplateType, &d.ContentMD, &d.Status, &curVerID, &confPageID, &confPageURL, &d.CreatedAt, &d.UpdatedAt, &ctSiteURL, &ctSpaceKey); err != nil {
			return nil, err
		}
		if genID.Valid {
			d.GenerationID = &genID.String
		}
		if curVerID.Valid {
			d.CurrentVersionID = &curVerID.String
		}
		if confPageID.Valid {
			d.ConfluencePageID = &confPageID.String
		}
		if confPageURL.Valid && confPageURL.String != "" {
			d.ConfluencePageURL = &confPageURL.String
		} else if confPageID.Valid && confPageID.String != "" && ctSiteURL.Valid && ctSiteURL.String != "" {
			computedURL := models.FormatConfluencePageURL(ctSiteURL.String, ctSpaceKey.String, confPageID.String, "")
			d.ConfluencePageURL = &computedURL
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func (r *Repository) UpdateDocumentContent(id, contentMD, title, userID, changeSummary string) (*models.DocumentVersion, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var maxVer int
	err = tx.QueryRow("SELECT COALESCE(MAX(version_number), 0) FROM document_versions WHERE document_id = ?", id).Scan(&maxVer)
	if err != nil {
		return nil, err
	}

	newVerNumber := maxVer + 1
	newVerID := uuid.New().String()
	now := time.Now().UTC()

	// Insert version
	_, err = tx.Exec(`
		INSERT INTO document_versions (id, document_id, version_number, content_md, change_summary, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, newVerID, id, newVerNumber, contentMD, changeSummary, userID, now)
	if err != nil {
		return nil, err
	}

	// Update document
	_, err = tx.Exec(`
		UPDATE documents
		SET content_md = ?, title = CASE WHEN ? != '' THEN ? ELSE title END, current_version_id = ?, updated_at = ?
		WHERE id = ?
	`, contentMD, title, title, newVerID, now, id)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &models.DocumentVersion{
		ID:            newVerID,
		DocumentID:    id,
		VersionNumber: newVerNumber,
		ContentMD:     contentMD,
		ChangeSummary: changeSummary,
		CreatedBy:     userID,
		CreatedAt:     now,
	}, nil
}

func (r *Repository) UpdateDocumentStatus(id string, status models.DocumentStatus) error {
	_, err := r.db.Exec("UPDATE documents SET status = ?, updated_at = ? WHERE id = ?", status, time.Now().UTC(), id)
	return err
}

func (r *Repository) UpdateDocumentConfluencePage(id, pageID, pageURL string) error {
	_, err := r.db.Exec("UPDATE documents SET confluence_page_id = ?, confluence_page_url = ?, updated_at = ? WHERE id = ?", pageID, pageURL, time.Now().UTC(), id)
	return err
}

// ----------------- FEEDBACK -----------------

func (r *Repository) CreateFeedback(f *models.DocumentFeedback) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now
	if f.Status == "" {
		f.Status = models.FeedbackOpen
	}

	query := `
		INSERT INTO document_feedback (id, document_id, user_id, section_anchor, category, content, is_blocking, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, f.ID, f.DocumentID, f.UserID, f.SectionAnchor, f.Category, f.Content, f.IsBlocking, f.Status, f.CreatedAt, f.UpdatedAt)
	return err
}

func (r *Repository) UpdateFeedbackStatus(id string, status models.FeedbackStatus) error {
	_, err := r.db.Exec("UPDATE document_feedback SET status = ?, updated_at = ? WHERE id = ?", status, time.Now().UTC(), id)
	return err
}

func (r *Repository) CountOpenBlockingFeedback(documentID string) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM document_feedback WHERE document_id = ? AND is_blocking = 1 AND status = 'open'`
	err := r.db.QueryRow(query, documentID).Scan(&count)
	return count, err
}

func (r *Repository) MarkOpenFeedbackAddressed(documentID string) error {
	query := `UPDATE document_feedback SET status = 'addressed', updated_at = ? WHERE document_id = ? AND status = 'open'`
	_, err := r.db.Exec(query, time.Now().UTC(), documentID)
	return err
}

func (r *Repository) ListDocumentFeedback(documentID string) ([]models.DocumentFeedback, error) {
	doc, err := r.GetDocumentByID(documentID)
	if err != nil {
		return nil, err
	}
	return doc.Feedback, nil
}

// ----------------- ATLASSIAN -----------------

func (r *Repository) UpsertAtlassianConnection(conn *models.AtlassianConnection) error {
	if conn.ID == "" {
		existing, _ := r.GetAtlassianConnectionByUserID(conn.UserID)
		if existing != nil {
			conn.ID = existing.ID
		} else {
			conn.ID = uuid.New().String()
		}
	}
	now := time.Now().UTC()
	conn.CreatedAt = now
	conn.UpdatedAt = now

	query := `
		INSERT INTO atlassian_connections (id, user_id, access_token_encrypted, refresh_token_encrypted, token_expiry, cloud_id, site_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			access_token_encrypted = excluded.access_token_encrypted,
			refresh_token_encrypted = excluded.refresh_token_encrypted,
			token_expiry = excluded.token_expiry,
			cloud_id = excluded.cloud_id,
			site_url = excluded.site_url,
			updated_at = excluded.updated_at
	`
	_, err := r.db.Exec(query, conn.ID, conn.UserID, conn.AccessTokenEncrypted, conn.RefreshTokenEncrypted, conn.TokenExpiry, conn.CloudID, conn.SiteURL, conn.CreatedAt, conn.UpdatedAt)
	return err
}

func (r *Repository) GetAtlassianConnectionByUserID(userID string) (*models.AtlassianConnection, error) {
	query := `SELECT id, user_id, access_token_encrypted, refresh_token_encrypted, token_expiry, cloud_id, site_url, created_at, updated_at FROM atlassian_connections WHERE user_id = ? ORDER BY updated_at DESC LIMIT 1`
	row := r.db.QueryRow(query, userID)

	var c models.AtlassianConnection
	var refToken, cloudID, siteURL sql.NullString
	var expiry sql.NullTime
	err := row.Scan(&c.ID, &c.UserID, &c.AccessTokenEncrypted, &refToken, &expiry, &cloudID, &siteURL, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if refToken.Valid {
		c.RefreshTokenEncrypted = refToken.String
	}
	if cloudID.Valid {
		c.CloudID = cloudID.String
	}
	if siteURL.Valid {
		c.SiteURL = siteURL.String
	}
	if expiry.Valid {
		c.TokenExpiry = &expiry.Time
	}
	return &c, nil
}

func (r *Repository) UpsertConfluenceTarget(t *models.ConfluenceTarget) error {
	existing, _ := r.GetConfluenceTargetByProject(t.ProjectID)
	now := time.Now().UTC()
	t.UpdatedAt = &now

	connID := t.ConnectionID
	if connID == "" {
		connID = "conn_" + t.ProjectID
	}

	if existing != nil {
		t.ID = existing.ID
		if t.CloudID == "" {
			t.CloudID = existing.CloudID
		}
		if t.SiteURL == "" {
			t.SiteURL = existing.SiteURL
		}
		if t.SiteName == "" {
			t.SiteName = existing.SiteName
		}
		query := `
			UPDATE confluence_targets
			SET cloud_id = ?, site_url = ?, site_name = ?, space_key = ?, space_name = ?, parent_page_id = ?, parent_page_title = ?, updated_by = ?, updated_at = ?
			WHERE id = ?
		`
		_, err := r.db.Exec(query, t.CloudID, t.SiteURL, t.SiteName, t.SpaceKey, t.SpaceName, t.ParentPageID, t.ParentPageTitle, t.UpdatedBy, t.UpdatedAt, t.ID)
		return err
	}

	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	query := `
		INSERT INTO confluence_targets (id, project_id, connection_id, cloud_id, site_url, site_name, space_key, space_name, parent_page_id, parent_page_title, updated_by, updated_at, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	`
	_, err := r.db.Exec(query, t.ID, t.ProjectID, connID, t.CloudID, t.SiteURL, t.SiteName, t.SpaceKey, t.SpaceName, t.ParentPageID, t.ParentPageTitle, t.UpdatedBy, t.UpdatedAt)
	return err
}

func (r *Repository) GetConfluenceTargetByProject(projectID string) (*models.ConfluenceTarget, error) {
	query := `
		SELECT ct.id, ct.project_id, ct.connection_id, ct.cloud_id, ct.site_url, ct.site_name, ct.space_key, ct.space_name, ct.parent_page_id, ct.parent_page_title, ct.updated_by, u.name, ct.updated_at, ct.is_default
		FROM confluence_targets ct
		LEFT JOIN users u ON ct.updated_by = u.id
		WHERE ct.project_id = ? LIMIT 1
	`
	row := r.db.QueryRow(query, projectID)

	var t models.ConfluenceTarget
	var connID, cloudID, siteURL, siteName, parentPageID, parentPageTitle, updatedBy, updaterName sql.NullString
	var updatedAt sql.NullTime
	var isDefault int
	err := row.Scan(&t.ID, &t.ProjectID, &connID, &cloudID, &siteURL, &siteName, &t.SpaceKey, &t.SpaceName, &parentPageID, &parentPageTitle, &updatedBy, &updaterName, &updatedAt, &isDefault)
	if err != nil {
		return nil, err
	}
	if connID.Valid {
		t.ConnectionID = connID.String
	}
	if cloudID.Valid {
		t.CloudID = cloudID.String
	}
	if siteURL.Valid {
		t.SiteURL = siteURL.String
	}
	if siteName.Valid {
		t.SiteName = siteName.String
	}
	if parentPageID.Valid {
		t.ParentPageID = parentPageID.String
	}
	if parentPageTitle.Valid {
		t.ParentPageTitle = parentPageTitle.String
	}
	if updatedBy.Valid {
		t.UpdatedBy = &updatedBy.String
	}
	if updaterName.Valid {
		t.UpdaterName = updaterName.String
	}
	if updatedAt.Valid {
		t.UpdatedAt = &updatedAt.Time
	}
	t.IsDefault = isDefault == 1
	return &t, nil
}

func (r *Repository) DeleteConfluenceTarget(projectID string) error {
	_, err := r.db.Exec("DELETE FROM confluence_targets WHERE project_id = ?", projectID)
	return err
}

// ----------------- PROJECT MEMBERS -----------------

func (r *Repository) AddProjectMember(pm *models.ProjectMember) error {
	if pm.ID == "" {
		pm.ID = uuid.New().String()
	}
	pm.CreatedAt = time.Now().UTC()
	if pm.Status == "" {
		pm.Status = models.ProjectMemberStatusActive
	}
	query := `
		INSERT INTO project_members (id, project_id, user_id, user_email, role, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, user_email) DO UPDATE SET
			role = excluded.role,
			status = excluded.status,
			user_id = COALESCE(excluded.user_id, project_members.user_id)
	`
	_, err := r.db.Exec(query, pm.ID, pm.ProjectID, pm.UserID, pm.UserEmail, pm.Role, pm.Status, pm.CreatedAt)
	return err
}

func (r *Repository) ListProjectMembers(projectID string) ([]models.ProjectMember, error) {
	query := `
		SELECT pm.id, pm.project_id, pm.user_id, COALESCE(u.name, ''), pm.user_email, pm.role, pm.status, pm.created_at
		FROM project_members pm
		LEFT JOIN users u ON pm.user_id = u.id
		WHERE pm.project_id = ?
		ORDER BY pm.role ASC, pm.created_at ASC
	`
	rows, err := r.db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]models.ProjectMember, 0)
	for rows.Next() {
		var pm models.ProjectMember
		var uID, uName sql.NullString
		if err := rows.Scan(&pm.ID, &pm.ProjectID, &uID, &uName, &pm.UserEmail, &pm.Role, &pm.Status, &pm.CreatedAt); err != nil {
			return nil, err
		}
		if uID.Valid {
			pm.UserID = &uID.String
		}
		if uName.Valid {
			pm.UserName = uName.String
		}
		members = append(members, pm)
	}
	return members, rows.Err()
}

func (r *Repository) DeleteProjectMember(memberID, projectID string) error {
	_, err := r.db.Exec("DELETE FROM project_members WHERE id = ? AND project_id = ? AND role != 'owner'", memberID, projectID)
	return err
}

func (r *Repository) IsProjectMemberOrOwner(projectID, userID string) (bool, models.ProjectMemberRole, error) {
	query := `SELECT role FROM project_members WHERE project_id = ? AND user_id = ? AND status = 'active' LIMIT 1`
	var role models.ProjectMemberRole
	err := r.db.QueryRow(query, projectID, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, role, nil
}

func (r *Repository) ClaimPendingProjectMemberships(email, userID string) error {
	query := `UPDATE project_members SET user_id = ?, status = 'active' WHERE LOWER(user_email) = LOWER(?) AND (user_id IS NULL OR user_id = '')`
	_, err := r.db.Exec(query, userID, email)
	return err
}

// ----------------- NOTIFICATIONS -----------------

func (r *Repository) UpsertNotificationPreference(p *models.NotificationPreference) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	p.CreatedAt = time.Now().UTC()

	query := `
		INSERT INTO notification_preferences (id, user_id, project_id, channel, enabled, teams_webhook_url_encrypted, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			enabled = excluded.enabled,
			teams_webhook_url_encrypted = excluded.teams_webhook_url_encrypted
	`
	_, err := r.db.Exec(query, p.ID, p.UserID, p.ProjectID, p.Channel, p.Enabled, p.TeamsWebhookURLEncrypted, p.CreatedAt)
	return err
}

func (r *Repository) ListNotificationPreferences(userID string, projectID *string) ([]models.NotificationPreference, error) {
	var query string
	var args []any
	if projectID != nil && *projectID != "" {
		query = `SELECT id, user_id, project_id, channel, enabled, teams_webhook_url_encrypted, created_at FROM notification_preferences WHERE user_id = ? AND (project_id = ? OR project_id IS NULL)`
		args = []any{userID, *projectID}
	} else {
		query = `SELECT id, user_id, project_id, channel, enabled, teams_webhook_url_encrypted, created_at FROM notification_preferences WHERE user_id = ?`
		args = []any{userID}
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]models.NotificationPreference, 0)
	for rows.Next() {
		var np models.NotificationPreference
		var pID, whEnc sql.NullString
		var enabled int
		if err := rows.Scan(&np.ID, &np.UserID, &pID, &np.Channel, &enabled, &whEnc, &np.CreatedAt); err != nil {
			return nil, err
		}
		if pID.Valid {
			np.ProjectID = &pID.String
		}
		if whEnc.Valid {
			np.TeamsWebhookURLEncrypted = whEnc.String
		}
		np.Enabled = enabled == 1
		list = append(list, np)
	}
	return list, rows.Err()
}
