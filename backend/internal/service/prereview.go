package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"docforge/internal/llm"
	"docforge/internal/models"
	"docforge/internal/repository"
)

type PreReviewFinding struct {
	RuleName      string // link_validation, glossary_consistency, code_spot_check, ai_artifact, missing_info
	Severity      string // blocking, warning, info
	Message       string
	SectionAnchor string
	IsBlocking    bool
}

type PreReviewReport struct {
	TotalChecks   int                `json:"total_checks"`
	Passed        bool               `json:"passed"`
	Findings      []PreReviewFinding `json:"findings"`
	SelfCorrected bool               `json:"self_corrected"`
	InferredCount int                `json:"inferred_count"`
	MissingCount  int                `json:"missing_count"`
	ModelUsed     string             `json:"model_used,omitempty"`
	InputTokens   int                `json:"input_tokens,omitempty"`
	OutputTokens  int                `json:"output_tokens,omitempty"`
	TokensUsed    int                `json:"tokens_used,omitempty"`
}

type PreReviewEngine struct {
	repo *repository.Repository
}

func NewPreReviewEngine(repo *repository.Repository) *PreReviewEngine {
	return &PreReviewEngine{repo: repo}
}

var linkRegex = regexp.MustCompile(`\[([^\]]+)\]\(#([^\)]+)\)`)
var headingRegex = regexp.MustCompile(`(?m)^#{1,4}\s+(.+)$`)
var endpointRegex = regexp.MustCompile(`(?m)(?:POST|GET|PUT|DELETE|PATCH)\s+/[A-Za-z0-9_\-/{}.]+`)
var aiArtifactRegex = regexp.MustCompile(`(?i)(?:as an ai language model|in summary, this document has outlined|as mentioned previously|i hope this helps)`)

// RunPreReview performs all automated checks, triggers self-correction for formatting/glossary, and creates blocking feedback for factual discrepancies
func (e *PreReviewEngine) RunPreReview(ctx context.Context, doc *models.Document, provider llm.Provider, projectID string, llmCfg *models.LLMConfig) (*PreReviewReport, string, error) {
	currentMD := doc.ContentMD
	report := &PreReviewReport{
		Findings: make([]PreReviewFinding, 0),
	}

	// 1. Initial Inspection
	e.inspectDraft(ctx, currentMD, projectID, report)

	// 2. Identify issues that can be automatically self-corrected
	var fixableIssues []string
	for _, f := range report.Findings {
		if f.RuleName == "glossary_consistency" || f.RuleName == "ai_artifact" || f.RuleName == "link_validation" {
			fixableIssues = append(fixableIssues, fmt.Sprintf("[%s] %s", f.RuleName, f.Message))
		}
	}

	// 3. Self-correction loop if fixable issues exist
	if len(fixableIssues) > 0 && provider != nil {
		req := llm.SelfCorrectionRequest{
			CurrentMarkdown: currentMD,
			Issues:          fixableIssues,
			ProjectName:     doc.Title,
		}
		if llmCfg != nil {
			req.Model = llmCfg.DefaultModel
			req.Temperature = llmCfg.Temperature
			req.MaxTokens = llmCfg.MaxTokens
		}

		resp, err := provider.SelfCorrect(ctx, req)
		if err == nil && resp != nil && resp.UpdatedMarkdown != "" {
			currentMD = resp.UpdatedMarkdown
			report.SelfCorrected = true
			report.ModelUsed = resp.ModelUsed
			report.InputTokens = resp.InputTokens
			report.OutputTokens = resp.OutputTokens
			report.TokensUsed = resp.TokensUsed

			// Re-inspect to clear resolved issues
			report.Findings = make([]PreReviewFinding, 0)
			e.inspectDraft(ctx, currentMD, projectID, report)
		}
	}

	// 4. Any residual code mismatches or missing information items create hard-gated Blocking Feedback
	var authorUserID string
	if project, pErr := e.repo.GetProjectByID(projectID); pErr == nil && project != nil {
		authorUserID = project.OwnerID
	}

	for _, f := range report.Findings {
		if f.IsBlocking {
			category := models.FeedbackMissingInfo
			if f.RuleName == "code_spot_check" {
				category = models.FeedbackCorrection
			}

			fb := &models.DocumentFeedback{
				ID:            uuid.New().String(),
				DocumentID:    doc.ID,
				UserID:        authorUserID,
				AuthorName:    "System: Pre-Review Gate",
				AuthorEmail:   "pre-review@docforge.local",
				SectionAnchor: f.SectionAnchor,
				Category:      category,
				Content:       fmt.Sprintf("[Pre-Review Gate] %s", f.Message),
				IsBlocking:    true,
				Status:        models.FeedbackOpen,
			}
			_ = e.repo.CreateFeedback(fb)
		}
	}

	report.Passed = len(report.Findings) == 0
	return report, currentMD, nil
}

