package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"docforge/internal/config"
	"docforge/internal/crypto"
	"docforge/internal/llm"
	"docforge/internal/models"
	"docforge/internal/repository"
	"docforge/internal/templates"
)

type GenerationService struct {
	cfg          *config.Config
	repo         *repository.Repository
	notifier     *NotificationService
	completeness *CompletenessService
	prereview    *PreReviewEngine
	workerCancel context.CancelFunc
	wg           sync.WaitGroup
}

func NewGenerationService(cfg *config.Config, repo *repository.Repository, notifier *NotificationService) *GenerationService {
	return &GenerationService{
		cfg:          cfg,
		repo:         repo,
		notifier:     notifier,
		completeness: NewCompletenessService(repo),
		prereview:    NewPreReviewEngine(repo),
	}
}

func (s *GenerationService) StartWorker(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	s.workerCancel = cancel

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		log.Println("[WORKER] Generation background worker started")

		for {
			select {
			case <-ctx.Done():
				log.Println("[WORKER] Generation background worker stopping")
				return
			case <-ticker.C:
				s.processNextQueuedJob(ctx)
			}
		}
	}()
}

func (s *GenerationService) StopWorker() {
	if s.workerCancel != nil {
		s.workerCancel()
	}
	s.wg.Wait()
}

func (s *GenerationService) processNextQueuedJob(ctx context.Context) {
	gen, err := s.repo.ClaimNextQueuedGeneration()
	if err != nil || gen == nil {
		return // No pending jobs or error
	}

	log.Printf("[WORKER] Processing generation job %s for project %s", gen.ID, gen.ProjectID)

	project, err := s.repo.GetProjectByID(gen.ProjectID)
	if err != nil {
		_ = s.repo.UpdateGenerationStatus(gen.ID, models.GenerationFailed, "", fmt.Sprintf("project error: %v", err))
		return
	}

	user, err := s.repo.GetUserByID(gen.RequestedBy)
	if err != nil {
		_ = s.repo.UpdateGenerationStatus(gen.ID, models.GenerationFailed, "", fmt.Sprintf("user error: %v", err))
		return
	}

	// Fetch sources
	var sources []models.Source
	for _, sID := range gen.SourceIDs {
		src, err := s.repo.GetSourceByID(sID)
		if err == nil && src != nil {
			sources = append(sources, *src)
		}
	}

	// If no specific sources linked, load all project sources
	if len(sources) == 0 {
		allSrcs, _ := s.repo.ListSourcesByProject(gen.ProjectID)
		sources = allSrcs
	}

	// Load LLM configuration
	var llmCfg *models.LLMConfig
	targetLLMID := gen.LLMConfigID
	if (targetLLMID == nil || *targetLLMID == "") && project.LLMConfigID != nil && *project.LLMConfigID != "" {
		targetLLMID = project.LLMConfigID
	}
	if targetLLMID != nil && *targetLLMID != "" {
		cfgList, _ := s.repo.ListLLMConfigs()
		for _, c := range cfgList {
			if c.ID == *targetLLMID {
				llmCfg = &c
				break
			}
		}
	}
	if llmCfg == nil {
		llmCfg, _ = s.repo.GetActiveLLMConfig()
	}

	var decryptedKey string
	if llmCfg != nil && llmCfg.APIKeyEncrypted != "" {
		decryptedKey, _ = crypto.Decrypt(llmCfg.APIKeyEncrypted, s.cfg.EncryptionKey)
	}

	provider := llm.NewProvider(llmCfg, decryptedKey)

	// ==================== STAGE 1: SYSTEM MAPPING ====================
	sysStage := &models.GenerationStage{
		GenerationID: gen.ID,
		StageName:    "system_mapping",
		Status:       models.StageProcessing,
		InputContext: fmt.Sprintf("Project: %s, Sources: %d", project.Name, len(sources)),
	}
	_ = s.repo.CreateGenerationStage(sysStage)
	sysStartTime := time.Now()

	keyChunks, _ := s.repo.SearchSourceChunks(gen.ProjectID, "main config server route endpoint spec model", nil, 10)
	if len(keyChunks) == 0 {
		keyChunks, _ = s.repo.ListSourceChunksByProject(gen.ProjectID)
	}

	sysReq := llm.SystemMapRequest{
		ProjectName: project.Name,
		Description: project.Description,
		Chunks:      keyChunks,
	}
	if llmCfg != nil {
		sysReq.Model = llmCfg.DefaultModel
		sysReq.Temperature = llmCfg.Temperature
		sysReq.MaxTokens = llmCfg.MaxTokens
	}

	sysResp, err := provider.GenerateSystemMap(ctx, sysReq)
	systemMapMD := ""
	if err == nil && sysResp != nil {
		systemMapMD = sysResp.Markdown
		sysStage.Status = models.StageCompleted
		sysStage.OutputPayload = systemMapMD
		sysStage.TokensUsed = sysResp.TokensUsed
		_ = s.repo.RecordTokenUsage(&models.TokenUsage{
			ProjectID:    project.ID,
			GenerationID: &gen.ID,
			StageID:      &sysStage.ID,
			Operation:    "system_mapping",
			ModelUsed:    sysResp.ModelUsed,
			InputTokens:  sysResp.InputTokens,
			OutputTokens: sysResp.OutputTokens,
			TotalTokens:  sysResp.TokensUsed,
		})
	} else {
		sysStage.Status = models.StageCompleted
	}
	sysStage.DurationMs = time.Since(sysStartTime).Milliseconds()
	_ = s.repo.UpdateGenerationStage(sysStage)
	if systemMapMD != "" {
		_ = s.repo.UpdateGenerationSystemMap(gen.ID, systemMapMD)
	}

	// ==================== STAGE 2: COMPLETENESS VALIDATION ====================
	compStage := &models.GenerationStage{
		GenerationID: gen.ID,
		StageName:    "completeness_check",
		Status:       models.StageProcessing,
	}
	_ = s.repo.CreateGenerationStage(compStage)
	compStartTime := time.Now()

	scorecard, err := s.completeness.EvaluateProject(ctx, gen.ProjectID)
	if err == nil && scorecard != nil {
		compJSON := s.completeness.ToJSON(scorecard)
		compStage.Status = models.StageCompleted
		compStage.OutputPayload = compJSON
		_ = s.repo.UpdateGenerationScorecard(gen.ID, compJSON)
	} else {
		compStage.Status = models.StageCompleted
	}
	compStage.DurationMs = time.Since(compStartTime).Milliseconds()
	_ = s.repo.UpdateGenerationStage(compStage)

	// Parse template types
	var templateTypes []string
	if strings.Contains(gen.TemplateSet, ",") {
		for _, part := range strings.Split(gen.TemplateSet, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				templateTypes = append(templateTypes, p)
			}
		}
	} else if gen.TemplateSet != "" {
		templateTypes = append(templateTypes, strings.TrimSpace(gen.TemplateSet))
	} else {
		templateTypes = templates.ListSupportedTemplates()
	}

	docsCreated := 0
	var lastModelUsed string

	for _, tmplType := range templateTypes {
		// ==================== STAGE 3: SECTION-BY-SECTION POPULATION ====================
		secStage := &models.GenerationStage{
			GenerationID: gen.ID,
			StageName:    fmt.Sprintf("section_population:%s", tmplType),
			Status:       models.StageProcessing,
			InputContext: fmt.Sprintf("Template: %s", tmplType),
		}
		_ = s.repo.CreateGenerationStage(secStage)
		secStartTime := time.Now()

		contentMD, modelUsed, tokensUsed := s.synthesizeDocument(ctx, gen, project, secStage.ID, tmplType, systemMapMD, provider, llmCfg, sources)
		lastModelUsed = modelUsed

		secStage.Status = models.StageCompleted
		secStage.TokensUsed = tokensUsed
		secStage.DurationMs = time.Since(secStartTime).Milliseconds()
		_ = s.repo.UpdateGenerationStage(secStage)

		doc := &models.Document{
			GenerationID: &gen.ID,
			ProjectID:    project.ID,
			Title:        templates.GetTemplateTitle(tmplType, project.Name),
			TemplateType: tmplType,
			ContentMD:    contentMD,
			Status:       models.DocStatusInReview,
		}

		if err := s.repo.CreateDocument(doc, gen.RequestedBy); err != nil {
			log.Printf("[WORKER] Error saving document %s: %v", tmplType, err)
			continue
		}

		// ==================== STAGE 4: AUTOMATED PRE-REVIEW CHECKS ====================
		preStage := &models.GenerationStage{
			GenerationID: gen.ID,
			StageName:    fmt.Sprintf("pre_review:%s", tmplType),
			Status:       models.StageProcessing,
			InputContext: fmt.Sprintf("Document: %s", doc.Title),
		}
		_ = s.repo.CreateGenerationStage(preStage)
		preStartTime := time.Now()

		report, finalMD, preErr := s.prereview.RunPreReview(ctx, doc, provider, gen.ProjectID, llmCfg)
		if preErr == nil && report != nil {
			preStage.Status = models.StageCompleted
			preStage.TokensUsed = report.TokensUsed
			if report.TokensUsed > 0 {
				_ = s.repo.RecordTokenUsage(&models.TokenUsage{
					ProjectID:    project.ID,
					GenerationID: &gen.ID,
					DocumentID:   &doc.ID,
					StageID:      &preStage.ID,
					Operation:    "pre_review_correction",
					ModelUsed:    report.ModelUsed,
					InputTokens:  report.InputTokens,
					OutputTokens: report.OutputTokens,
					TotalTokens:  report.TokensUsed,
				})
			}
			if report.SelfCorrected && finalMD != doc.ContentMD {
				doc.ContentMD = finalMD
				_, _ = s.repo.UpdateDocumentContent(doc.ID, finalMD, doc.Title, gen.RequestedBy, "Applied automated pre-review corrections.")
			}
		} else {
			preStage.Status = models.StageCompleted
		}
		preStage.DurationMs = time.Since(preStartTime).Milliseconds()
		_ = s.repo.UpdateGenerationStage(preStage)

		docsCreated++
	}

	_ = s.repo.UpdateGenerationTotalTokens(gen.ID)

	if docsCreated == 0 {
		_ = s.repo.UpdateGenerationStatus(gen.ID, models.GenerationFailed, lastModelUsed, "Failed to synthesize any documents")
		return
	}

	_ = s.repo.UpdateGenerationStatus(gen.ID, models.GenerationCompleted, lastModelUsed, "")
	gen.Status = models.GenerationCompleted
	gen.ModelUsed = lastModelUsed

	// Dispatch notification
	prefs, _ := s.repo.ListNotificationPreferences(gen.RequestedBy, &gen.ProjectID)
	s.notifier.SendGenerationNotification(ctx, user, project, gen, docsCreated, prefs)
	_ = s.repo.MarkGenerationNotificationSent(gen.ID)

	log.Printf("[WORKER] Generation %s completed with %d documents synthesized", gen.ID, docsCreated)
}

