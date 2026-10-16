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
	InputTokens     int
	OutputTokens    int
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
	InputTokens     int
	OutputTokens    int
	TokensUsed      int
}

type SystemMapRequest struct {
	ProjectName string
	Description string
	Chunks      []models.SourceChunk
	Model       string
	Temperature float64
	MaxTokens   int
}

type SystemMapResponse struct {
	Markdown     string
	ModelUsed    string
	InputTokens  int
	OutputTokens int
	TokensUsed   int
}

type SectionGenerationRequest struct {
	SectionTitle string
	SectionGoal  string
	SystemMap    string
	TemplatePart string
	ProjectName  string
	Chunks       []models.SourceChunk
	Model        string
	Temperature  float64
	MaxTokens    int
}

type SectionGenerationResponse struct {
	Markdown     string
	ModelUsed    string
	InputTokens  int
	OutputTokens int
	TokensUsed   int
}

type SelfCorrectionRequest struct {
	CurrentMarkdown string
	Issues          []string
	ProjectName     string
	Model           string
	Temperature     float64
	MaxTokens       int
}

type SelfCorrectionResponse struct {
	UpdatedMarkdown string
	ModelUsed       string
	InputTokens     int
	OutputTokens    int
	TokensUsed      int
}

type Provider interface {
	Name() string
	Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error)
	ReviseWithFeedback(ctx context.Context, req FeedbackRevisionRequest) (*FeedbackRevisionResponse, error)
	GenerateSystemMap(ctx context.Context, req SystemMapRequest) (*SystemMapResponse, error)
	GenerateSection(ctx context.Context, req SectionGenerationRequest) (*SectionGenerationResponse, error)
	SelfCorrect(ctx context.Context, req SelfCorrectionRequest) (*SelfCorrectionResponse, error)
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
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) callChat(ctx context.Context, systemPrompt, userPrompt, model string, temperature float64, maxTokens int) (string, int, int, int, error) {
	if model == "" {
		model = p.defaultModel
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := openAIChatRequest{
		Model: model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return "", 0, 0, 0, err
	}

	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return "", 0, 0, 0, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, 0, err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to decode response: %w (raw: %s)", err, string(respBytes))
	}

	if chatResp.Error != nil {
		return "", 0, 0, 0, fmt.Errorf("openai api error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", 0, 0, 0, fmt.Errorf("no choices returned from openai")
	}

	inTok := chatResp.Usage.PromptTokens
	outTok := chatResp.Usage.CompletionTokens
	totTok := chatResp.Usage.TotalTokens
	if totTok == 0 && (inTok > 0 || outTok > 0) {
		totTok = inTok + outTok
	}

	return cleanMarkdownOutput(chatResp.Choices[0].Message.Content), inTok, outTok, totTok, nil
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
	content, inTok, outTok, tokens, err := p.callChat(ctx, systemPrompt, userPrompt, model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}

	return &GenerationResponse{
		MarkdownContent: content,
		ModelUsed:       model,
		InputTokens:     inTok,
		OutputTokens:    outTok,
		TokensUsed:      tokens,
	}, nil
}

func (p *OpenAIProvider) GenerateSystemMap(ctx context.Context, req SystemMapRequest) (*SystemMapResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	systemPrompt := `You are DocForge System Mapper.
Synthesize a concise, high-level architecture component map and data-flow overview based on the provided technical source chunks.
Include a Mermaid flowchart (flowchart TD) of key components and data paths. Return ONLY clean Markdown.`

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\nDescription: %s\n\n=== SOURCE CHUNKS ===\n", req.ProjectName, req.Description))
	for _, c := range req.Chunks {
		sb.WriteString(fmt.Sprintf("\n%s\n%s\n", c.SearchHeader, c.Content))
	}

	content, inTok, outTok, tokens, err := p.callChat(ctx, systemPrompt, sb.String(), model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}
	return &SystemMapResponse{
		Markdown:     content,
		ModelUsed:    model,
		InputTokens:  inTok,
		OutputTokens: outTok,
		TokensUsed:   tokens,
	}, nil
}

func (p *OpenAIProvider) GenerateSection(ctx context.Context, req SectionGenerationRequest) (*SectionGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	systemPrompt := `You are DocForge Section Generator.
Synthesize the requested document section adhering strictly to the provided template and grounding chunks.
GROUND TRUTH RULES:
1. Use ONLY the provided sources. Cite originating file and line numbers inline as [filename#Lstart-Lend].
2. Mark any inference or assumption as [INFERRED]. Mark any missing details as [MISSING].
3. Do not invent endpoints, parameters, or behavior not present in the sources. Return ONLY clean Markdown.`

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\nSection: %s\nSection Goal: %s\n\n", req.ProjectName, req.SectionTitle, req.SectionGoal))
	if req.SystemMap != "" {
		sb.WriteString("=== SYSTEM MAP SKELETON ===\n" + req.SystemMap + "\n\n")
	}
	if req.TemplatePart != "" {
		sb.WriteString("=== SECTION TEMPLATE ===\n```markdown\n" + req.TemplatePart + "\n```\n\n")
	}
	sb.WriteString("=== GROUNDING SOURCE CHUNKS ===\n")
	for _, c := range req.Chunks {
		sb.WriteString(fmt.Sprintf("\n%s\n%s\n", c.SearchHeader, c.Content))
	}

	content, inTok, outTok, tokens, err := p.callChat(ctx, systemPrompt, sb.String(), model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}
	return &SectionGenerationResponse{
		Markdown:     content,
		ModelUsed:    model,
		InputTokens:  inTok,
		OutputTokens: outTok,
		TokensUsed:   tokens,
	}, nil
}

func (p *OpenAIProvider) SelfCorrect(ctx context.Context, req SelfCorrectionRequest) (*SelfCorrectionResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	systemPrompt := `You are DocForge Pre-Review Self-Correction.
Revise the provided technical documentation to resolve the flagged pre-review issues.
- Fix broken section links or inconsistent headings.
- Enforce canonical glossary terminology (replace avoided terms).
- Remove repetitive boilerplate phrasing or AI hedging language.
Keep all technical facts and grounding citations intact. Return ONLY clean Markdown.`

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\n\n=== CURRENT DRAFT ===\n```markdown\n%s\n```\n\n", req.ProjectName, req.CurrentMarkdown))
	sb.WriteString("=== DETECTED PRE-REVIEW ISSUES TO CORRECT ===\n")
	for i, issue := range req.Issues {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue))
	}

	content, inTok, outTok, tokens, err := p.callChat(ctx, systemPrompt, sb.String(), model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}
	return &SelfCorrectionResponse{
		UpdatedMarkdown: content,
		ModelUsed:       model,
		InputTokens:     inTok,
		OutputTokens:    outTok,
		TokensUsed:      tokens,
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
	raw, inTok, outTok, tokens, err := p.callChat(ctx, systemPrompt, userPrompt, model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}

	content, summary := parseChangeSummary(raw)

	return &FeedbackRevisionResponse{
		UpdatedMarkdown: cleanMarkdownOutput(content),
		ChangeSummary:   summary,
		ModelUsed:       model,
		InputTokens:     inTok,
		OutputTokens:    outTok,
		TokensUsed:      tokens,
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

func (p *AnthropicProvider) callMessages(ctx context.Context, systemPrompt, userPrompt, model string, temperature float64, maxTokens int) (string, int, int, int, error) {
	if model == "" {
		model = p.defaultModel
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := anthropicRequest{
		Model:       model,
		System:      systemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: userPrompt}},
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return "", 0, 0, 0, err
	}

	url := fmt.Sprintf("%s/messages", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return "", 0, 0, 0, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, 0, err
	}

	var aResp anthropicResponse
	if err := json.Unmarshal(respBytes, &aResp); err != nil {
		return "", 0, 0, 0, err
	}

	if aResp.Error != nil {
		return "", 0, 0, 0, fmt.Errorf("anthropic error: %s", aResp.Error.Message)
	}

	var text string
	for _, c := range aResp.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}

	inTok := aResp.Usage.InputTokens
	outTok := aResp.Usage.OutputTokens
	tokens := inTok + outTok
	return cleanMarkdownOutput(text), inTok, outTok, tokens, nil
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

	userPrompt := buildGenerationUserPrompt(req)
	content, inTok, outTok, tokens, err := p.callMessages(ctx, systemPrompt, userPrompt, model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}

	return &GenerationResponse{
		MarkdownContent: content,
		ModelUsed:       model,
		InputTokens:     inTok,
		OutputTokens:    outTok,
		TokensUsed:      tokens,
	}, nil
}

