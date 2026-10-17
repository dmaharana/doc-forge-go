package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"docforge/internal/config"
	"docforge/internal/crypto"
	"docforge/internal/ingestion"
	"docforge/internal/models"
	"docforge/internal/repository"
	"docforge/internal/service"
)

type Handlers struct {
	cfg       *config.Config
	repo      *repository.Repository
	storage   ingestion.Storage
	genSvc    *service.GenerationService
	atlassian *service.AtlassianService
}

func NewHandlers(
	cfg *config.Config,
	repo *repository.Repository,
	storage ingestion.Storage,
	genSvc *service.GenerationService,
	atlassian *service.AtlassianService,
) *Handlers {
	return &Handlers{
		cfg:       cfg,
		repo:      repo,
		storage:   storage,
		genSvc:    genSvc,
		atlassian: atlassian,
	}
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// ----------------- ADMIN USERS -----------------

func (h *Handlers) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.ListUsers()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, users)
}

type CreateUserRequest struct {
	Email    string          `json:"email"`
	Name     string          `json:"name"`
	Role     models.UserRole `json:"role"`
	Password string          `json:"password"`
}

func (h *Handlers) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "name, email, and password required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	role := req.Role
	if role != models.RoleAdmin && role != models.RoleUser {
		role = models.RoleUser
	}

	user := &models.User{
		ID:           uuid.New().String(),
		Email:        req.Email,
		Name:         req.Name,
		Role:         role,
		PasswordHash: string(hash),
		IsActive:     true,
	}

	if err := h.repo.CreateUser(user); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to create user: %v", err)})
		return
	}

	respondJSON(w, http.StatusCreated, user)
}