func (s *GenerationService) synthesizeDocument(ctx context.Context, gen *models.Generation, project *models.Project, stageID string, tmplType, systemMapMD string, provider llm.Provider, llmCfg *models.LLMConfig, sources []models.Source) (string, string, int) {
	if tmplType == "product_spec" || tmplType == "runbook" {
		return s.synthesizeSectionBySection(ctx, gen, project, stageID, tmplType, systemMapMD, provider, llmCfg)
	}

	// For ADR or release notes: retrieve top FTS5 chunks
	chunks, _ := s.repo.SearchSourceChunks(project.ID, tmplType+" architecture decision release changes", nil, 8)
	if len(chunks) == 0 {
		chunks, _ = s.repo.ListSourceChunksByProject(project.ID)
	}

	tmplData := templates.TemplateData{
		Title:       templates.GetTemplateTitle(tmplType, project.Name),
		ProjectName: project.Name,
		Date:        time.Now().Format("2006-01-02"),
		Summary:     project.Description,
		Sources:     sources,
	}
	baseTmpl, _ := templates.RenderTemplate(tmplType, tmplData)

	genReq := llm.GenerationRequest{
		TemplateType: tmplType,
		TemplateBody: baseTmpl,
		ProjectName:  project.Name,
		Description:  project.Description,
		Sources:      sources,
	}
	if llmCfg != nil {
		genReq.Model = llmCfg.DefaultModel
		genReq.Temperature = llmCfg.Temperature
		genReq.MaxTokens = llmCfg.MaxTokens
	}

	genResp, err := provider.Generate(ctx, genReq)
	if err != nil {
		return baseTmpl, "template-fallback", 0
	}

	_ = s.repo.RecordTokenUsage(&models.TokenUsage{
		ProjectID:    project.ID,
		GenerationID: &gen.ID,
		StageID:      &stageID,
		Operation:    "direct_generation",
		ModelUsed:    genResp.ModelUsed,
		InputTokens:  genResp.InputTokens,
		OutputTokens: genResp.OutputTokens,
		TotalTokens:  genResp.TokensUsed,
	})

	return genResp.MarkdownContent, genResp.ModelUsed, genResp.TokensUsed
}

