package llm_test

import (
	"context"
	"strings"
	"testing"

	"docforge/internal/llm"
	"docforge/internal/models"
)

func TestMockProviderMultiStage(t *testing.T) {
	provider := llm.NewMockProvider("mock-test-synthesizer")

	// 1. System Map
	sysResp, err := provider.GenerateSystemMap(context.Background(), llm.SystemMapRequest{
		ProjectName: "Billing Core",
		Description: "Handles payments",
	})
	if err != nil {
		t.Fatalf("GenerateSystemMap failed: %v", err)
	}
	if !strings.Contains(sysResp.Markdown, "System Architecture Map") {
		t.Errorf("expected System Architecture Map in markdown, got: %s", sysResp.Markdown)
	}

	// 2. Section Generation
	secResp, err := provider.GenerateSection(context.Background(), llm.SectionGenerationRequest{
		SectionTitle: "API Contracts",
		SectionGoal:  "Document REST endpoints",
		ProjectName:  "Billing Core",
		Chunks: []models.SourceChunk{
			{FilePath: "api/routes.go", LineStart: 10, LineEnd: 30, SymbolName: "PostCharge"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateSection failed: %v", err)
	}
	if !strings.Contains(secResp.Markdown, "api/routes.go#L10-L30") {
		t.Errorf("expected citation [api/routes.go#L10-L30], got: %s", secResp.Markdown)
	}

	// 3. Self Correction
	scResp, err := provider.SelfCorrect(context.Background(), llm.SelfCorrectionRequest{
		CurrentMarkdown: "This page describes the task for the blocker.",
		Issues:          []string{"Avoid using page, task, blocker"},
		ProjectName:     "Billing Core",
	})
	if err != nil {
		t.Fatalf("SelfCorrect failed: %v", err)
	}
	if strings.Contains(scResp.UpdatedMarkdown, " page ") || strings.Contains(scResp.UpdatedMarkdown, " task ") {
		t.Errorf("expected avoided words to be replaced, got: %s", scResp.UpdatedMarkdown)
	}
}

func TestNewProviderResolution(t *testing.T) {
	// Nil config -> mock
	pNil := llm.NewProvider(nil, "")
	if pNil.Name() != "mock-llm" {
		t.Errorf("expected mock-llm for nil config, got %s", pNil.Name())
	}

	// Mock config
	pMock := llm.NewProvider(&models.LLMConfig{Provider: "mock", DefaultModel: "test-model"}, "")
	if pMock.Name() != "mock-llm" {
		t.Errorf("expected mock-llm for mock config, got %s", pMock.Name())
	}

	// Local/Ollama config without API key -> MUST NOT be mock!
	pOllama := llm.NewProvider(&models.LLMConfig{
		Provider:     "azure",
		BaseURL:      "http://localhost:11434/v1",
		DefaultModel: "gemma4:31b-cloud",
	}, "")
	if pOllama.Name() != "openai-compatible" {
		t.Fatalf("expected openai-compatible for local provider without key, got %s", pOllama.Name())
	}

	// Anthropic
	pAnthropic := llm.NewProvider(&models.LLMConfig{
		Provider:     "anthropic",
		BaseURL:      "https://api.anthropic.com/v1",
		DefaultModel: "claude-3-5-sonnet",
	}, "test-key")
	if pAnthropic.Name() != "anthropic" {
		t.Fatalf("expected anthropic provider, got %s", pAnthropic.Name())
	}
}