type UpdateUserRequest struct {
	Name     *string `json:"name,omitempty"`
	Role     *string `json:"role,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

func (h *Handlers) AdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	currentUser := GetCurrentUser(r)

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	targetUser, err := h.repo.GetUserByID(id)
	if err != nil || targetUser == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	name := targetUser.Name
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}

	role := string(targetUser.Role)
	if req.Role != nil && *req.Role != "" {
		newRole := *req.Role
		if newRole != string(models.RoleAdmin) && newRole != string(models.RoleUser) {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role: must be 'admin' or 'user'"})
			return
		}

		// Prevent last remaining active admin from demoting themselves to user
		if targetUser.ID == currentUser.ID && newRole == string(models.RoleUser) {
			allUsers, err := h.repo.ListUsers()
			if err == nil {
				otherAdmins := 0
				for _, u := range allUsers {
					if u.ID != currentUser.ID && u.Role == models.RoleAdmin && u.IsActive {
						otherAdmins++
					}
				}
				if otherAdmins == 0 {
					respondJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot demote the only remaining active administrator"})
					return
				}
			}
		}
		role = newRole
	}

	isActive := targetUser.IsActive
	if req.IsActive != nil {
		// Prevent last remaining active admin from deactivating themselves
		if targetUser.ID == currentUser.ID && !*req.IsActive {
			allUsers, err := h.repo.ListUsers()
			if err == nil {
				otherAdmins := 0
				for _, u := range allUsers {
					if u.ID != currentUser.ID && u.Role == models.RoleAdmin && u.IsActive {
						otherAdmins++
					}
				}
				if otherAdmins == 0 {
					respondJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot deactivate the only remaining active administrator"})
					return
				}
			}
		}
		isActive = *req.IsActive
	}

	if err := h.repo.UpdateUser(id, name, role, isActive); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	user, err := h.repo.GetUserByID(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	respondJSON(w, http.StatusOK, user)
}

// ----------------- ADMIN LLM CONFIGS -----------------

func (h *Handlers) AdminListLLMConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.repo.ListLLMConfigs()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	for i := range configs {
		if configs[i].APIKeyEncrypted != "" {
			configs[i].APIKeyMasked = "••••••••••••"
		}
	}
	respondJSON(w, http.StatusOK, configs)
}

func (h *Handlers) ListAvailableLLMConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.repo.ListLLMConfigs()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	for i := range configs {
		if configs[i].APIKeyEncrypted != "" {
			configs[i].APIKeyMasked = "••••••••••••"
		}
	}
	respondJSON(w, http.StatusOK, configs)
}

type SaveLLMConfigRequest struct {
	ID           string  `json:"id"`
	Provider     string  `json:"provider"`
	APIKey       string  `json:"api_key"`
	BaseURL      string  `json:"base_url"`
	DefaultModel string  `json:"default_model"`
	MaxTokens    int     `json:"max_tokens"`
	Temperature  float64 `json:"temperature"`
	IsActive     bool    `json:"is_active"`
}

func (h *Handlers) AdminSaveLLMConfig(w http.ResponseWriter, r *http.Request) {
	var req SaveLLMConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	var encKey string
	if req.APIKey != "" {
		var err error
		encKey, err = crypto.Encrypt(req.APIKey, h.cfg.EncryptionKey)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt api key"})
			return
		}
	} else if req.ID != "" {
		// Preserve existing encrypted key if not updated
		list, _ := h.repo.ListLLMConfigs()
		for _, c := range list {
			if c.ID == req.ID {
				encKey = c.APIKeyEncrypted
				break
			}
		}
	} else if req.IsActive {
		// If ID was not provided but we're activating, preserve key from current active config if provider/baseURL match
		if active, err := h.repo.GetActiveLLMConfig(); err == nil && active != nil && active.APIKeyEncrypted != "" {
			if req.Provider == active.Provider {
				encKey = active.APIKeyEncrypted
			}
		}
	}

	cfg := &models.LLMConfig{
		ID:              req.ID,
		Provider:        req.Provider,
		APIKeyEncrypted: encKey,
		BaseURL:         req.BaseURL,
		DefaultModel:    req.DefaultModel,
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		IsActive:        req.IsActive,
	}

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}

	if err := h.repo.SaveLLMConfig(cfg); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	cfg.APIKeyMasked = "••••••••••••"
	respondJSON(w, http.StatusOK, cfg)
}

// ----------------- PROJECTS -----------------

func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	isAdmin := user.Role == models.RoleAdmin

	projects, err := h.repo.ListProjects(user.ID, isAdmin)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, projects)
}

type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	LLMConfigID *string `json:"llm_config_id,omitempty"`
}

func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if req.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "project name required"})
		return
	}

	var llmCfgID *string
	if req.LLMConfigID != nil && *req.LLMConfigID != "" && *req.LLMConfigID != "default" {
		llmCfgID = req.LLMConfigID
	}

	p := &models.Project{
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     user.ID,
		OwnerName:   user.Name,
		Status:      "active",
		LLMConfigID: llmCfgID,
	}

	if err := h.repo.CreateProject(p); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Re-fetch created project to populate associated LLM metadata if set
	if created, err := h.repo.GetProjectByID(p.ID); err == nil && created != nil {
		p = created
	}

	respondJSON(w, http.StatusCreated, p)
}

func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	p, err := h.repo.GetProjectByID(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(p.ID, user.ID)
	if user.Role != models.RoleAdmin && !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: you are not a member of this project"})
		return
	}

	p.UserRole = string(role)
	if user.Role == models.RoleAdmin && p.UserRole == "" {
		p.UserRole = "admin"
	}

	respondJSON(w, http.StatusOK, p)
}

func (h *Handlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(id, user.ID)
	if user.Role != models.RoleAdmin && (!isMember || role != models.ProjectMemberRoleOwner) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: only project owner or administrator can modify project settings"})
		return
	}

	p, err := h.repo.GetProjectByID(id)
	if err != nil || p == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		LLMConfigID *string `json:"llm_config_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	name := p.Name
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}
	desc := p.Description
	if req.Description != nil {
		desc = *req.Description
	}
	status := p.Status
	if req.Status != nil && *req.Status != "" {
		status = *req.Status
	}

	llmConfigID := p.LLMConfigID
	if req.LLMConfigID != nil {
		if *req.LLMConfigID == "" || *req.LLMConfigID == "default" {
			llmConfigID = nil
		} else {
			llmConfigID = req.LLMConfigID
		}
	}

	if err := h.repo.UpdateProject(id, name, desc, status, llmConfigID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	updatedProject, _ := h.repo.GetProjectByID(id)
	if updatedProject != nil {
		updatedProject.UserRole = string(role)
		respondJSON(w, http.StatusOK, updatedProject)
		return
	}
	p.Name = name
	p.Description = desc
	p.Status = status
	p.LLMConfigID = llmConfigID
	p.UserRole = string(role)
	respondJSON(w, http.StatusOK, p)
}