func (s *GenerationService) synthesizeSectionBySection(ctx context.Context, gen *models.Generation, project *models.Project, stageID string, tmplType, systemMapMD string, provider llm.Provider, llmCfg *models.LLMConfig) (string, string, int) {
	sections := []struct {
		Title string
		Goal  string
		Query string
	}{
		{
			Title: "1. Executive Summary & Vision",
			Goal:  "Summarize system purpose, capabilities, and primary objectives.",
			Query: "overview purpose vision architecture",
		},
		{
			Title: "2. Architecture & Data Flow",
			Goal:  "Document component hierarchy, data pipelines, and database interactions.",
			Query: "architecture component router service database table flow",
		},
		{
			Title: "3. API & Integration Endpoints",
			Goal:  "Document REST routes, endpoints, request/response formats, and parameters.",
			Query: "endpoint route handler api http get post",
		},
		{
			Title: "4. Security & Operational Verification",
			Goal:  "Document authentication, environment configs, diagnostics, and runbooks.",
			Query: "security auth token config environment health metrics",
		},
	}

	var sectionMDs []string
	lastModel := "multi-stage-synthesizer"
	totalTokens := 0

	for _, sec := range sections {
		chunks, _ := s.repo.SearchSourceChunks(project.ID, sec.Query, nil, 6)
		secReq := llm.SectionGenerationRequest{
			SectionTitle: sec.Title,
			SectionGoal:  sec.Goal,
			SystemMap:    systemMapMD,
			ProjectName:  project.Name,
			Chunks:       chunks,
		}
		if llmCfg != nil {
			secReq.Model = llmCfg.DefaultModel
			secReq.Temperature = llmCfg.Temperature
			secReq.MaxTokens = llmCfg.MaxTokens
		}

		resp, err := provider.GenerateSection(ctx, secReq)
		if err == nil && resp != nil {
			sectionMDs = append(sectionMDs, resp.Markdown)
			lastModel = resp.ModelUsed
			totalTokens += resp.TokensUsed

			_ = s.repo.RecordTokenUsage(&models.TokenUsage{
				ProjectID:    project.ID,
				GenerationID: &gen.ID,
				StageID:      &stageID,
				Operation:    "section_population",
				ModelUsed:    resp.ModelUsed,
				InputTokens:  resp.InputTokens,
				OutputTokens: resp.OutputTokens,
				TotalTokens:  resp.TokensUsed,
			})
		}
	}

	docTitle := templates.GetTemplateTitle(tmplType, project.Name)
	var fullDoc strings.Builder
	fullDoc.WriteString(fmt.Sprintf("# %s\n\n**Project**: %s  \n**Status**: IN REVIEW  \n**Synthesized**: %s  \n**Model**: %s  \n\n---\n\n",
		docTitle, project.Name, time.Now().Format("2006-01-02"), lastModel))

	if systemMapMD != "" {
		fullDoc.WriteString("## System Topology\n" + systemMapMD + "\n\n---\n\n")
	}

	for _, sMD := range sectionMDs {
		fullDoc.WriteString(sMD + "\n\n")
	}

	return fullDoc.String(), lastModel, totalTokens
}

