package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"docforge/internal/config"
	"docforge/internal/db"
	"docforge/internal/ingestion"
	"docforge/internal/models"
	"docforge/internal/repository"
	"docforge/internal/service"
)

func setupTestServer(t *testing.T) (http.Handler, *config.Config, *repository.Repository, func()) {
	tmpDir, err := os.MkdirTemp("", "docforge-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		Port:          8080,
		DBPath:        tmpDir + "/test.db",
		JWTSecret:     "test-jwt-secret-key-32-bytes-long!",
		EncryptionKey: []byte("12345678901234567890123456789012"),
		UploadDir:     tmpDir + "/uploads",
		DevMode:       true,
		MockAtlassian: true,
		AdminEmail:    "admin@docforge.test",
		AdminPassword: "AdminPassword123!",
		AdminName:     "Test Admin",
	}

	database, err := db.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}

	repo := repository.New(database)
	storage, err := ingestion.NewLocalStorage(cfg.UploadDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	notifier := service.NewNotificationService(cfg)
	atlassian := service.NewAtlassianService(cfg, repo)
	genSvc := service.NewGenerationService(cfg, repo, notifier)

	ctx, cancel := context.WithCancel(context.Background())
	genSvc.StartWorker(ctx)

	authHandler := NewAuthHandler(cfg, repo, atlassian)
	handlers := NewHandlers(cfg, repo, storage, genSvc, atlassian)
	router := NewRouter(cfg, repo, handlers, authHandler)

	cleanup := func() {
		cancel()
		genSvc.StopWorker()
		database.Close()
		os.RemoveAll(tmpDir)
	}

	return router, cfg, repo, cleanup
}

