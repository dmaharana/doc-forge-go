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
	workerCancel context.CancelFunc
	wg           sync.WaitGroup
}

func NewGenerationService(cfg *config.Config, repo *repository.Repository, notifier *NotificationService) *GenerationService {
	return &GenerationService{
		cfg:      cfg,
		repo:     repo,
		notifier: notifier,
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
	if gen.LLMConfigID != nil && *gen.LLMConfigID != "" {
		cfgList, _ := s.repo.ListLLMConfigs()
		for _, c := range cfgList {
			if c.ID == *gen.LLMConfigID {
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
		tmplData := templates.TemplateData{
			Title:       templates.GetTemplateTitle(tmplType, project.Name),
			ProjectName: project.Name,
			Date:        time.Now().Format("2006-01-02"),
			Summary:     project.Description,
			Sources:     sources,
		}

		baseTmpl, err := templates.RenderTemplate(tmplType, tmplData)
		if err != nil {
			log.Printf("[WORKER] Error rendering base template %s: %v", tmplType, err)
			continue
		}

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
			log.Printf("[WORKER] LLM generation error for %s: %v", tmplType, err)
			// fallback to base template content
			genResp = &llm.GenerationResponse{
				MarkdownContent: baseTmpl,
				ModelUsed:       "template-fallback",
			}
		}

		lastModelUsed = genResp.ModelUsed

		doc := &models.Document{
			GenerationID: &gen.ID,
			ProjectID:    project.ID,
			Title:        templates.GetTemplateTitle(tmplType, project.Name),
			TemplateType: tmplType,
			ContentMD:    genResp.MarkdownContent,
			Status:       models.DocStatusInReview,
		}

		if err := s.repo.CreateDocument(doc, gen.RequestedBy); err != nil {
			log.Printf("[WORKER] Error saving document %s: %v", tmplType, err)
		} else {
			docsCreated++
		}
	}

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

	// Create new document version
	newVersion, err := s.repo.UpdateDocumentContent(doc.ID, revResp.UpdatedMarkdown, doc.Title, userID, revResp.ChangeSummary)
	if err != nil {
		return nil, fmt.Errorf("failed to save new version: %w", err)
	}

	// Mark addressed feedback
	_ = s.repo.MarkOpenFeedbackAddressed(doc.ID)

	return newVersion, nil
}
