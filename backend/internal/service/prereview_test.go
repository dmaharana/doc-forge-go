package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"docforge/internal/config"
	"docforge/internal/db"
	"docforge/internal/llm"
	"docforge/internal/models"
	"docforge/internal/repository"
	"docforge/internal/service"
)

func setupTestRepo(t *testing.T) (*db.DB, *repository.Repository) {
	t.Helper()
	cfg := &config.Config{
		DBPath:        ":memory:",
		AdminEmail:    "admin@docforge.test",
		AdminPassword: "TestPassword123!",
		AdminName:     "Test Admin",
	}
	database, err := db.Connect(cfg)
	if err != nil {
		t.Fatalf("db error: %v", err)
	}
	repo := repository.New(database)
	return database, repo
}

func TestCompletenessEvaluation(t *testing.T) {
	database, repo := setupTestRepo(t)
	defer database.Close()

	user := &models.User{Email: "comp@test.local", Name: "Comp", Role: models.RoleUser, PasswordHash: "h", IsActive: true}
	_ = repo.CreateUser(user)
	project := &models.Project{Name: "Payments Gateway", OwnerID: user.ID}
	_ = repo.CreateProject(project)

	compSvc := service.NewCompletenessService(repo)

	// Score without chunks
	scorecard, err := compSvc.EvaluateProject(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("failed to evaluate: %v", err)
	}
	if scorecard.CoverageScore != 0 {
		t.Errorf("expected 0 coverage on empty project, got %d", scorecard.CoverageScore)
	}
	if len(scorecard.MissingAreas) != 5 {
		t.Errorf("expected 5 missing areas, got %d", len(scorecard.MissingAreas))
	}

	src := &models.Source{ProjectID: project.ID, Name: "main.go", Type: models.SourceTypeUpload, Status: "ready"}
	_ = repo.CreateSource(src)

	// Add entrypoint chunk and config chunk
	chunks := []models.SourceChunk{
		{
			ProjectID:    project.ID,
			SourceID:     src.ID,
			ChunkIndex:   0,
			FilePath:     "cmd/main.go",
			SymbolName:   "main",
			ChunkType:    "code_symbol",
			SearchHeader: "[Source: cmd/main.go | Symbol: main | Type: function]",
			Content:      "func main() { log.Println(\"start\") }",
			CreatedAt:    time.Now().UTC(),
		},
		{
			ProjectID:    project.ID,
			SourceID:     src.ID,
			ChunkIndex:   1,
			FilePath:     ".env.example",
			ChunkType:    "code_symbol",
			SearchHeader: "[Source: .env.example | Type: config]",
			Content:      "PORT=8080\nDB=sqlite.db",
			CreatedAt:    time.Now().UTC(),
		},
	}
	if err := repo.SaveSourceChunks(chunks); err != nil {
		t.Fatalf("failed to save chunks: %v", err)
	}

	scorecard2, _ := compSvc.EvaluateProject(context.Background(), project.ID)
	if scorecard2.CoverageScore != 40 {
		t.Errorf("expected 40 points (entrypoint + config), got %d", scorecard2.CoverageScore)
	}
}

func TestPreReviewEngineGate(t *testing.T) {
	database, repo := setupTestRepo(t)
	defer database.Close()

	user := &models.User{Email: "prerev@test.local", Name: "PreReview Tester", Role: models.RoleUser, PasswordHash: "h", IsActive: true}
	_ = repo.CreateUser(user)
	project := &models.Project{Name: "Gateway", OwnerID: user.ID}
	_ = repo.CreateProject(project)

	src := &models.Source{ProjectID: project.ID, Name: "routes.go", Type: models.SourceTypeUpload, Status: "ready"}
	_ = repo.CreateSource(src)

	// Ingest a real endpoint chunk: GET /api/v1/health
	_ = repo.SaveSourceChunks([]models.SourceChunk{
		{
			ProjectID:    project.ID,
			SourceID:     src.ID,
			ChunkIndex:   0,
			FilePath:     "routes.go",
			SymbolName:   "GET /api/v1/health",
			ChunkType:    "code_symbol",
			SearchHeader: "[Source: routes.go | Symbol: GET /api/v1/health | Type: endpoint]",
			Content:      "r.Get(\"/api/v1/health\", HealthCheck)",
			CreatedAt:    time.Now().UTC(),
		},
	})

	doc := &models.Document{
		ProjectID:    project.ID,
		Title:        "Product Spec",
		TemplateType: "product_spec",
		ContentMD: `# Product Spec: Gateway

This page describes the task for the team.

## API Endpoints
- [Health Check](#health-check): GET /api/v1/health
- [Fictitious Endpoint](#broken): POST /api/v1/nonexistent/secret/charge

## Technical Gaps
We need to check [MISSING] billing configuration.
`,
		Status: models.DocStatusInReview,
	}
	_ = repo.CreateDocument(doc, user.ID)

	provider := llm.NewMockProvider("test-mock")
	engine := service.NewPreReviewEngine(repo)

	report, updatedMD, err := engine.RunPreReview(context.Background(), doc, provider, project.ID, nil)
	if err != nil {
		t.Fatalf("RunPreReview failed: %v", err)
	}

	// 1. Check self-correction replaced " page " and " task "
	if strings.Contains(updatedMD, " page ") || strings.Contains(updatedMD, " task ") {
		t.Errorf("expected avoided glossary words to be replaced by self-correction, got: %s", updatedMD)
	}

	// 2. Check blocking feedback was created for nonexistent endpoint POST /api/v1/nonexistent/secret/charge
	// and for [MISSING] tag
	feedbacks, err := repo.ListDocumentFeedback(doc.ID)
	if err != nil {
		t.Fatalf("failed to list feedback: %v", err)
	}

	hasBlockingFeedback := false
	for _, fb := range feedbacks {
		if fb.IsBlocking && strings.Contains(fb.Content, "nonexistent") {
			hasBlockingFeedback = true
			break
		}
	}

	if !hasBlockingFeedback {
		t.Errorf("expected blocking feedback for unverified endpoint, but none found. Feedback list: %+v", feedbacks)
	}

	if report.MissingCount != 1 {
		t.Errorf("expected 1 missing tag, got %d", report.MissingCount)
	}
}