func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(id, user.ID)
	if user.Role != models.RoleAdmin && (!isMember || role != models.ProjectMemberRoleOwner) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: only project owner or administrator can delete project"})
		return
	}

	if err := h.repo.DeleteProject(id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ----------------- PROJECT MEMBERS -----------------

func (h *Handlers) ListProjectMembers(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, _, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: you are not a member of this project"})
		return
	}

	members, err := h.repo.ListProjectMembers(projectID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, members)
}

type AddProjectMemberRequest struct {
	Email string                   `json:"email"`
	Role  models.ProjectMemberRole `json:"role"`
}

func (h *Handlers) AddProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && (!isMember || role != models.ProjectMemberRoleOwner) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: only project owner can add members"})
		return
	}

	var req AddProjectMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if req.Email == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}
	if req.Role == "" {
		req.Role = models.ProjectMemberRoleMember
	}

	memberRole := req.Role
	pm := &models.ProjectMember{
		ProjectID: projectID,
		UserEmail: strings.TrimSpace(strings.ToLower(req.Email)),
		Role:      memberRole,
		Status:    models.ProjectMemberStatusPending,
	}

	existingUser, err := h.repo.GetUserByEmail(pm.UserEmail)
	if err == nil && existingUser != nil {
		pm.UserID = &existingUser.ID
		pm.UserName = existingUser.Name
		pm.Status = models.ProjectMemberStatusActive
	}

	if err := h.repo.AddProjectMember(pm); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, pm)
}

func (h *Handlers) DeleteProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")
	user := GetCurrentUser(r)

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && (!isMember || role != models.ProjectMemberRoleOwner) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: only project owner can remove members"})
		return
	}

	if err := h.repo.DeleteProjectMember(memberID, projectID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ----------------- SOURCES -----------------

func (h *Handlers) ListSources(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	sources, err := h.repo.ListSourcesByProject(projectID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, sources)
}

func (h *Handlers) UploadSourceFile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	// Limit multipart body to 32MB
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "file too large or invalid multipart"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file field"})
		return
	}
	defer file.Close()

	// Save to storage
	filePath, sizeBytes, err := h.storage.Save(header.Filename, file)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("storage error: %v", err)})
		return
	}

	// Extract text
	extractRes, err := ingestion.ExtractFromFile(filePath, header.Filename)
	var extractedText, metaJSON, mimeType string
	if err != nil {
		extractedText = fmt.Sprintf("[Extraction Notice: %v]", err)
	} else {
		extractedText = extractRes.Text
		metaJSON = extractRes.MetadataJSON
		mimeType = extractRes.MimeType
	}

	source := &models.Source{
		ProjectID:        projectID,
		Name:             header.Filename,
		Type:             models.SourceTypeUpload,
		FilePath:         filePath,
		OriginalFilename: header.Filename,
		MimeType:         mimeType,
		SizeBytes:        sizeBytes,
		ExtractedText:    extractedText,
		MetadataJSON:     metaJSON,
		Status:           "ready",
	}

	if err := h.repo.CreateSource(source); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if extractRes != nil && len(extractRes.Chunks) > 0 {
		for i := range extractRes.Chunks {
			extractRes.Chunks[i].SourceID = source.ID
			extractRes.Chunks[i].ProjectID = projectID
		}
		_ = h.repo.SaveSourceChunks(extractRes.Chunks)
	}

	respondJSON(w, http.StatusCreated, source)
}

type AddRemoteSourceRequest struct {
	Name       string `json:"name"`
	RemoteURL  string `json:"remote_url"`
	RemoteType string `json:"remote_type"` // git, http, local, s3, etc.
}

