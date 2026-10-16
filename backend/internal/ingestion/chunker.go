package ingestion

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"docforge/internal/models"
)

type ChunkerOptions struct {
	MaxChunkChars int
	OverlapChars  int
}

func DefaultChunkerOptions() ChunkerOptions {
	return ChunkerOptions{
		MaxChunkChars: 1500,
		OverlapChars:  200,
	}
}

// ChunkSource transforms extracted source text or code into a slice of enriched models.SourceChunk
func ChunkSource(sourceID, projectID, filePath, originalFilename, mimeType, content string) []models.SourceChunk {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(filePath))
	}

	displayPath := originalFilename
	if displayPath == "" {
		displayPath = filePath
	}

	// 1. Code files (.go, .ts, .js, .py, .java, .json, .yaml, .yml)
	if isCodeExtension(ext) {
		return chunkCodeFile(sourceID, projectID, displayPath, content)
	}

	// 2. Markdown / Documentation (.md, .markdown)
	if ext == ".md" || ext == ".markdown" || mimeType == "text/markdown" {
		return chunkMarkdownFile(sourceID, projectID, displayPath, content)
	}

	// 3. Fallback to paragraph/sliding-window chunker
	return chunkTextContent(sourceID, projectID, displayPath, "text_block", content)
}

func chunkCodeFile(sourceID, projectID, filePath, content string) []models.SourceChunk {
	parsed := ParseCode(filePath, content)
	now := time.Now().UTC()
	var chunks []models.SourceChunk

	if parsed != nil && len(parsed.Symbols) > 0 {
		for i, sym := range parsed.Symbols {
			header := fmt.Sprintf("[Source: %s | Symbol: %s | Kind: %s | Lines: %d-%d]",
				filePath, sym.Name, sym.Kind, sym.LineStart, sym.LineEnd)

			body := sym.BodyContent
			if sym.Docstring != "" {
				body = fmt.Sprintf("Doc: %s\n%s", sym.Docstring, body)
			}

			chunks = append(chunks, models.SourceChunk{
				ID:           uuid.New().String(),
				SourceID:     sourceID,
				ProjectID:    projectID,
				ChunkIndex:   i,
				FilePath:     filePath,
				SymbolName:   sym.Name,
				ChunkType:    "code_symbol",
				LineStart:    sym.LineStart,
				LineEnd:      sym.LineEnd,
				SearchHeader: header,
				Content:      fmt.Sprintf("%s\n%s", header, body),
				CreatedAt:    now,
			})
		}
		return chunks
	}

	// If no symbols parsed (e.g. config file or small script), chunk by lines
	lines := strings.Split(content, "\n")
	chunkIdx := 0
	for i := 0; i < len(lines); i += 40 {
		end := i + 40
		if end > len(lines) {
			end = len(lines)
		}
		body := strings.Join(lines[i:end], "\n")
		header := fmt.Sprintf("[Source: %s | Lines: %d-%d]", filePath, i+1, end)

		chunks = append(chunks, models.SourceChunk{
			ID:           uuid.New().String(),
			SourceID:     sourceID,
			ProjectID:    projectID,
			ChunkIndex:   chunkIdx,
			FilePath:     filePath,
			ChunkType:    "code_symbol",
			LineStart:    i + 1,
			LineEnd:      end,
			SearchHeader: header,
			Content:      fmt.Sprintf("%s\n%s", header, body),
			CreatedAt:    now,
		})
		chunkIdx++
	}
	return chunks
}

var headingRegex = regexp.MustCompile(`(?m)^(#{1,4})\s+(.+)$`)

func chunkMarkdownFile(sourceID, projectID, filePath, content string) []models.SourceChunk {
	matches := headingRegex.FindAllStringSubmatchIndex(content, -1)
	now := time.Now().UTC()
	var chunks []models.SourceChunk

	if len(matches) == 0 {
		return chunkTextContent(sourceID, projectID, filePath, "heading", content)
	}

	chunkIdx := 0
	for i, match := range matches {
		start := match[0]
		var end int
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = len(content)
		}

		sectionText := strings.TrimSpace(content[start:end])
		if sectionText == "" {
			continue
		}

		headingTitle := content[match[4]:match[5]]
		header := fmt.Sprintf("[Source: %s | Section: %s]", filePath, headingTitle)

		chunks = append(chunks, models.SourceChunk{
			ID:           uuid.New().String(),
			SourceID:     sourceID,
			ProjectID:    projectID,
			ChunkIndex:   chunkIdx,
			FilePath:     filePath,
			SymbolName:   headingTitle,
			ChunkType:    "heading",
			SearchHeader: header,
			Content:      fmt.Sprintf("%s\n%s", header, sectionText),
			CreatedAt:    now,
		})
		chunkIdx++
	}

	return chunks
}

func chunkTextContent(sourceID, projectID, filePath, chunkType, content string) []models.SourceChunk {
	now := time.Now().UTC()
	var chunks []models.SourceChunk

	paragraphs := strings.Split(content, "\n\n")
	var currentChunk strings.Builder
	chunkIdx := 0
	startLine := 1

	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}

		if currentChunk.Len()+len(trimmed) > 1200 && currentChunk.Len() > 0 {
			header := fmt.Sprintf("[Source: %s | Chunk: %d]", filePath, chunkIdx+1)
			chunks = append(chunks, models.SourceChunk{
				ID:           uuid.New().String(),
				SourceID:     sourceID,
				ProjectID:    projectID,
				ChunkIndex:   chunkIdx,
				FilePath:     filePath,
				ChunkType:    chunkType,
				SearchHeader: header,
				Content:      fmt.Sprintf("%s\n%s", header, currentChunk.String()),
				CreatedAt:    now,
			})
			chunkIdx++
			currentChunk.Reset()
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(trimmed)
	}

	if currentChunk.Len() > 0 {
		header := fmt.Sprintf("[Source: %s | Chunk: %d]", filePath, chunkIdx+1)
		chunks = append(chunks, models.SourceChunk{
			ID:           uuid.New().String(),
			SourceID:     sourceID,
			ProjectID:    projectID,
			ChunkIndex:   chunkIdx,
			FilePath:     filePath,
			ChunkType:    chunkType,
			LineStart:    startLine,
			SearchHeader: header,
			Content:      fmt.Sprintf("%s\n%s", header, currentChunk.String()),
			CreatedAt:    now,
		})
	}

	return chunks
}

func isCodeExtension(ext string) bool {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".java", ".rs", ".cpp", ".c", ".h", ".cs", ".php", ".rb", ".json", ".yaml", ".yml", ".proto", ".sql":
		return true
	default:
		return false
	}
}