func (p *AnthropicProvider) GenerateSystemMap(ctx context.Context, req SystemMapRequest) (*SystemMapResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	systemPrompt := `You are DocForge System Mapper.
Generate a concise, high-level architecture component map and data-flow overview based on the provided technical source chunks.
Include a Mermaid flowchart (flowchart TD) of key components and data paths. Return ONLY clean Markdown.`

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\nDescription: %s\n\n=== SOURCE CHUNKS ===\n", req.ProjectName, req.Description))
	for _, c := range req.Chunks {
		sb.WriteString(fmt.Sprintf("\n%s\n%s\n", c.SearchHeader, c.Content))
	}

	content, inTok, outTok, tokens, err := p.callMessages(ctx, systemPrompt, sb.String(), model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}
	return &SystemMapResponse{
		Markdown:     content,
		ModelUsed:    model,
		InputTokens:  inTok,
		OutputTokens: outTok,
		TokensUsed:   tokens,
	}, nil
}

func (p *AnthropicProvider) GenerateSection(ctx context.Context, req SectionGenerationRequest) (*SectionGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	systemPrompt := `You are DocForge Section Generator.
Synthesize the requested document section adhering strictly to the provided template and grounding chunks.
GROUND TRUTH RULES:
1. Use ONLY the provided sources. Cite originating file and line numbers inline as [filename#Lstart-Lend].
2. Mark any inference or assumption as [INFERRED]. Mark any missing details as [MISSING].
3. Do not invent endpoints, parameters, or behavior not present in the sources. Return ONLY clean Markdown.`

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\nSection: %s\nSection Goal: %s\n\n", req.ProjectName, req.SectionTitle, req.SectionGoal))
	if req.SystemMap != "" {
		sb.WriteString("=== SYSTEM MAP SKELETON ===\n" + req.SystemMap + "\n\n")
	}
	if req.TemplatePart != "" {
		sb.WriteString("=== SECTION TEMPLATE ===\n```markdown\n" + req.TemplatePart + "\n```\n\n")
	}
	sb.WriteString("=== GROUNDING SOURCE CHUNKS ===\n")
	for _, c := range req.Chunks {
		sb.WriteString(fmt.Sprintf("\n%s\n%s\n", c.SearchHeader, c.Content))
	}

	content, inTok, outTok, tokens, err := p.callMessages(ctx, systemPrompt, sb.String(), model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}
	return &SectionGenerationResponse{
		Markdown:     content,
		ModelUsed:    model,
		InputTokens:  inTok,
		OutputTokens: outTok,
		TokensUsed:   tokens,
	}, nil
}