func (h *Handlers) AddRemoteSource(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	var req AddRemoteSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if req.RemoteURL == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "remote_url required"})
		return
	}

	name := req.Name
	if name == "" {
		name = req.RemoteURL
	}

	extractRes, err := ingestion.FetchRemoteSource(r.Context(), req.RemoteURL, req.RemoteType)
	var extractedText, metaJSON, mimeType string
	if err != nil {
		extractedText = fmt.Sprintf("[Remote Fetch Notice: %v]", err)
		mimeType = "text/plain"
	} else {
		extractedText = extractRes.Text
		metaJSON = extractRes.MetadataJSON
		mimeType = extractRes.MimeType
	}

	source := &models.Source{
		ProjectID:     projectID,
		Name:          name,
		Type:          models.SourceTypeRemote,
		RemoteURL:     req.RemoteURL,
		RemoteType:    req.RemoteType,
		MimeType:      mimeType,
		ExtractedText: extractedText,
		MetadataJSON:  metaJSON,
		Status:        "ready",
	}

	if err := h.repo.CreateSource(source); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if extractRes != nil && len(extractRes.Chunks) > 0 {
		for i := range extractRes.Chunks {
			extractRes.Chunks[i].SourceID = source.ID
			extractRes.Chunks[i].ProjectID = projectID
		}
		_ = h.repo.SaveSourceChunks(extractRes.Chunks)
	}

	respondJSON(w, http.StatusCreated, source)
}

func (h *Handlers) DeleteSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteSource(id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ----------------- GENERATIONS -----------------

func (h *Handlers) ListGenerations(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	list, err := h.repo.ListGenerationsByProject(projectID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, list)
}

type RequestGenerationPayload struct {
	TemplateSet string   `json:"template_set"` // e.g. "product_spec,adr,release_notes,runbook"
	SourceIDs   []string `json:"source_ids"`
	LLMConfigID *string  `json:"llm_config_id,omitempty"`
}

func (h *Handlers) SubmitGeneration(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	var req RequestGenerationPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if req.TemplateSet == "" {
		req.TemplateSet = "product_spec,adr,release_notes,runbook"
	}

	llmConfigID := req.LLMConfigID
	if llmConfigID == nil || *llmConfigID == "" || *llmConfigID == "default" {
		if p, err := h.repo.GetProjectByID(projectID); err == nil && p != nil && p.LLMConfigID != nil {
			llmConfigID = p.LLMConfigID
		} else {
			llmConfigID = nil
		}
	}

	gen := &models.Generation{
		ProjectID:   projectID,
		RequestedBy: user.ID,
		TemplateSet: req.TemplateSet,
		LLMConfigID: llmConfigID,
	}

	if err := h.repo.CreateGeneration(gen, req.SourceIDs); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusAccepted, gen)
}

func (h *Handlers) GetGeneration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gen, err := h.repo.GetGenerationByID(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "generation not found"})
		return
	}
	respondJSON(w, http.StatusOK, gen)
}

// ----------------- DOCUMENTS -----------------

func (h *Handlers) ListDocuments(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	docs, err := h.repo.ListDocumentsByProject(projectID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, docs)
}

func (h *Handlers) GetDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	doc, err := h.repo.GetDocumentByID(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

type UpdateDocumentContentRequest struct {
	Title         string `json:"title"`
	ContentMD     string `json:"content_md"`
	ChangeSummary string `json:"change_summary"`
}

func (h *Handlers) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	var req UpdateDocumentContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	summary := req.ChangeSummary
	if summary == "" {
		summary = "Manual edit by reviewer"
	}

	newVersion, err := h.repo.UpdateDocumentContent(id, req.ContentMD, req.Title, user.ID, summary)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	doc, _ := h.repo.GetDocumentByID(id)
	respondJSON(w, http.StatusOK, map[string]any{
		"document":    doc,
		"new_version": newVersion,
	})
}

