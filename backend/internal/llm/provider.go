package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"docforge/internal/models"
)

type GenerationRequest struct {
	TemplateType string
	TemplateBody string
	ProjectName  string
	Description  string
	Sources      []models.Source
	Model        string
	Temperature  float64
	MaxTokens    int
}

type GenerationResponse struct {
	MarkdownContent string
	ModelUsed       string
	TokensUsed      int
}

type FeedbackRevisionRequest struct {
	CurrentMarkdown string
	FeedbackItems   []models.DocumentFeedback
	ProjectName     string
	Sources         []models.Source
	Model           string
	Temperature     float64
	MaxTokens       int
}

type FeedbackRevisionResponse struct {
	UpdatedMarkdown string
	ChangeSummary   string
	ModelUsed       string
}

type Provider interface {
	Name() string
	Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error)
	ReviseWithFeedback(ctx context.Context, req FeedbackRevisionRequest) (*FeedbackRevisionResponse, error)
}

// Factory creates a provider based on config
func NewProvider(cfg *models.LLMConfig, decryptedKey string) Provider {
	if cfg == nil || decryptedKey == "" {
		return NewMockProvider("mock-provider")
	}

	switch strings.ToLower(cfg.Provider) {
	case "anthropic":
		return NewAnthropicProvider(decryptedKey, cfg.BaseURL, cfg.DefaultModel)
	case "mock":
		return NewMockProvider(cfg.DefaultModel)
	case "openai", "openai-compatible", "azure", "local", "ollama":
		fallthrough
	default:
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return NewOpenAIProvider(decryptedKey, baseURL, cfg.DefaultModel)
	}
}

// ----------------- OPENAI COMPATIBLE -----------------

type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	defaultModel string
	client     *http.Client
}

