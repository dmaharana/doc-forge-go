package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"docforge/internal/repository"
)

type CompletenessScorecard struct {
	CoverageScore   int      `json:"coverage_score"` // 0 to 100
	HasEntrypoint   bool     `json:"has_entrypoint"`
	HasConfig       bool     `json:"has_config"`
	HasAPIEndpoints bool     `json:"has_api_endpoints"`
	HasDataModels   bool     `json:"has_data_models"`
	HasTests        bool     `json:"has_tests"`
	TotalChunks     int      `json:"total_chunks"`
	TotalSymbols    int      `json:"total_symbols"`
	MissingAreas    []string `json:"missing_areas"`
}

type CompletenessService struct {
	repo *repository.Repository
}

func NewCompletenessService(repo *repository.Repository) *CompletenessService {
	return &CompletenessService{repo: repo}
}

func (s *CompletenessService) EvaluateProject(ctx context.Context, projectID string) (*CompletenessScorecard, error) {
	chunks, err := s.repo.ListSourceChunksByProject(projectID)
	if err != nil {
		return nil, err
	}

	scorecard := &CompletenessScorecard{
		TotalChunks:  len(chunks),
		MissingAreas: make([]string, 0),
	}

	symbolSet := make(map[string]struct{})

	for _, c := range chunks {
		if c.SymbolName != "" {
			symbolSet[c.SymbolName] = struct{}{}
		}

		base := strings.ToLower(filepath.Base(c.FilePath))
		contentLower := strings.ToLower(c.Content)

		// Entrypoints
		if base == "main.go" || base == "index.ts" || base == "app.py" || base == "server.go" || base == "main.py" || strings.Contains(contentLower, "func main(") {
			scorecard.HasEntrypoint = true
		}

		// Configs
		if strings.Contains(base, ".env") || strings.Contains(base, "config") || strings.Contains(base, "settings") || strings.Contains(base, "docker-compose") {
			scorecard.HasConfig = true
		}

		// API routes & endpoints
		if c.ChunkType == "code_symbol" && (strings.Contains(c.SearchHeader, "endpoint") || strings.Contains(c.SearchHeader, "spec_route") || strings.Contains(c.SearchHeader, "HTTP_HANDLER")) {
			scorecard.HasAPIEndpoints = true
		}

		// Data models
		if c.ChunkType == "code_symbol" && (strings.Contains(c.SearchHeader, "struct") || strings.Contains(c.SearchHeader, "interface") || strings.Contains(c.SearchHeader, "class") || strings.Contains(c.SearchHeader, "type")) {
			scorecard.HasDataModels = true
		}

		// Tests
		if strings.Contains(base, "_test.go") || strings.Contains(base, ".spec.") || strings.Contains(base, ".test.") || strings.HasPrefix(base, "test_") {
			scorecard.HasTests = true
		}
	}

	scorecard.TotalSymbols = len(symbolSet)

	// Calculate score
	points := 0
	if scorecard.HasEntrypoint {
		points += 20
	} else {
		scorecard.MissingAreas = append(scorecard.MissingAreas, "No application entrypoint (main.go/index.ts/app.py) detected")
	}

	if scorecard.HasConfig {
		points += 20
	} else {
		scorecard.MissingAreas = append(scorecard.MissingAreas, "No configuration or environment definitions (.env/config.yaml) detected")
	}

	if scorecard.HasAPIEndpoints {
		points += 25
	} else {
		scorecard.MissingAreas = append(scorecard.MissingAreas, "No API routes or endpoint definitions detected")
	}

	if scorecard.HasDataModels {
		points += 20
	} else {
		scorecard.MissingAreas = append(scorecard.MissingAreas, "No domain models, structs, or database schemas detected")
	}

	if scorecard.HasTests {
		points += 15
	} else {
		scorecard.MissingAreas = append(scorecard.MissingAreas, "No automated test suites or specification files detected")
	}

	scorecard.CoverageScore = points
	return scorecard, nil
}

func (s *CompletenessService) ToJSON(sc *CompletenessScorecard) string {
	b, _ := json.Marshal(sc)
	return string(b)
}
