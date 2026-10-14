package ingestion

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

type ExtractionResult struct {
	Text         string
	MetadataJSON string
	MimeType     string
}

// ExtractFromFile inspects the file extension / content and parses plain text, pdf, xlsx, zip, or image
func ExtractFromFile(path, originalFilename string) (*ExtractionResult, error) {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(path))
	}

	switch ext {
	case ".pdf":
		return extractPDF(path)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return extractExcel(path)
	case ".zip":
		return extractZip(path)
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return extractImage(path)
	default:
		// Text / code / markdown
		return extractText(path)
	}
}

func extractText(path string) (*ExtractionResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	text := string(data)
	meta := map[string]any{
		"character_count": len(text),
		"line_count":      strings.Count(text, "\n") + 1,
	}
	metaJSON, _ := json.Marshal(meta)

	return &ExtractionResult{
		Text:         text,
		MetadataJSON: string(metaJSON),
		MimeType:     "text/plain",
	}, nil
}

func extractPDF(path string) (*ExtractionResult, error) {
	content, err := readPdf(path)
	if err != nil {
		return nil, fmt.Errorf("failed to extract pdf: %w", err)
	}

	meta := map[string]any{
		"parser":          "pure-go-pdf",
		"character_count": len(content),
	}
	metaJSON, _ := json.Marshal(meta)

	return &ExtractionResult{
		Text:         content,
		MetadataJSON: string(metaJSON),
		MimeType:     "application/pdf",
	}, nil
}

func readPdf(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}

	_, err = buf.ReadFrom(b)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func extractExcel(path string) (*ExtractionResult, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	sheets := f.GetSheetList()
	sheetStats := make(map[string]int)

	for _, sheet := range sheets {
		sb.WriteString(fmt.Sprintf("\n### Sheet: %s\n\n", sheet))
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		sheetStats[sheet] = len(rows)

		for rowIdx, row := range rows {
			if rowIdx == 0 {
				// Table header
				sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
				sb.WriteString("|" + strings.Repeat(" --- |", len(row)) + "\n")
			} else {
				// Table data row
				sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
			}
		}
		sb.WriteString("\n")
	}

	meta := map[string]any{
		"sheets": sheets,
		"stats":  sheetStats,
	}
	metaJSON, _ := json.Marshal(meta)

	return &ExtractionResult{
		Text:         sb.String(),
		MetadataJSON: string(metaJSON),
		MimeType:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}, nil
}

func extractZip(path string) (*ExtractionResult, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	var sb strings.Builder
	var fileList []string
	totalFiles := 0

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Skip known binary extensions
		ext := strings.ToLower(filepath.Ext(f.Name))
		if isBinaryExt(ext) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 1024*512)) // limit 512KB per file
		rc.Close()
		if err != nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n--- File: %s ---\n```\n", f.Name))
		sb.WriteString(string(data))
		sb.WriteString("\n```\n")

		fileList = append(fileList, f.Name)
		totalFiles++
		if totalFiles > 100 {
			sb.WriteString("\n...[archive file limit reached]\n")
			break
		}
	}

	meta := map[string]any{
		"total_text_files": totalFiles,
		"files":            fileList,
	}
	metaJSON, _ := json.Marshal(meta)

	return &ExtractionResult{
		Text:         sb.String(),
		MetadataJSON: string(metaJSON),
		MimeType:     "application/zip",
	}, nil
}

func isBinaryExt(ext string) bool {
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".iso", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".mp4", ".mov", ".zip", ".tar", ".gz":
		return true
	default:
		return false
	}
}

func extractImage(path string) (*ExtractionResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg, format, err := image.DecodeConfig(f)
	var width, height int
	if err == nil {
		width = cfg.Width
		height = cfg.Height
	}

	// Read image bytes for base64
	_, _ = f.Seek(0, 0)
	imgBytes, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	b64Data := base64.StdEncoding.EncodeToString(imgBytes)

	meta := map[string]any{
		"width":      width,
		"height":     height,
		"format":     format,
		"has_base64": true,
	}
	metaJSON, _ := json.Marshal(meta)

	description := fmt.Sprintf("[Image Reference: format=%s, dimensions=%dx%d, size_bytes=%d]\nBase64 Payload prefix: data:image/%s;base64,%s...",
		format, width, height, len(imgBytes), format, safePrefix(b64Data, 64))

	return &ExtractionResult{
		Text:         description,
		MetadataJSON: string(metaJSON),
		MimeType:     fmt.Sprintf("image/%s", format),
	}, nil
}

func safePrefix(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

// ----------------- REMOTE SOURCE INGESTION -----------------

func FetchRemoteSource(ctx context.Context, remoteURL, remoteType string) (*ExtractionResult, error) {
	switch strings.ToLower(remoteType) {
	case "git":
		return fetchGitRepo(ctx, remoteURL)
	case "local":
		return extractText(remoteURL)
	case "http", "https", "web":
		fallthrough
	default:
		return fetchHTTP(ctx, remoteURL)
	}
}

func fetchHTTP(ctx context.Context, url string) (*ExtractionResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "DocForge-Ingestion-Bot/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote url: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024*2)) // 2MB max
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	text := string(data)

	// Clean basic HTML tags if it's an HTML page
	if strings.Contains(contentType, "html") {
		text = stripHTML(text)
	}

	meta := map[string]any{
		"status_code":  resp.StatusCode,
		"content_type": contentType,
		"url":          url,
	}
	metaJSON, _ := json.Marshal(meta)

	return &ExtractionResult{
		Text:         text,
		MetadataJSON: string(metaJSON),
		MimeType:     contentType,
	}, nil
}

func fetchGitRepo(ctx context.Context, repoURL string) (*ExtractionResult, error) {
	// Clone shallowly into temp directory
	tmpDir, err := os.MkdirTemp("", "docforge-git-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, tmpDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone failed for %s: %w", repoURL, err)
	}

	var sb strings.Builder
	var filesRead []string

	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(path, "/.git/") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if isBinaryExt(ext) {
			return nil
		}

		rel, _ := filepath.Rel(tmpDir, path)
		data, err := os.ReadFile(path)
		if err == nil && len(data) < 100*1024 { // limit 100KB per file
			sb.WriteString(fmt.Sprintf("\n### File: %s\n```\n%s\n```\n", rel, string(data)))
			filesRead = append(filesRead, rel)
		}
		if len(filesRead) > 50 {
			return io.EOF // stop walking
		}
		return nil
	})

	meta := map[string]any{
		"repo_url":   repoURL,
		"files_read": filesRead,
	}
	metaJSON, _ := json.Marshal(meta)

	return &ExtractionResult{
		Text:         sb.String(),
		MetadataJSON: string(metaJSON),
		MimeType:     "application/x-git",
	}, nil
}

var tagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTML(html string) string {
	clean := tagRegex.ReplaceAllString(html, " ")
	lines := strings.Split(clean, "\n")
	var trimmed []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" {
			trimmed = append(trimmed, t)
		}
	}
	return strings.Join(trimmed, "\n")
}