func (p *AnthropicProvider) SelfCorrect(ctx context.Context, req SelfCorrectionRequest) (*SelfCorrectionResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	systemPrompt := `You are DocForge Pre-Review Self-Correction.
Revise the provided technical documentation to resolve the flagged pre-review issues.
- Fix broken section links or inconsistent headings.
- Enforce canonical glossary terminology (replace avoided terms).
- Remove repetitive boilerplate phrasing or AI hedging language.
Keep all technical facts and grounding citations intact. Return ONLY clean Markdown.`

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\n\n=== CURRENT DRAFT ===\n```markdown\n%s\n```\n\n", req.ProjectName, req.CurrentMarkdown))
	sb.WriteString("=== DETECTED PRE-REVIEW ISSUES TO CORRECT ===\n")
	for i, issue := range req.Issues {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue))
	}

	content, inTok, outTok, tokens, err := p.callMessages(ctx, systemPrompt, sb.String(), model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}
	return &SelfCorrectionResponse{
		UpdatedMarkdown: content,
		ModelUsed:       model,
		InputTokens:     inTok,
		OutputTokens:    outTok,
		TokensUsed:      tokens,
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

	userPrompt := buildFeedbackRevisionPrompt(req)
	text, inTok, outTok, tokens, err := p.callMessages(ctx, systemPrompt, userPrompt, model, req.Temperature, req.MaxTokens)
	if err != nil {
		return nil, err
	}

	content, summary := parseChangeSummary(text)
	return &FeedbackRevisionResponse{
		UpdatedMarkdown: cleanMarkdownOutput(content),
		ChangeSummary:   summary,
		ModelUsed:       model,
		InputTokens:     inTok,
		OutputTokens:    outTok,
		TokensUsed:      tokens,
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
		InputTokens:     300,
		OutputTokens:    150,
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
		InputTokens:     150,
		OutputTokens:    100,
		TokensUsed:      250,
	}, nil
}