// ApproveDocument implements the hard gate check
func (h *Handlers) ApproveDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Hard Gate: Prevent approval if any is_blocking feedback is open!
	blockingCount, err := h.repo.CountOpenBlockingFeedback(id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if blockingCount > 0 {
		respondJSON(w, http.StatusConflict, map[string]any{
			"error":          fmt.Sprintf("cannot approve document: %d blocking feedback item(s) are still open", blockingCount),
			"blocking_count": blockingCount,
		})
		return
	}

	if err := h.repo.UpdateDocumentStatus(id, models.DocStatusApproved); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	doc, _ := h.repo.GetDocumentByID(id)
	respondJSON(w, http.StatusOK, doc)
}

func (h *Handlers) DownloadDocumentMarkdown(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	doc, err := h.repo.GetDocumentByID(id)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}

	filename := fmt.Sprintf("%s.md", sanitizeFilename(doc.Title))
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write([]byte(doc.ContentMD))
}

func sanitizeFilename(s string) string {
	clean := strings.ReplaceAll(s, " ", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, "\\", "_")
	return clean
}

// ----------------- FEEDBACK -----------------

type CreateFeedbackRequest struct {
	SectionAnchor string                  `json:"section_anchor"`
	Category      models.FeedbackCategory `json:"category"`
	Content       string                  `json:"content"`
	IsBlocking    bool                    `json:"is_blocking"`
}

func (h *Handlers) AddFeedback(w http.ResponseWriter, r *http.Request) {
	documentID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	var req CreateFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if req.Content == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "feedback content required"})
		return
	}

	fb := &models.DocumentFeedback{
		DocumentID:    documentID,
		UserID:        user.ID,
		SectionAnchor: req.SectionAnchor,
		Category:      req.Category,
		Content:       req.Content,
		IsBlocking:    req.IsBlocking,
		Status:        models.FeedbackOpen,
	}

	if err := h.repo.CreateFeedback(fb); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	fb.AuthorName = user.Name
	fb.AuthorEmail = user.Email
	respondJSON(w, http.StatusCreated, fb)
}

type UpdateFeedbackStatusRequest struct {
	Status models.FeedbackStatus `json:"status"` // open, addressed, dismissed
}

func (h *Handlers) UpdateFeedbackStatus(w http.ResponseWriter, r *http.Request) {
	fbID := chi.URLParam(r, "fbId")

	var req UpdateFeedbackStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if err := h.repo.UpdateFeedbackStatus(fbID, req.Status); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": string(req.Status)})
}

// UpdateWithFeedback triggers AI revision
func (h *Handlers) UpdateWithFeedback(w http.ResponseWriter, r *http.Request) {
	documentID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	newVersion, err := h.genSvc.UpdateWithFeedback(r.Context(), documentID, user.ID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	doc, _ := h.repo.GetDocumentByID(documentID)
	respondJSON(w, http.StatusOK, map[string]any{
		"message":     "Document updated successfully with feedback",
		"document":    doc,
		"new_version": newVersion,
	})
}

// ----------------- ATLASSIAN & CONFLUENCE -----------------

func (h *Handlers) AtlassianConnect(w http.ResponseWriter, r *http.Request) {
	state := uuid.New().String()
	authURL := h.atlassian.GetAuthorizationURL(state)
	respondJSON(w, http.StatusOK, map[string]string{
		"authorization_url": authURL,
		"is_mock_mode":      fmt.Sprintf("%t", h.atlassian.IsMockMode()),
	})
}

func (h *Handlers) AtlassianCallback(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	code := r.URL.Query().Get("code")
	if code == "" {
		code = "mock_atlassian_code"
	}

	conn, err := h.atlassian.ExchangeCode(r.Context(), user.ID, code)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":     "connected",
		"site_url":   conn.SiteURL,
		"connection": conn,
	})
}

func (h *Handlers) AtlassianListSpaces(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	spaces, err := h.atlassian.ListSpaces(r.Context(), user.ID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, spaces)
}

type SetConfluenceTargetRequest struct {
	SpaceKey        string `json:"space_key"`
	SpaceName       string `json:"space_name"`
	ParentPageID    string `json:"parent_page_id,omitempty"`
	ParentPageTitle string `json:"parent_page_title,omitempty"`
}