// UpdateWithFeedback performs the AI revision loop
func (s *GenerationService) UpdateWithFeedback(ctx context.Context, documentID, userID string) (*models.DocumentVersion, error) {
	doc, err := s.repo.GetDocumentByID(documentID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	var openFeedback []models.DocumentFeedback
	for _, fb := range doc.Feedback {
		if fb.Status == models.FeedbackOpen {
			openFeedback = append(openFeedback, fb)
		}
	}

	if len(openFeedback) == 0 {
		return nil, fmt.Errorf("no open feedback items to address")
	}

	project, _ := s.repo.GetProjectByID(doc.ProjectID)
	sources, _ := s.repo.ListSourcesByProject(doc.ProjectID)

	llmCfg, _ := s.repo.GetActiveLLMConfig()
	var decryptedKey string
	if llmCfg != nil && llmCfg.APIKeyEncrypted != "" {
		decryptedKey, _ = crypto.Decrypt(llmCfg.APIKeyEncrypted, s.cfg.EncryptionKey)
	}

	provider := llm.NewProvider(llmCfg, decryptedKey)

	projectName := "Project"
	if project != nil {
		projectName = project.Name
	}

	req := llm.FeedbackRevisionRequest{
		CurrentMarkdown: doc.ContentMD,
		FeedbackItems:   openFeedback,
		ProjectName:     projectName,
		Sources:         sources,
	}
	if llmCfg != nil {
		req.Model = llmCfg.DefaultModel
		req.Temperature = llmCfg.Temperature
		req.MaxTokens = llmCfg.MaxTokens
	}

	revResp, err := provider.ReviseWithFeedback(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to revise with feedback: %w", err)
	}

	_ = s.repo.RecordTokenUsage(&models.TokenUsage{
		ProjectID:    doc.ProjectID,
		DocumentID:   &doc.ID,
		Operation:    "feedback_revision",
		ModelUsed:    revResp.ModelUsed,
		InputTokens:  revResp.InputTokens,
		OutputTokens: revResp.OutputTokens,
		TotalTokens:  revResp.TokensUsed,
	})

	// Create new document version
	newVersion, err := s.repo.UpdateDocumentContent(doc.ID, revResp.UpdatedMarkdown, doc.Title, userID, revResp.ChangeSummary)
	if err != nil {
		return nil, fmt.Errorf("failed to save new version: %w", err)
	}

	// Mark addressed feedback
	_ = s.repo.MarkOpenFeedbackAddressed(doc.ID)

	return newVersion, nil
}