func (p *MockProvider) GenerateSystemMap(ctx context.Context, req SystemMapRequest) (*SystemMapResponse, error) {
	md := fmt.Sprintf(`# System Architecture Map: %s

## 1. High-Level Component Topology
%s is structured into decoupled modules with pure-Go persistence.

`+"```mermaid"+`
flowchart TD
    Client["Web UI / API Client"] --> Router["HTTP Router & Middleware"]
    Router --> Service["Service Layer"]
    Service --> Storage[("SQLite Persistence (WAL)")]
    Service --> Worker["Async Job Worker Pipeline"]
`+"```"+`

## 2. Core Service Components
- **API Router**: Handles REST request validation, JWT authentication, and route dispatching.
- **Service Layer**: Orchestrates ingestion, chunk indexing, and structured documentation generation.
- **Persistence Engine**: SQLite database in WAL mode with FTS5 lexical retrieval index.
- **Worker Queue**: Background in-process queue processing asynchronous generation jobs.

## 3. Data Flow Overview
1. **Ingest**: Heterogeneous technical sources (code, markdown, spreadsheets, diagrams) are extracted, chunked, and indexed into FTS5.
2. **Synthesize**: Multi-stage generation builds the System Map, retrieves targeted source chunks per section, and compiles draft documents.
3. **Audit**: Automated pre-review checks enforce glossary terminology and verify code claims before human sign-off.
`, req.ProjectName, req.ProjectName)

	return &SystemMapResponse{
		Markdown:     md,
		ModelUsed:    p.modelName,
		InputTokens:  220,
		OutputTokens: 130,
		TokensUsed:   350,
	}, nil
}

func (p *MockProvider) GenerateSection(ctx context.Context, req SectionGenerationRequest) (*SectionGenerationResponse, error) {
	var citations []string
	for _, c := range req.Chunks {
		if c.FilePath != "" {
			citations = append(citations, fmt.Sprintf("- Grounded by `[%s#L%d-L%d]` (%s)", c.FilePath, c.LineStart, c.LineEnd, c.SymbolName))
		}
	}
	citationBlock := strings.Join(citations, "\n")
	if citationBlock == "" {
		citationBlock = "- Synthesized from architectural baseline."
	}

	md := fmt.Sprintf(`### %s

%s for %s based on retrieved ground truth technical chunks.

#### Evidence Grounding
%s

#### Specifications
- [x] Conforms to project architectural constraints.
- [x] Verified against active symbol definitions.
`, req.SectionTitle, req.SectionGoal, req.ProjectName, citationBlock)

	return &SectionGenerationResponse{
		Markdown:     md,
		ModelUsed:    p.modelName,
		InputTokens:  180,
		OutputTokens: 100,
		TokensUsed:   280,
	}, nil
}

func (p *MockProvider) SelfCorrect(ctx context.Context, req SelfCorrectionRequest) (*SelfCorrectionResponse, error) {
	corrected := req.CurrentMarkdown
	// Replace common avoided words from CONTEXT.md
	replacements := map[string]string{
		" page ":    " Document ",
		" pages ":   " Documents ",
		" draft ":   " Document ",
		" task ":    " Generation ",
		" job ":     " Generation ",
		" blocker ": " Blocking Feedback ",
		" snippet ": " Source Chunk ",
	}
	for oldWord, newWord := range replacements {
		corrected = strings.ReplaceAll(corrected, oldWord, newWord)
		corrected = strings.ReplaceAll(corrected, strings.Title(oldWord), newWord)
	}

	return &SelfCorrectionResponse{
		UpdatedMarkdown: corrected,
		ModelUsed:       p.modelName,
		InputTokens:     100,
		OutputTokens:    50,
		TokensUsed:      150,
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
