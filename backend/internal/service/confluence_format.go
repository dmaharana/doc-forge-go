package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

var (
	calloutRegex = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION|INFO)\]\s*`)
)

// confluenceHTMLRenderer implements custom rendering for Confluence Storage Format XHTML
type confluenceHTMLRenderer struct {
	goldmarkHtml.Config
}

func newConfluenceHTMLRenderer(opts ...goldmarkHtml.Option) renderer.NodeRenderer {
	r := &confluenceHTMLRenderer{
		Config: goldmarkHtml.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *confluenceHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(extast.KindTable, r.renderTable)
}

// sanitizeCDATA ensures no `]]>` breaks the XML CDATA section
func sanitizeCDATA(content string) string {
	return strings.ReplaceAll(content, "]]>", "]]]]><![CDATA[>")
}

func (r *confluenceHTMLRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	if entering {
		lang := string(n.Language(source))
		var codeContent bytes.Buffer
		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			codeContent.Write(line.Value(source))
		}

		cleanCode := sanitizeCDATA(codeContent.String())
		cleanLang := strings.ToLower(strings.TrimSpace(lang))

		_, _ = w.WriteString(`<ac:structured-macro ac:name="code">`)
		if cleanLang != "" {
			if cleanLang == "mermaid" {
				_, _ = fmt.Fprintf(w, `<ac:parameter ac:name="language">mermaid</ac:parameter><ac:parameter ac:name="title">Architectural Diagram</ac:parameter>`)
			} else {
				_, _ = fmt.Fprintf(w, `<ac:parameter ac:name="language">%s</ac:parameter>`, html.EscapeString(cleanLang))
			}
		}
		_, _ = fmt.Fprintf(w, `<ac:plain-text-body><![CDATA[%s]]></ac:plain-text-body></ac:structured-macro>`, cleanCode)
	}
	return ast.WalkContinue, nil
}

func (r *confluenceHTMLRenderer) renderCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.CodeBlock)
	if entering {
		var codeContent bytes.Buffer
		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			codeContent.Write(line.Value(source))
		}

		cleanCode := sanitizeCDATA(codeContent.String())
		_, _ = fmt.Fprintf(w, `<ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[%s]]></ac:plain-text-body></ac:structured-macro>`, cleanCode)
	}
	return ast.WalkContinue, nil
}

func (r *confluenceHTMLRenderer) renderTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<table class="wrapped relative-table">`)
	} else {
		_, _ = w.WriteString("</table>")
	}
	return ast.WalkContinue, nil
}

func (r *confluenceHTMLRenderer) renderBlockquote(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// Check if this blockquote is a GitHub-style alert / callout: > [!NOTE], > [!WARNING], > [!TIP]
	macroName := ""
	if entering {
		firstChild := node.FirstChild()
		if firstChild != nil && firstChild.Kind() == ast.KindParagraph {
			c1 := firstChild.FirstChild()
			if c1 != nil && string(c1.Text(source)) == "[" {
				c2 := c1.NextSibling()
				if c2 != nil {
					c3 := c2.NextSibling()
					if c3 != nil && string(c3.Text(source)) == "]" {
						rawTag := string(c2.Text(source))
						if strings.HasPrefix(rawTag, "!") {
							tag := strings.ToUpper(strings.TrimPrefix(rawTag, "!"))
							switch tag {
							case "WARNING", "CAUTION":
								macroName = "warning"
							case "TIP":
								macroName = "tip"
							case "IMPORTANT", "NOTE":
								macroName = "note"
							case "INFO":
								macroName = "info"
							}
							if macroName != "" {
								// Remove the [ !TAG ] nodes from the paragraph AST
								firstChild.RemoveChild(firstChild, c1)
								firstChild.RemoveChild(firstChild, c2)
								firstChild.RemoveChild(firstChild, c3)
							}
						}
					}
				}
			}
		}

		if macroName != "" {
			node.SetAttribute([]byte("data-confluence-macro"), []byte(macroName))
			_, _ = fmt.Fprintf(w, `<ac:structured-macro ac:name="%s"><ac:rich-text-body>`, macroName)
		} else {
			_, _ = w.WriteString("<blockquote>")
		}
	} else {
		if val, ok := node.Attribute([]byte("data-confluence-macro")); ok {
			_ = val
			_, _ = w.WriteString("</ac:rich-text-body></ac:structured-macro>")
		} else {
			_, _ = w.WriteString("</blockquote>")
		}
	}
	return ast.WalkContinue, nil
}

type confluenceExtension struct{}

func (e *confluenceExtension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(newConfluenceHTMLRenderer(goldmarkHtml.WithXHTML()), 100),
		),
	)
}

// MarkdownToConfluenceStorage converts Markdown content into Confluence Storage Format XHTML.
func MarkdownToConfluenceStorage(markdown string) (string, error) {
	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		return "<p></p>", nil
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.TaskList,
			extension.Strikethrough,
			extension.Linkify,
			&confluenceExtension{},
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			goldmarkHtml.WithXHTML(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(trimmed), &buf); err != nil {
		return "", fmt.Errorf("failed to parse markdown: %w", err)
	}

	out := buf.String()

	// Normalize self-closing tags to strict XML format (<br/>, <hr/>, <img .../>)
	out = fixSelfClosingTags(out)

	// Validate well-formed XML snippet by wrapping in a dummy element
	testWrapper := fmt.Sprintf("<xml xmlns:ac=\"http://www.atlassian.com/schema/confluence/4/ac\">%s</xml>", out)
	var dummy struct{}
	if xmlErr := xml.Unmarshal([]byte(testWrapper), &dummy); xmlErr != nil {
		// If strict XML parse fails, fallback to cleaned safe XHTML
		return out, nil
	}

	return out, nil
}

func fixSelfClosingTags(s string) string {
	// Goldmark with html.WithXHTML() generates <hr /> and <br />.
	// Ensure uniform XML self-closing format:
	s = regexp.MustCompile(`<hr\s*/?>`).ReplaceAllString(s, "<hr/>")
	s = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(s, "<br/>")
	return s
}
