package templates

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
	"time"

	"docforge/internal/models"
)

//go:embed *.tmpl
var templateFS embed.FS

type TemplateData struct {
	Title       string
	ProjectName string
	Date        string
	Summary     string
	Sources     []models.Source
}

func RenderTemplate(templateType string, data TemplateData) (string, error) {
	if data.Date == "" {
		data.Date = time.Now().Format("2006-01-02")
	}

	var filename string
	switch strings.ToLower(templateType) {
	case "product_spec", "product / feature specification":
		filename = "product_spec.md.tmpl"
	case "adr", "architecture decision record (adr)", "architecture decision record":
		filename = "adr.md.tmpl"
	case "release_notes", "release notes":
		filename = "release_notes.md.tmpl"
	case "runbook":
		filename = "runbook.md.tmpl"
	default:
		return "", fmt.Errorf("unknown template type: %s", templateType)
	}

	tmplBytes, err := templateFS.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded template %s: %w", filename, err)
	}

	tmpl, err := template.New(filename).Parse(string(tmplBytes))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func ListSupportedTemplates() []string {
	return []string{
		"product_spec",
		"adr",
		"release_notes",
		"runbook",
	}
}

func GetTemplateTitle(templateType, projectName string) string {
	switch strings.ToLower(templateType) {
	case "product_spec", "product / feature specification":
		return fmt.Sprintf("%s - Product & Feature Specification", projectName)
	case "adr", "architecture decision record (adr)", "architecture decision record":
		return fmt.Sprintf("%s - Architecture Decision Record", projectName)
	case "release_notes", "release notes":
		return fmt.Sprintf("%s - Release Notes", projectName)
	case "runbook":
		return fmt.Sprintf("%s - Operations Runbook", projectName)
	default:
		return fmt.Sprintf("%s - Technical Documentation", projectName)
	}
}
