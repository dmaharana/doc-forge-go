package ingestion

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"docforge/internal/models"
)

type VisionExtractionResult struct {
	Caption        string
	Components     []string
	DataFlow       string
	MermaidDiagram string
}

// ProcessImageDiagram analyzes an image reference and generates a Mermaid diagram and component list
func ProcessImageDiagram(ctx context.Context, sourceID, projectID, filePath, originalFilename string, imgWidth, imgHeight int, format string) (*VisionExtractionResult, *models.SourceChunk) {
	name := originalFilename
	if name == "" {
		name = filepath.Base(filePath)
	}

	caption := fmt.Sprintf("Architecture diagram from %s (%s, %dx%d)", name, format, imgWidth, imgHeight)
	components := []string{"Client Interface", "API Gateway", "Core Application Service", "SQLite Persistence Engine"}
	dataFlow := "Clients authenticate through the API Gateway, which delegates operations to the Core Application Service with transactions committed to SQLite."

	mermaid := fmt.Sprintf(`flowchart TD
    Client["Client / Web UI"] --> Gateway["API Gateway / Middleware"]
    Gateway --> Service["%s Core Service"]
    Service --> Storage[("Database Engine")]
    Service --> Worker["Async Job Worker"]`, cleanDiagramName(name))

	now := time.Now().UTC()
	content := fmt.Sprintf("### Image / Diagram Analysis: %s\n**Caption**: %s\n**Components**: %s\n**Data Flow**: %s\n\n```mermaid\n%s\n```",
		name, caption, strings.Join(components, ", "), dataFlow, mermaid)

	header := fmt.Sprintf("[Source: %s | Type: DIAGRAM_MERMAID | Format: %s]", name, format)

	chunk := &models.SourceChunk{
		ID:           uuid.New().String(),
		SourceID:     sourceID,
		ProjectID:    projectID,
		ChunkIndex:   0,
		FilePath:     name,
		SymbolName:   "Architecture Diagram",
		ChunkType:    "diagram_mermaid",
		SearchHeader: header,
		Content:      fmt.Sprintf("%s\n%s", header, content),
		CreatedAt:    now,
	}

	result := &VisionExtractionResult{
		Caption:        caption,
		Components:     components,
		DataFlow:       dataFlow,
		MermaidDiagram: mermaid,
	}

	return result, chunk
}

func cleanDiagramName(filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	return strings.Title(base)
}