func TestEndToEndHappyPath(t *testing.T) {
	router, cfg, repo, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Login with seeded admin
	loginBody, _ := json.Marshal(map[string]string{
		"email":    cfg.AdminEmail,
		"password": cfg.AdminPassword,
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: code %d, body: %s", w.Code, w.Body.String())
	}

	var loginResp LoginResponse
	_ = json.Unmarshal(w.Body.Bytes(), &loginResp)
	token := loginResp.Token
	if token == "" {
		t.Fatalf("empty token returned")
	}

	authHeader := "Bearer " + token

	// 2. Admin configures Mock LLM Provider
	llmCfgBody, _ := json.Marshal(map[string]any{
		"provider":      "mock",
		"api_key":       "mock-key-123",
		"default_model": "docforge-gpt-mock",
		"max_tokens":    4096,
		"temperature":   0.2,
		"is_active":     true,
	})
	req = httptest.NewRequest("POST", "/api/v1/admin/llm", bytes.NewReader(llmCfgBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to save LLM config: %d, body: %s", w.Code, w.Body.String())
	}

	// 3. User creates a Project
	projBody, _ := json.Marshal(map[string]string{
		"name":        "Payments Gateway Service",
		"description": "High-throughput payment orchestration engine supporting credit card tokenization and ACH transfers.",
	})
	req = httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(projBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create project: %d, body: %s", w.Code, w.Body.String())
	}

	var project models.Project
	_ = json.Unmarshal(w.Body.Bytes(), &project)

	// 4. Ingest a Source (remote/text source)
	srcBody, _ := json.Marshal(map[string]string{
		"name":        "payments_api.json",
		"remote_url":  "https://example.com/api/spec.json",
		"remote_type": "http",
	})
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/projects/%s/sources/remote", project.ID), bytes.NewReader(srcBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to add source: %d, body: %s", w.Code, w.Body.String())
	}

	var source models.Source
	_ = json.Unmarshal(w.Body.Bytes(), &source)

	// 5. Submit Generation Request for product_spec and adr
	genReqBody, _ := json.Marshal(map[string]any{
		"template_set": "product_spec,adr",
		"source_ids":   []string{source.ID},
	})
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/projects/%s/generations", project.ID), bytes.NewReader(genReqBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("failed to submit generation: %d, body: %s", w.Code, w.Body.String())
	}

	var gen models.Generation
	_ = json.Unmarshal(w.Body.Bytes(), &gen)

	// Wait for worker to complete the generation job
	var completedGen *models.Generation
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		completedGen, _ = repo.GetGenerationByID(gen.ID)
		if completedGen != nil && (completedGen.Status == models.GenerationCompleted || completedGen.Status == models.GenerationFailed) {
			break
		}
	}

	if completedGen == nil || completedGen.Status != models.GenerationCompleted {
		t.Fatalf("generation did not complete in time, status: %v", completedGen)
	}

	// 6. List generated documents
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s/documents", project.ID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to list documents: %d", w.Code)
	}

	var docs []models.Document
	_ = json.Unmarshal(w.Body.Bytes(), &docs)
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents synthesized, got %d", len(docs))
	}

	doc := docs[0]

	// 7. Add Feedback to the document - including a BLOCKING feedback item
	fbBody, _ := json.Marshal(map[string]any{
		"section_anchor": "## 4. API & Integration Endpoints",
		"category":       "correction",
		"content":        "The settlement endpoint route should be /v2/settlements instead of /v1.",
		"is_blocking":    true,
	})
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/documents/%s/feedback", doc.ID), bytes.NewReader(fbBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to add feedback: %d, body: %s", w.Code, w.Body.String())
	}

	// 8. VERIFY HARD GATE: Attempt to approve document with open blocking feedback -> must FAIL with 409 Conflict!
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/documents/%s/approve", doc.ID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected HTTP 409 Conflict due to open blocking feedback, got %d: %s", w.Code, w.Body.String())
	}

	// 9. Trigger "Update document with feedback" (AI revision loop)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/documents/%s/update-with-feedback", doc.ID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to update with feedback: %d, body: %s", w.Code, w.Body.String())
	}

	// Verify feedback was marked addressed automatically
	updatedDoc, err := repo.GetDocumentByID(doc.ID)
	if err != nil {
		t.Fatalf("failed to get updated document: %v", err)
	}

	blockingCount, _ := repo.CountOpenBlockingFeedback(doc.ID)
	if blockingCount != 0 {
		t.Fatalf("expected 0 open blocking items after update with feedback, got %d", blockingCount)
	}

	if len(updatedDoc.Versions) != 2 {
		t.Fatalf("expected 2 versions in history, got %d", len(updatedDoc.Versions))
	}

	// 10. Approve Document now that blocking feedback is resolved
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/documents/%s/approve", doc.ID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("approval failed after blocking items resolved: %d, body: %s", w.Code, w.Body.String())
	}

	// 11. Download Document as Markdown
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/documents/%s/download", doc.ID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("download failed: %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(doc.Title)) && len(w.Body.Bytes()) < 100 {
		t.Fatalf("expected document content in download response, got: %s", w.Body.String())
	}

	// 12. Connect Mock Atlassian for Admin user
	req = httptest.NewRequest("GET", "/api/v1/atlassian/callback?code=mock_atlassian_code", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("atlassian connect failed: %d", w.Code)
	}

	// 13. Project Confluence Spaces & Pages Discovery
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s/confluence/spaces", project.ID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list confluence spaces failed: %d", w.Code)
	}

	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s/confluence/pages?space_key=ENG", project.ID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list confluence pages failed: %d", w.Code)
	}

	// 14. Set Project Confluence Target with Space and Parent Page
	targetBody, _ := json.Marshal(map[string]string{
		"space_key":         "ENG",
		"space_name":        "Engineering & Architecture",
		"parent_page_id":    "mock-page-1",
		"parent_page_title": "Architecture & Standards",
	})
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/projects/%s/confluence-target", project.ID), bytes.NewReader(targetBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set confluence target failed: %d", w.Code)
	}

	// 15. Add Team Member to Project by Email
	memberBody, _ := json.Marshal(map[string]string{
		"email": "reviewer@example.com",
		"role":  "member",
	})
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/projects/%s/members", project.ID), bytes.NewReader(memberBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add project member failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 16. Log in as reviewer via Atlassian SSO
	req = httptest.NewRequest("GET", "/api/v1/auth/atlassian/callback?code="+url.QueryEscape("mock_atlassian_code:reviewer@example.com:Reviewer Bob"), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("atlassian sso login failed: %d, body: %s", w.Code, w.Body.String())
	}
	var reviewerLogin LoginResponse
	_ = json.Unmarshal(w.Body.Bytes(), &reviewerLogin)
	reviewerAuthHeader := "Bearer " + reviewerLogin.Token

	// 17. Verify reviewer can view the project and export to Confluence
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s", project.ID), nil)
	req.Header.Set("Authorization", reviewerAuthHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reviewer access to project failed: %d", w.Code)
	}

	// 18. Export Document to Confluence using reviewer's session
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/documents/%s/export-confluence", doc.ID), nil)
	req.Header.Set("Authorization", reviewerAuthHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("confluence export by reviewer failed: %d, body: %s", w.Code, w.Body.String())
	}

	// 19. Space Access Control Check: Attempt export to an unauthorized space
	unauthExportBody, _ := json.Marshal(map[string]string{
		"override_space_key": "RESTRICTED_CONFIDENTIAL",
	})
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/documents/%s/export-confluence", doc.ID), bytes.NewReader(unauthExportBody))
	req.Header.Set("Authorization", reviewerAuthHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 Forbidden for inaccessible space, got %d", w.Code)
	}

	// 20. Privacy Isolation Check: Uninvited user must NOT have access to project
	req = httptest.NewRequest("GET", "/api/v1/auth/atlassian/callback?code="+url.QueryEscape("mock_atlassian_code:stranger@example.com:Stranger User"), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var strangerLogin LoginResponse
	_ = json.Unmarshal(w.Body.Bytes(), &strangerLogin)
	strangerAuthHeader := "Bearer " + strangerLogin.Token

	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s", project.ID), nil)
	req.Header.Set("Authorization", strangerAuthHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 Forbidden for non-member, got %d", w.Code)
	}
}