func (h *Handlers) SetProjectConfluenceTarget(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && (!isMember || role != models.ProjectMemberRoleOwner) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: only project owner can configure confluence target"})
		return
	}

	var req SetConfluenceTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if req.SpaceKey == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "space_key is required"})
		return
	}

	// Verify that user has access to space
	hasAccess, err := h.atlassian.CheckSpaceAccess(r.Context(), user.ID, req.SpaceKey)
	if err == nil && !hasAccess && !h.atlassian.IsMockMode() {
		respondJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("You do not have access to Confluence space '%s'", req.SpaceKey),
		})
		return
	}

	conn, _ := h.repo.GetAtlassianConnectionByUserID(user.ID)
	connID := "mock-connection-id"
	if conn != nil {
		connID = conn.ID
	}

	target := &models.ConfluenceTarget{
		ProjectID:       projectID,
		ConnectionID:    connID,
		SpaceKey:        req.SpaceKey,
		SpaceName:       req.SpaceName,
		ParentPageID:    req.ParentPageID,
		ParentPageTitle: req.ParentPageTitle,
		UpdatedBy:       &user.ID,
		IsDefault:       true,
	}

	if err := h.repo.UpsertConfluenceTarget(target); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	target, _ = h.repo.GetConfluenceTargetByProject(projectID)
	respondJSON(w, http.StatusOK, target)
}

func (h *Handlers) GetProjectConfluenceTarget(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, _, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: you are not a member of this project"})
		return
	}

	target, err := h.repo.GetConfluenceTargetByProject(projectID)
	if err != nil || target == nil {
		respondJSON(w, http.StatusOK, nil)
		return
	}
	respondJSON(w, http.StatusOK, target)
}

func (h *Handlers) DeleteProjectConfluenceTarget(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && (!isMember || role != models.ProjectMemberRoleOwner) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: only project owner can disconnect confluence target"})
		return
	}

	if err := h.repo.DeleteConfluenceTarget(projectID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (h *Handlers) ListProjectConfluenceSpaces(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, _, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: you are not a member of this project"})
		return
	}

	spaces, err := h.atlassian.ListSpaces(r.Context(), user.ID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, spaces)
}

func (h *Handlers) ListProjectConfluencePages(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	isMember, _, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: you are not a member of this project"})
		return
	}

	spaceKey := r.URL.Query().Get("space_key")
	if spaceKey == "" {
		target, _ := h.repo.GetConfluenceTargetByProject(projectID)
		if target != nil {
			spaceKey = target.SpaceKey
		}
	}
	if spaceKey == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "space_key is required"})
		return
	}

	pages, err := h.atlassian.ListSpacePages(r.Context(), user.ID, spaceKey)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, pages)
}

type ExportDocumentConfluenceRequest struct {
	OverrideSpaceKey     string `json:"override_space_key,omitempty"`
	OverrideSpaceName    string `json:"override_space_name,omitempty"`
	OverrideParentPageID string `json:"override_parent_page_id,omitempty"`
	SetAsProjectDefault  bool   `json:"set_as_project_default,omitempty"`
}