func NewOpenAIProvider(apiKey, baseURL, defaultModel string) *OpenAIProvider {
	baseURL = strings.TrimRight(baseURL, "/")
	if defaultModel == "" {
		defaultModel = "gpt-4o"
	}
	return &OpenAIProvider{
		apiKey:       apiKey,
		baseURL:      baseURL,
		defaultModel: defaultModel,
		client:       &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai-compatible"
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []openAIMessageContent
}

type openAIMessageContent struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	systemPrompt := `You are DocForge, a senior technical documentation specialist.
Synthesize high-quality product technical documentation based SOLELY on the provided sources.
GROUND TRUTH RULES:
1. Code, test cases, and configuration files are the highest authority. Existing docs/sheets are secondary.
2. Mark any reasonable inference as [INFERRED] and any missing information as [MISSING]. Do not invent undocumented behavior or false APIs.
3. Adhere strictly to the requested Markdown template structure.`

	userPrompt := buildGenerationUserPrompt(req)

	body := openAIChatRequest{
		Model: model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	if body.MaxTokens == 0 {
		body.MaxTokens = 4096
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (raw: %s)", err, string(respBytes))
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("openai api error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from openai")
	}

	return &GenerationResponse{
		MarkdownContent: cleanMarkdownOutput(chatResp.Choices[0].Message.Content),
		ModelUsed:       model,
		TokensUsed:      chatResp.Usage.TotalTokens,
	}, nil
}

func (p *OpenAIProvider) ReviseWithFeedback(ctx context.Context, req FeedbackRevisionRequest) (*FeedbackRevisionResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	systemPrompt := `You are DocForge, a senior technical documentation specialist.
You are revising an existing technical document based on structured human reviewer feedback.
Update the Markdown content carefully addressing each feedback item.
Keep untouched sections intact.
At the end of your response, provide a brief summary of changes in this format:
<!-- CHANGE_SUMMARY -->
<summary bullet points>`

	userPrompt := buildFeedbackRevisionPrompt(req)

	body := openAIChatRequest{
		Model: model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 4096
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, err
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("api error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no output returned")
	}

	raw := chatResp.Choices[0].Message.Content
	content, summary := parseChangeSummary(raw)

	return &FeedbackRevisionResponse{
		UpdatedMarkdown: cleanMarkdownOutput(content),
		ChangeSummary:   summary,
		ModelUsed:       model,
	}, nil
}

// ----------------- ANTHROPIC -----------------

type AnthropicProvider struct {
	apiKey     string
	baseURL    string
	defaultModel string
	client     *http.Client
}

func NewAnthropicProvider(apiKey, baseURL, defaultModel string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if defaultModel == "" {
		defaultModel = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicProvider{
		apiKey:       apiKey,
		baseURL:      baseURL,
		defaultModel: defaultModel,
		client:       &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

type anthropicRequest struct {
	Model       string              `json:"model"`
	System      string              `json:"system"`
	Messages    []anthropicMessage  `json:"messages"`
	MaxTokens   int                 `json:"max_tokens"`
	Temperature float64             `json:"temperature"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *AnthropicProvider) Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	systemPrompt := `You are DocForge, a senior technical documentation specialist.
Synthesize high-quality product technical documentation based SOLELY on the provided sources.
GROUND TRUTH RULES:
1. Code, test cases, and configuration files are the highest authority. Existing docs/sheets are secondary.
2. Mark any reasonable inference as [INFERRED] and any missing information as [MISSING]. Do not invent undocumented behavior or false APIs.
3. Adhere strictly to the requested Markdown template structure.`

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := anthropicRequest{
		Model:       model,
		System:      systemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: buildGenerationUserPrompt(req)}},
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/messages", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var aResp anthropicResponse
	if err := json.Unmarshal(respBytes, &aResp); err != nil {
		return nil, err
	}

	if aResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", aResp.Error.Message)
	}

	var text string
	for _, c := range aResp.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}

	return &GenerationResponse{
		MarkdownContent: cleanMarkdownOutput(text),
		ModelUsed:       model,
		TokensUsed:      aResp.Usage.InputTokens + aResp.Usage.OutputTokens,
	}, nil
}

func (p *AnthropicProvider) ReviseWithFeedback(ctx context.Context, req FeedbackRevisionRequest) (*FeedbackRevisionResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	systemPrompt := `You are DocForge, a senior technical documentation specialist.
You are revising an existing technical document based on structured human reviewer feedback.
Update the Markdown content carefully addressing each feedback item.
Keep untouched sections intact.
At the end of your response, provide a brief summary of changes in this format:
<!-- CHANGE_SUMMARY -->
<summary bullet points>`

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := anthropicRequest{
		Model:       model,
		System:      systemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: buildFeedbackRevisionPrompt(req)}},
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/messages", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var aResp anthropicResponse
	if err := json.Unmarshal(respBytes, &aResp); err != nil {
		return nil, err
	}

	if aResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", aResp.Error.Message)
	}

	var text string
	for _, c := range aResp.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}

	content, summary := parseChangeSummary(text)
	return &FeedbackRevisionResponse{
		UpdatedMarkdown: cleanMarkdownOutput(content),
		ChangeSummary:   summary,
		ModelUsed:       model,
	}, nil
}

// ----------------- MOCK PROVIDER (OFFLINE/LOCAL DEV) -----------------

type MockProvider struct {
	modelName string
}

func NewMockProvider(modelName string) *MockProvider {
	if modelName == "" {
		modelName = "docforge-builtin-synthesizer"
	}
	return &MockProvider{modelName: modelName}
}

func (p *MockProvider) Name() string {
	return "mock-llm"
}

func (p *MockProvider) Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error) {
	// Synthesize realistic document incorporating source snippets
	var sourceHighlights []string
	for _, s := range req.Sources {
		snippet := s.ExtractedText
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		sourceHighlights = append(sourceHighlights, fmt.Sprintf("- **%s** (%s): %s", s.Name, s.Type, strings.ReplaceAll(snippet, "\n", " ")))
	}

	sourcesBlock := strings.Join(sourceHighlights, "\n")
	if sourcesBlock == "" {
		sourcesBlock = "No source details provided."
	}

	var content string
	switch req.TemplateType {
	case "product_spec":
		content = fmt.Sprintf(`# Product & Feature Specification: %s

**Project**: %s  
**Status**: IN REVIEW  
**Synthesized**: %s  
**Model**: %s  

---

## 1. Executive Summary & Vision
%s is designed to provide robust product capabilities based on analyzed system assets.

## 2. Ingested Technical Evidence
The following primary source materials were analyzed:
%s

## 3. Core Functional Requirements
- **FR-1**: System must support unified data ingestion and validation for all defined entities.
- **FR-2**: All operations must enforce authentication boundaries and audit trails.
- **FR-3**: Asynchronous processing pipeline guarantees non-blocking user workflows.

## 4. API & Integration Endpoints
- [x] RESTful API endpoints with structured JSON schemas.
- [x] Role-based access control protecting administrative endpoints.

## 5. Security & Verification
- All cryptographic keys and external integration tokens are encrypted using AES-GCM.
- Blocking issues must be addressed before final sign-off.
`, req.ProjectName, req.ProjectName, time.Now().Format("2006-01-02"), p.modelName, req.Description, sourcesBlock)

	case "adr":
		content = fmt.Sprintf(`# ADR: %s - Architectural Baseline

- **Status**: Accepted
- **Date**: %s
- **Project**: %s
- **Model**: %s

---

## 1. Context & Problem Statement
%s requires a maintainable, high-performance architecture with minimal operational friction.

## 2. Decision
We chose a modular architecture with pure-Go persistence and an asynchronous execution loop.

### Ground Truth References
%s

## 3. Consequences
- **Positive**: Zero external database dependencies; self-contained single static deployment.
- **Trade-off**: Requires in-process lock discipline for concurrent transactions.
`, req.ProjectName, time.Now().Format("2006-01-02"), req.ProjectName, p.modelName, req.Description, sourcesBlock)

	case "release_notes":
		content = fmt.Sprintf(`# Release Notes: %s Initial Release

**Release Date**: %s  
**Version**: 1.0.0-rc  
**Project**: %s  

---

## 🚀 Key Highlights
Initial synthesized documentation baseline for %s.

## 📦 Verified Sources
%s

## 🔒 Security & Governance
- Enforced hard gates on blocking feedback before document sign-off.
- Verified encrypted secret storage at rest.
`, req.ProjectName, time.Now().Format("2006-01-02"), req.ProjectName, req.ProjectName, sourcesBlock)

	case "runbook":
		content = fmt.Sprintf(`# Runbook: %s Operations

**System Tier**: Tier 1  
**Last Verified**: %s  
**Project**: %s  

---

## 1. Operational Overview
Operational procedures and troubleshooting guide for %s.

## 2. System Verification & Diagnostics
- Verify service health at '/health'
- Monitor background generation job queues in SQLite

## 3. Sources & Dependencies
%s
`, req.ProjectName, time.Now().Format("2006-01-02"), req.ProjectName, req.ProjectName, sourcesBlock)

	default:
		content = req.TemplateBody
	}

	return &GenerationResponse{
		MarkdownContent: content,
		ModelUsed:       p.modelName,
		TokensUsed:      450,
	}, nil
}

func (p *MockProvider) ReviseWithFeedback(ctx context.Context, req FeedbackRevisionRequest) (*FeedbackRevisionResponse, error) {
	var changes []string
	updated := req.CurrentMarkdown

	for _, fb := range req.FeedbackItems {
		itemSummary := fmt.Sprintf("- Addressed %s (%s): %s", fb.Category, fb.Status, fb.Content)
		changes = append(changes, itemSummary)
	}

	revisionNotes := fmt.Sprintf("\n\n---\n### 🔄 Revision Notes (%s)\n%s\n", time.Now().Format("2006-01-02 15:04"), strings.Join(changes, "\n"))
	updated += revisionNotes

	summaryText := fmt.Sprintf("Applied %d reviewer feedback items: incorporated technical clarifications and addressed blocking items.", len(req.FeedbackItems))

	return &FeedbackRevisionResponse{
		UpdatedMarkdown: updated,
		ChangeSummary:   summaryText,
		ModelUsed:       p.modelName,
	}, nil
}

// ----------------- HELPERS -----------------

func buildGenerationUserPrompt(req GenerationRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\n", req.ProjectName))
	if req.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n\n", req.Description))
	}

	sb.WriteString(fmt.Sprintf("Template Type: %s\n", req.TemplateType))
	sb.WriteString("Base Template Markdown:\n```markdown\n")
	sb.WriteString(req.TemplateBody)
	sb.WriteString("\n```\n\n")

	sb.WriteString("=== INGESTED TECHNICAL SOURCES ===\n")
	for i, s := range req.Sources {
		sb.WriteString(fmt.Sprintf("\n--- Source [%d]: %s (Type: %s, Mime: %s) ---\n", i+1, s.Name, s.Type, s.MimeType))
		text := s.ExtractedText
		if len(text) > 8000 {
			text = text[:8000] + "\n...[truncated]"
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	sb.WriteString("\nSynthesize the complete document conforming to the template above using the source facts. Return ONLY clean Markdown.")
	return sb.String()
}

func buildFeedbackRevisionPrompt(req FeedbackRevisionRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\n\n", req.ProjectName))
	sb.WriteString("=== CURRENT DOCUMENT MARKDOWN ===\n```markdown\n")
	sb.WriteString(req.CurrentMarkdown)
	sb.WriteString("\n```\n\n")

	sb.WriteString("=== STRUCTURED FEEDBACK ITEMS TO ADDRESS ===\n")
	for i, f := range req.FeedbackItems {
		blockingStr := ""
		if f.IsBlocking {
			blockingStr = " [BLOCKING]"
		}
		anchorStr := ""
		if f.SectionAnchor != "" {
			anchorStr = fmt.Sprintf(" [Section: %s]", f.SectionAnchor)
		}
		sb.WriteString(fmt.Sprintf("%d.%s%s Category: %s\nFeedback: %s\n\n", i+1, blockingStr, anchorStr, f.Category, f.Content))
	}

	sb.WriteString("\nRevise the document incorporating the feedback above. Keep unmodified sections intact. At the very end include <!-- CHANGE_SUMMARY --> followed by bullet points.")
	return sb.String()
}

func parseChangeSummary(output string) (string, string) {
	delim := "<!-- CHANGE_SUMMARY -->"
	idx := strings.Index(output, delim)
	if idx == -1 {
		return output, "Document updated based on reviewer feedback."
	}
	content := strings.TrimSpace(output[:idx])
	summary := strings.TrimSpace(output[idx+len(delim):])
	if summary == "" {
		summary = "Document updated based on reviewer feedback."
	}
	return content, summary
}

func cleanMarkdownOutput(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```markdown ... ``` wrapper if the model enveloped the whole response
	if strings.HasPrefix(s, "```markdown") && strings.HasSuffix(s, "```") {
		s = strings.TrimPrefix(s, "```markdown")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	} else if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}
