package service

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

func TestMarkdownToConfluenceStorage(t *testing.T) {
	mdInput := `# Architecture Overview

This is an introductory paragraph with **bold** text and ` + "`inline_code()`" + `.

> [!NOTE]
> Ensure all services are deployed with TLS enabled.

> [!WARNING]
> Do not expose the SQLite port to external networks.

## Data Model

| Field | Type | Description |
| --- | --- | --- |
| id | string | Unique document identifier |
| status | string | Review state |

## Components Flow

` + "```mermaid" + `
flowchart TD
  Client[Web Client] --> Gateway[API Gateway]
  Gateway --> Service[Document Service]
  Service --> DB[(SQLite FTS5)]
` + "```" + `

## Implementation Details

` + "```go" + `
package main

import "fmt"

func main() {
	fmt.Println("DocForge Confluence Export")
}
` + "```" + `

---
End of Document.`

	result, err := MarkdownToConfluenceStorage(mdInput)
	if err != nil {
		t.Fatalf("MarkdownToConfluenceStorage returned unexpected error: %v", err)
	}

	// 1. Verify it produces well-formed XML
	wrapper := fmt.Sprintf("<xml xmlns:ac=\"http://www.atlassian.com/schema/confluence/4/ac\">%s</xml>", result)
	var dummy struct{}
	if err := xml.Unmarshal([]byte(wrapper), &dummy); err != nil {
		t.Fatalf("Generated Confluence storage format is not valid XML: %v\nResult was:\n%s", err, result)
	}

	// 2. Verify headings and paragraphs
	if !strings.Contains(result, "<h1>Architecture Overview</h1>") && !strings.Contains(result, "<h1") {
		t.Errorf("expected <h1> heading in result, got:\n%s", result)
	}
	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Errorf("expected <strong>bold</strong> in result, got:\n%s", result)
	}

	// 3. Verify Callouts
	if !strings.Contains(result, `<ac:structured-macro ac:name="note">`) {
		t.Errorf("expected <ac:structured-macro ac:name=\"note\"> for NOTE callout, got:\n%s", result)
	}
	if !strings.Contains(result, `<ac:structured-macro ac:name="warning">`) {
		t.Errorf("expected <ac:structured-macro ac:name=\"warning\"> for WARNING callout, got:\n%s", result)
	}

	// 4. Verify Table with wrapped relative-table class
	if !strings.Contains(result, `<table class="wrapped relative-table">`) {
		t.Errorf("expected <table class=\"wrapped relative-table\">, got:\n%s", result)
	}
	if !strings.Contains(result, "<th>Field</th>") || !strings.Contains(result, "<td>id</td>") {
		t.Errorf("expected table cells in result, got:\n%s", result)
	}

	// 5. Verify Mermaid diagram formatted as Confluence code macro
	if !strings.Contains(result, `<ac:parameter ac:name="language">mermaid</ac:parameter>`) {
		t.Errorf("expected mermaid language parameter, got:\n%s", result)
	}
	if !strings.Contains(result, `<ac:parameter ac:name="title">Architectural Diagram</ac:parameter>`) {
		t.Errorf("expected Architectural Diagram title parameter, got:\n%s", result)
	}
	if !strings.Contains(result, "flowchart TD") {
		t.Errorf("expected flowchart content inside CDATA, got:\n%s", result)
	}

	// 6. Verify Go code block
	if !strings.Contains(result, `<ac:parameter ac:name="language">go</ac:parameter>`) {
		t.Errorf("expected go language parameter, got:\n%s", result)
	}
	if !strings.Contains(result, `fmt.Println("DocForge Confluence Export")`) {
		t.Errorf("expected go code content inside CDATA, got:\n%s", result)
	}

	// 7. Verify horizontal rule self-closing
	if !strings.Contains(result, "<hr/>") {
		t.Errorf("expected self-closing <hr/> tag, got:\n%s", result)
	}
}

func TestMarkdownToConfluenceStorage_CDATAEscape(t *testing.T) {
	mdInput := "```xml\n<foo>]]>bar</foo>\n```"
	result, err := MarkdownToConfluenceStorage(mdInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it remains valid XML even with nested CDATA closing sequence
	wrapper := fmt.Sprintf("<xml xmlns:ac=\"http://www.atlassian.com/schema/confluence/4/ac\">%s</xml>", result)
	var dummy struct{}
	if err := xml.Unmarshal([]byte(wrapper), &dummy); err != nil {
		t.Fatalf("CDATA escaping failed to produce valid XML: %v\nResult:\n%s", err, result)
	}
}

func TestMarkdownToConfluenceStorage_EmptyInput(t *testing.T) {
	result, err := MarkdownToConfluenceStorage("   ")
	if err != nil {
		t.Fatalf("unexpected error on empty input: %v", err)
	}
	if result != "<p></p>" {
		t.Errorf("expected '<p></p>', got: %q", result)
	}
}