func (h *Handlers) ExportDocumentToConfluence(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	doc, err := h.repo.GetDocumentByID(docID)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	isMember, role, _ := h.repo.IsProjectMemberOrOwner(doc.ProjectID, user.ID)
	if user.Role != models.RoleAdmin && !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: you are not a member of this project"})
		return
	}

	var req ExportDocumentConfluenceRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	spaceKey := req.OverrideSpaceKey
	parentPageID := req.OverrideParentPageID

	if spaceKey == "" {
		target, err := h.repo.GetConfluenceTargetByProject(doc.ProjectID)
		if err != nil || target == nil {
			respondJSON(w, http.StatusBadRequest, map[string]any{
				"error":        "No Confluence target configured for this project. Please configure a target space.",
				"needs_target": true,
			})
			return
		}
		spaceKey = target.SpaceKey
		parentPageID = target.ParentPageID
	}

	// Verify that exporting user has access to space
	hasAccess, err := h.atlassian.CheckSpaceAccess(r.Context(), user.ID, spaceKey)
	if err == nil && !hasAccess {
		accessibleSpaces, _ := h.atlassian.ListSpaces(r.Context(), user.ID)
		respondJSON(w, http.StatusForbidden, map[string]any{
			"error":                   fmt.Sprintf("You do not have access to Confluence space '%s'.", spaceKey),
			"code":                    "SPACE_ACCESS_DENIED",
			"target_space_key":        spaceKey,
			"accessible_spaces":       accessibleSpaces,
			"can_set_project_default": user.Role == models.RoleAdmin || role == models.ProjectMemberRoleOwner,
		})
		return
	}

	pageID, err := h.atlassian.PublishDocument(r.Context(), user.ID, doc.ProjectID, docID, spaceKey, parentPageID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// If owner requested to update project default
	if req.SetAsProjectDefault && req.OverrideSpaceKey != "" && (user.Role == models.RoleAdmin || role == models.ProjectMemberRoleOwner) {
		_ = h.repo.UpsertConfluenceTarget(&models.ConfluenceTarget{
			ProjectID:    doc.ProjectID,
			SpaceKey:     req.OverrideSpaceKey,
			SpaceName:    req.OverrideSpaceName,
			ParentPageID: req.OverrideParentPageID,
			UpdatedBy:    &user.ID,
			IsDefault:    true,
		})
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"confluence_page_id": pageID,
		"status":             "published",
	})
}

// ----------------- NOTIFICATION PREFERENCES -----------------

func (h *Handlers) GetNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	projectIDParam := r.URL.Query().Get("project_id")
	var projectID *string
	if projectIDParam != "" {
		projectID = &projectIDParam
	}

	prefs, err := h.repo.ListNotificationPreferences(user.ID, projectID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	for i := range prefs {
		if prefs[i].TeamsWebhookURLEncrypted != "" {
			prefs[i].TeamsWebhookMasked = "https://outlook.office.com/webhook/••••••••••••"
		}
	}

	respondJSON(w, http.StatusOK, prefs)
}

type SetNotificationPrefRequest struct {
	ProjectID       *string                    `json:"project_id,omitempty"`
	Channel         models.NotificationChannel `json:"channel"`
	Enabled         bool                       `json:"enabled"`
	TeamsWebhookURL string                     `json:"teams_webhook_url,omitempty"`
}

func (h *Handlers) SaveNotificationPreference(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)

	var req SetNotificationPrefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	var encWebhook string
	if req.TeamsWebhookURL != "" {
		var err error
		encWebhook, err = crypto.Encrypt(req.TeamsWebhookURL, h.cfg.EncryptionKey)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt teams webhook url"})
			return
		}
	}

	pref := &models.NotificationPreference{
		UserID:                   user.ID,
		ProjectID:                req.ProjectID,
		Channel:                  req.Channel,
		Enabled:                  req.Enabled,
		TeamsWebhookURLEncrypted: encWebhook,
	}

	if err := h.repo.UpsertNotificationPreference(pref); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	pref.TeamsWebhookMasked = "https://outlook.office.com/webhook/••••••••••••"
	respondJSON(w, http.StatusOK, pref)
}

func (h *Handlers) GetProjectCompleteness(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	compSvc := service.NewCompletenessService(h.repo)
	scorecard, err := compSvc.EvaluateProject(r.Context(), projectID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, scorecard)
}

func (h *Handlers) GetGenerationStages(w http.ResponseWriter, r *http.Request) {
	generationID := chi.URLParam(r, "id")
	stages, err := h.repo.ListGenerationStages(generationID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, stages)
}

func (h *Handlers) GetProjectTokenUsage(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	user := GetCurrentUser(r)

	// Access control: must be member or admin
	isMember, _, _ := h.repo.IsProjectMemberOrOwner(projectID, user.ID)
	if user.Role != models.RoleAdmin && !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: you are not a member of this project"})
		return
	}

	summary, err := h.repo.GetProjectTokenSummary(projectID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, summary)
}
