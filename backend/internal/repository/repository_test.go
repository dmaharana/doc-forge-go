package repository_test

import (
	"testing"
	"time"

	"docforge/internal/config"
	"docforge/internal/db"
	"docforge/internal/models"
	"docforge/internal/repository"
)

func setupTestDB(t *testing.T) (*db.DB, *repository.Repository) {
	t.Helper()
	cfg := &config.Config{
		DBPath:        ":memory:",
		AdminEmail:    "admin@docforge.test",
		AdminPassword: "TestPassword123!",
		AdminName:     "Test Admin",
	}
	database, err := db.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	repo := repository.New(database)
	return database, repo
}

func TestSourceChunksAndFTS5Search(t *testing.T) {
	database, repo := setupTestDB(t)
	defer database.Close()

	// Create user & project
	user := &models.User{
		Email:        "user@test.local",
		Name:         "Chunk Tester",
		Role:         models.RoleUser,
		PasswordHash: "hash",
		IsActive:     true,
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	project := &models.Project{
		Name:        "Searchable Project",
		Description: "Testing FTS5 search",
		OwnerID:     user.ID,
	}
	if err := repo.CreateProject(project); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	source := &models.Source{
		ProjectID: project.ID,
		Name:      "auth_service.go",
		Type:      models.SourceTypeUpload,
		Status:    "ready",
	}
	if err := repo.CreateSource(source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Insert chunks
	chunks := []models.SourceChunk{
		{
			SourceID:     source.ID,
			ProjectID:    project.ID,
			ChunkIndex:   0,
			FilePath:     "internal/auth/handler.go",
			SymbolName:   "HandleLogin",
			ChunkType:    "code_symbol",
			LineStart:    10,
			LineEnd:      45,
			SearchHeader: "[Source: internal/auth/handler.go | Symbol: HandleLogin | Type: HTTP_HANDLER]",
			Content:      "func HandleLogin(w http.ResponseWriter, r *http.Request) {\n  // Validates credentials and generates JWT session tokens\n}",
			CreatedAt:    time.Now().UTC(),
		},
		{
			SourceID:     source.ID,
			ProjectID:    project.ID,
			ChunkIndex:   1,
			FilePath:     "internal/config/config.go",
			SymbolName:   "Config",
			ChunkType:    "code_symbol",
			LineStart:    1,
			LineEnd:      25,
			SearchHeader: "[Source: internal/config/config.go | Symbol: Config | Type: STRUCT]",
			Content:      "type Config struct {\n  JWTSecret string\n  DatabaseURL string\n}",
			CreatedAt:    time.Now().UTC(),
		},
	}

	if err := repo.SaveSourceChunks(chunks); err != nil {
		t.Fatalf("failed to save source chunks: %v", err)
	}

	// Test list by source
	bySrc, err := repo.ListSourceChunksBySource(source.ID)
	if err != nil || len(bySrc) != 2 {
		t.Fatalf("expected 2 chunks by source, got %d (err: %v)", len(bySrc), err)
	}

	// Test FTS5 search
	results, err := repo.SearchSourceChunks(project.ID, "JWT tokens", nil, 10)
	if err != nil {
		t.Fatalf("FTS5 search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least 1 search hit for 'JWT tokens', got 0")
	}
	if results[0].SymbolName != "HandleLogin" {
		t.Fatalf("expected top hit to be HandleLogin, got %s", results[0].SymbolName)
	}

	// Test ListProjectCodeSymbols
	symbols, err := repo.ListProjectCodeSymbols(project.ID)
	if err != nil {
		t.Fatalf("failed to list project code symbols: %v", err)
	}
	if len(symbols) != 2 {
		t.Fatalf("expected 2 distinct code symbols, got %d", len(symbols))
	}
}

func TestGenerationStagesPersistence(t *testing.T) {
	database, repo := setupTestDB(t)
	defer database.Close()

	user := &models.User{
		Email:        "stage@test.local",
		Name:         "Stage Tester",
		Role:         models.RoleUser,
		PasswordHash: "hash",
		IsActive:     true,
	}
	_ = repo.CreateUser(user)

	project := &models.Project{
		Name:        "Stage Project",
		Description: "Testing generation stages",
		OwnerID:     user.ID,
	}
	_ = repo.CreateProject(project)

	gen := &models.Generation{
		ProjectID:   project.ID,
		RequestedBy: user.ID,
		TemplateSet: "product_spec",
	}
	if err := repo.CreateGeneration(gen, nil); err != nil {
		t.Fatalf("failed to create generation: %v", err)
	}

	stage := &models.GenerationStage{
		GenerationID: gen.ID,
		StageName:    "system_mapping",
		Status:       models.StageProcessing,
		InputContext: "configs and entrypoints",
	}
	if err := repo.CreateGenerationStage(stage); err != nil {
		t.Fatalf("failed to create generation stage: %v", err)
	}

	// Complete stage
	stage.Status = models.StageCompleted
	stage.OutputPayload = "System Architecture Map: ..."
	stage.TokensUsed = 120
	stage.DurationMs = 450
	if err := repo.UpdateGenerationStage(stage); err != nil {
		t.Fatalf("failed to update generation stage: %v", err)
	}

	// Update system map on generation
	if err := repo.UpdateGenerationSystemMap(gen.ID, stage.OutputPayload); err != nil {
		t.Fatalf("failed to update generation system map: %v", err)
	}

	fetchedGen, err := repo.GetGenerationByID(gen.ID)
	if err != nil {
		t.Fatalf("failed to get generation: %v", err)
	}
	if fetchedGen.SystemMapMD == nil || *fetchedGen.SystemMapMD != stage.OutputPayload {
		t.Fatalf("system map on generation mismatch: got %v", fetchedGen.SystemMapMD)
	}
	if len(fetchedGen.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(fetchedGen.Stages))
	}
	if fetchedGen.Stages[0].Status != models.StageCompleted {
		t.Fatalf("expected stage status 'completed', got '%s'", fetchedGen.Stages[0].Status)
	}
}

func TestProjectLLMAssociation(t *testing.T) {
	database, repo := setupTestDB(t)
	defer database.Close()

	user := &models.User{
		Email:        "llm_user@test.local",
		Name:         "LLM Tester",
		Role:         models.RoleAdmin,
		PasswordHash: "hash",
		IsActive:     true,
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// 1. Create an LLM config
	llmCfg := &models.LLMConfig{
		Provider:        "anthropic",
		APIKeyEncrypted: "enc_key",
		DefaultModel:    "claude-3-5-sonnet-20241022",
		MaxTokens:       4096,
		Temperature:     0.2,
		IsActive:        false,
	}
	if err := repo.SaveLLMConfig(llmCfg); err != nil {
		t.Fatalf("failed to save LLM config: %v", err)
	}

	// 2. Create project associated with this LLM config
	project := &models.Project{
		Name:        "Anthropic Project",
		Description: "Project with associated Claude model",
		OwnerID:     user.ID,
		LLMConfigID: &llmCfg.ID,
	}
	if err := repo.CreateProject(project); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// 3. Fetch project and verify association
	fetched, err := repo.GetProjectByID(project.ID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}
	if fetched.LLMConfigID == nil || *fetched.LLMConfigID != llmCfg.ID {
		t.Fatalf("expected llm_config_id %s, got %v", llmCfg.ID, fetched.LLMConfigID)
	}
	if fetched.LLMProvider != "anthropic" || fetched.LLMModel != "claude-3-5-sonnet-20241022" {
		t.Fatalf("expected provider 'anthropic' and model 'claude-3-5-sonnet-20241022', got '%s', '%s'", fetched.LLMProvider, fetched.LLMModel)
	}

	// 4. Update project to remove association (reset to system default)
	if err := repo.UpdateProject(project.ID, project.Name, project.Description, project.Status, nil); err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	cleared, err := repo.GetProjectByID(project.ID)
	if err != nil {
		t.Fatalf("failed to get cleared project: %v", err)
	}
	if cleared.LLMConfigID != nil {
		t.Fatalf("expected llm_config_id to be nil, got %v", cleared.LLMConfigID)
	}
	if cleared.LLMProvider != "" || cleared.LLMModel != "" {
		t.Fatalf("expected empty provider and model, got '%s', '%s'", cleared.LLMProvider, cleared.LLMModel)
	}
}

func TestProjectTokenUsageLedger(t *testing.T) {
	database, repo := setupTestDB(t)
	defer database.Close()

	user := &models.User{
		Email:        "token_test@test.local",
		Name:         "Token Tester",
		Role:         models.RoleUser,
		PasswordHash: "hash",
		IsActive:     true,
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	project := &models.Project{
		Name:        "Token Usage Project",
		Description: "Testing token ledger and chargeback aggregation",
		OwnerID:     user.ID,
	}
	if err := repo.CreateProject(project); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// 1. Initial token summary should be 0
	summary, err := repo.GetProjectTokenSummary(project.ID)
	if err != nil {
		t.Fatalf("failed to get token summary: %v", err)
	}
	if summary.TotalTokens != 0 || summary.EventsCount != 0 {
		t.Fatalf("expected 0 initial tokens, got total=%d count=%d", summary.TotalTokens, summary.EventsCount)
	}

	// 2. Create a generation
	gen := &models.Generation{
		ProjectID:   project.ID,
		RequestedBy: user.ID,
		Status:      models.GenerationProcessing,
		TemplateSet: "product_spec",
	}
	if err := repo.CreateGeneration(gen, nil); err != nil {
		t.Fatalf("failed to create generation: %v", err)
	}

	// 3. Record token events
	// Event 1: System mapping with gpt-4o
	err = repo.RecordTokenUsage(&models.TokenUsage{
		ProjectID:    project.ID,
		GenerationID: &gen.ID,
		Operation:    "system_mapping",
		ModelUsed:    "gpt-4o",
		InputTokens:  200,
		OutputTokens: 150,
		TotalTokens:  350,
	})
	if err != nil {
		t.Fatalf("failed to record token event 1: %v", err)
	}

	// Event 2: Section population with gpt-4o
	err = repo.RecordTokenUsage(&models.TokenUsage{
		ProjectID:    project.ID,
		GenerationID: &gen.ID,
		Operation:    "section_population",
		ModelUsed:    "gpt-4o",
		InputTokens:  500,
		OutputTokens: 300,
		TotalTokens:  800,
	})
	if err != nil {
		t.Fatalf("failed to record token event 2: %v", err)
	}

	// Event 3: Feedback revision with claude-3-5-sonnet
	err = repo.RecordTokenUsage(&models.TokenUsage{
		ProjectID:    project.ID,
		Operation:    "feedback_revision",
		ModelUsed:    "claude-3-5-sonnet",
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	})
	if err != nil {
		t.Fatalf("failed to record token event 3: %v", err)
	}

	// 4. Verify aggregated summary
	summary, err = repo.GetProjectTokenSummary(project.ID)
	if err != nil {
		t.Fatalf("failed to get token summary: %v", err)
	}
	if summary.TotalTokens != 1300 {
		t.Fatalf("expected 1300 total tokens, got %d", summary.TotalTokens)
	}
	if summary.TotalInputTokens != 800 {
		t.Fatalf("expected 800 input tokens, got %d", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 500 {
		t.Fatalf("expected 500 output tokens, got %d", summary.TotalOutputTokens)
	}
	if summary.EventsCount != 3 {
		t.Fatalf("expected 3 events, got %d", summary.EventsCount)
	}
	if summary.ByModel["gpt-4o"] != 1150 {
		t.Fatalf("expected 1150 gpt-4o tokens, got %d", summary.ByModel["gpt-4o"])
	}
	if summary.ByModel["claude-3-5-sonnet"] != 150 {
		t.Fatalf("expected 150 claude tokens, got %d", summary.ByModel["claude-3-5-sonnet"])
	}
	if summary.ByOperation["system_mapping"] != 350 {
		t.Fatalf("expected 350 system_mapping tokens, got %d", summary.ByOperation["system_mapping"])
	}
	if summary.ByOperation["section_population"] != 800 {
		t.Fatalf("expected 800 section_population tokens, got %d", summary.ByOperation["section_population"])
	}
	if summary.ByOperation["feedback_revision"] != 150 {
		t.Fatalf("expected 150 feedback_revision tokens, got %d", summary.ByOperation["feedback_revision"])
	}
	if len(summary.RecentUsage) != 3 {
		t.Fatalf("expected 3 recent usage items, got %d", len(summary.RecentUsage))
	}

	// 5. Verify Project TotalTokens in GetProjectByID and ListProjects
	proj, err := repo.GetProjectByID(project.ID)
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}
	if proj.TotalTokens != 1300 {
		t.Fatalf("expected 1300 project TotalTokens, got %d", proj.TotalTokens)
	}

	projs, err := repo.ListProjects(user.ID, false)
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}
	if len(projs) != 1 || projs[0].TotalTokens != 1300 {
		t.Fatalf("expected 1300 total tokens in listed project, got %d", projs[0].TotalTokens)
	}

	// 6. Verify generation total tokens update
	if err := repo.UpdateGenerationTotalTokens(gen.ID); err != nil {
		t.Fatalf("failed to update generation total tokens: %v", err)
	}
	fetchedGen, err := repo.GetGenerationByID(gen.ID)
	if err != nil {
		t.Fatalf("failed to get generation: %v", err)
	}
	if fetchedGen.TotalTokens != 1150 {
		t.Fatalf("expected 1150 generation total tokens, got %d", fetchedGen.TotalTokens)
	}
}