func (e *PreReviewEngine) inspectDraft(ctx context.Context, markdown, projectID string, report *PreReviewReport) {
	// A. Link and Anchor validation
	headings := headingRegex.FindAllStringSubmatch(markdown, -1)
	headingSlugs := make(map[string]bool)
	for _, h := range headings {
		slug := slugifyHeading(h[1])
		headingSlugs[slug] = true
	}

	links := linkRegex.FindAllStringSubmatch(markdown, -1)
	for _, l := range links {
		anchor := strings.TrimPrefix(l[2], "#")
		report.TotalChecks++
		if !headingSlugs[anchor] {
			report.Findings = append(report.Findings, PreReviewFinding{
				RuleName:      "link_validation",
				Severity:      "warning",
				Message:       fmt.Sprintf("Broken anchor reference '#%s' does not match any heading in the draft.", anchor),
				SectionAnchor: l[1],
				IsBlocking:    false,
			})
		}
	}

	// B. Glossary consistency (against CONTEXT.md definitions)
	report.TotalChecks++
	glossaryConflicts := findGlossaryConflicts(markdown)
	for _, conflict := range glossaryConflicts {
		report.Findings = append(report.Findings, PreReviewFinding{
			RuleName:   "glossary_consistency",
			Severity:   "warning",
			Message:    conflict,
			IsBlocking: false,
		})
	}

	// C. AI Artifact & Hedging Scanner
	report.TotalChecks++
	if aiArtifactRegex.MatchString(markdown) {
		report.Findings = append(report.Findings, PreReviewFinding{
			RuleName:   "ai_artifact",
			Severity:   "warning",
			Message:    "Detected repetitive AI conversational boilerplate/hedging phrasing in the generated draft.",
			IsBlocking: false,
		})
	}

	// D. Factual Code Spot-Check
	// Extract asserted HTTP routes e.g. POST /api/v1/auth/login
	endpoints := endpointRegex.FindAllString(markdown, -1)
	if len(endpoints) > 0 {
		symbols, _ := e.repo.ListProjectCodeSymbols(projectID)
		symbolMap := make(map[string]bool)
		for _, s := range symbols {
			symbolMap[strings.ToLower(s)] = true
		}

		for _, ep := range endpoints {
			report.TotalChecks++
			cleanEP := strings.ToLower(strings.TrimSpace(ep))
			// Check if known in symbol map or searchable in FTS5
			if !symbolMap[cleanEP] {
				hits, _ := e.repo.SearchSourceChunks(projectID, ep, []string{"code_symbol"}, 1)
				if len(hits) == 0 {
					report.Findings = append(report.Findings, PreReviewFinding{
						RuleName:      "code_spot_check",
						Severity:      "blocking",
						Message:       fmt.Sprintf("Asserted endpoint '%s' has 0 matching symbols or routes in ingested source code chunks.", ep),
						SectionAnchor: "API & Integration Endpoints",
						IsBlocking:    true,
					})
				}
			}
		}
	}

	// E. Inferred & Missing Tag Auditing
	report.InferredCount = strings.Count(markdown, "[INFERRED]")
	report.MissingCount = strings.Count(markdown, "[MISSING]")

	if report.MissingCount > 0 {
		report.TotalChecks++
		report.Findings = append(report.Findings, PreReviewFinding{
			RuleName:      "missing_info",
			Severity:      "blocking",
			Message:       fmt.Sprintf("Draft contains %d explicit [MISSING] tags denoting technical gaps that require human review and source verification.", report.MissingCount),
			SectionAnchor: "Ground Truth Gaps",
			IsBlocking:    true,
		})
	}
}

func slugifyHeading(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.ReplaceAll(h, " ", "-")
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, h)
}

func findGlossaryConflicts(text string) []string {
	var conflicts []string
	lower := " " + strings.ToLower(text) + " "

	avoidedTerms := map[string]string{
		" page ":        "Document",
		" pages ":       "Documents",
		" task ":        "Generation",
		" job ":         "Generation",
		" snippet ":     "Source Chunk",
		" blocker ":     "Blocking Feedback",
		" workspace ":   "Project",
		" login token ": "Atlassian Session",
	}

	for avoided, canonical := range avoidedTerms {
		if strings.Contains(lower, avoided) {
			conflicts = append(conflicts, fmt.Sprintf("Draft uses avoided term '%s' instead of canonical domain term '%s'.", strings.TrimSpace(avoided), canonical))
		}
	}
	return conflicts
}
