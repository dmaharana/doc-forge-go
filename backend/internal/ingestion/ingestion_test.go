package ingestion_test

import (
	"context"
	"testing"

	"docforge/internal/ingestion"
)

func TestParseGoAST(t *testing.T) {
	code := `package auth

import "net/http"

type UserProfile struct {
	ID    string
	Email string
}

type AuthRepository interface {
	FindUser(id string) (*UserProfile, error)
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Authentication handler
}

func (p *UserProfile) FullName() string {
	return p.Email
}
`
	res := ingestion.ParseCode("auth.go", code)
	if res == nil {
		t.Fatalf("expected parse result, got nil")
	}
	if res.Package != "auth" {
		t.Fatalf("expected package 'auth', got '%s'", res.Package)
	}

	foundStruct := false
	foundInterface := false
	foundEndpoint := false
	foundMethod := false

	for _, s := range res.Symbols {
		switch s.Kind {
		case "struct":
			if s.Name == "UserProfile" {
				foundStruct = true
			}
		case "interface":
			if s.Name == "AuthRepository" {
				foundInterface = true
			}
		case "endpoint":
			if s.Name == "HandleLogin" {
				foundEndpoint = true
			}
		case "method":
			if s.Name == "FullName" {
				foundMethod = true
			}
		}
	}

	if !foundStruct {
		t.Errorf("UserProfile struct not found")
	}
	if !foundInterface {
		t.Errorf("AuthRepository interface not found")
	}
	if !foundEndpoint {
		t.Errorf("HandleLogin endpoint not found")
	}
	if !foundMethod {
		t.Errorf("FullName method not found")
	}
}

func TestChunkMarkdownFile(t *testing.T) {
	doc := `# System Overview
This is the primary system architecture overview.

## Data Pipeline
Data enters through the ingest API and is saved into SQLite.

## Operational Diagnostics
Health checks run at /health.
`
	chunks := ingestion.ChunkSource("src-1", "prj-1", "docs/ARCH.md", "ARCH.md", "text/markdown", doc)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 heading chunks, got %d", len(chunks))
	}
	if chunks[0].SymbolName != "System Overview" {
		t.Errorf("expected first heading 'System Overview', got '%s'", chunks[0].SymbolName)
	}
	if chunks[1].SymbolName != "Data Pipeline" {
		t.Errorf("expected second heading 'Data Pipeline', got '%s'", chunks[1].SymbolName)
	}
}

func TestProcessImageDiagram(t *testing.T) {
	res, chunk := ingestion.ProcessImageDiagram(context.Background(), "s1", "p1", "uploads/arch.png", "arch.png", 1920, 1080, "png")
	if res == nil || chunk == nil {
		t.Fatalf("expected non-nil vision result and chunk")
	}
	if chunk.ChunkType != "diagram_mermaid" {
		t.Errorf("expected chunk type 'diagram_mermaid', got '%s'", chunk.ChunkType)
	}
	if len(res.Components) == 0 {
		t.Errorf("expected extracted components list")
	}
}
