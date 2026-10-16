package ingestion

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

type ParsedSymbol struct {
	Name        string
	Kind        string // struct, interface, function, method, endpoint, spec_route
	LineStart   int
	LineEnd     int
	Signature   string
	Docstring   string
	BodyContent string
}

type CodeParseResult struct {
	Language string
	Package  string
	Symbols  []ParsedSymbol
}

// ParseCode inspects a code or spec file and extracts symbols
func ParseCode(filePath, content string) *CodeParseResult {
	ext := strings.ToLower(filepath.Ext(filePath))
	filename := strings.ToLower(filepath.Base(filePath))

	// 1. OpenAPI / Swagger check
	if ext == ".json" || ext == ".yaml" || ext == ".yml" {
		if strings.Contains(filename, "swagger") || strings.Contains(filename, "openapi") ||
			strings.Contains(content, `"openapi"`) || strings.Contains(content, "openapi:") ||
			strings.Contains(content, `"swagger"`) || strings.Contains(content, "swagger:") {
			if res, ok := parseOpenAPISpec(filePath, content); ok {
				return res
			}
		}
	}

	// 2. Go AST parsing
	if ext == ".go" {
		return parseGoAST(filePath, content)
	}

	// 3. Fallback to generic symbol regex scanner (TypeScript, JavaScript, Python, Java, Rust)
	return parseGenericCode(filePath, content, ext)
}

func parseGoAST(filePath, content string) *CodeParseResult {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		// Fallback to generic regex scanner if AST parse fails (e.g. template or snippet)
		return parseGenericCode(filePath, content, ".go")
	}

	lines := strings.Split(content, "\n")
	res := &CodeParseResult{
		Language: "go",
		Package:  node.Name.Name,
		Symbols:  make([]ParsedSymbol, 0),
	}

	// Walk AST
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			start := fset.Position(x.Pos()).Line
			end := fset.Position(x.End()).Line
			kind := "type"
			switch x.Type.(type) {
			case *ast.StructType:
				kind = "struct"
			case *ast.InterfaceType:
				kind = "interface"
			}
			res.Symbols = append(res.Symbols, ParsedSymbol{
				Name:        x.Name.Name,
				Kind:        kind,
				LineStart:   start,
				LineEnd:     end,
				Signature:   fmt.Sprintf("type %s %s", x.Name.Name, kind),
				Docstring:   extractDoc(x.Doc),
				BodyContent: sliceLines(lines, start, end),
			})

		case *ast.FuncDecl:
			start := fset.Position(x.Pos()).Line
			end := fset.Position(x.End()).Line
			kind := "function"
			name := x.Name.Name
			sig := name
			if x.Recv != nil && len(x.Recv.List) > 0 {
				kind = "method"
			}

			// Check if function name indicates HTTP handler
			if strings.HasPrefix(name, "Handle") || strings.HasSuffix(name, "Handler") || strings.Contains(name, "Endpoint") {
				kind = "endpoint"
			}

			res.Symbols = append(res.Symbols, ParsedSymbol{
				Name:        name,
				Kind:        kind,
				LineStart:   start,
				LineEnd:     end,
				Signature:   sig,
				Docstring:   extractDoc(x.Doc),
				BodyContent: sliceLines(lines, start, end),
			})

		case *ast.CallExpr:
			// Detect Chi, Gin, or net/http route registrations e.g. r.Get("/path", handler)
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				method := strings.ToUpper(sel.Sel.Name)
				if isHTTPMethod(method) && len(x.Args) >= 1 {
					if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						routePath := strings.Trim(lit.Value, `"`)
						start := fset.Position(x.Pos()).Line
						end := fset.Position(x.End()).Line
						res.Symbols = append(res.Symbols, ParsedSymbol{
							Name:        fmt.Sprintf("%s %s", method, routePath),
							Kind:        "endpoint",
							LineStart:   start,
							LineEnd:     end,
							Signature:   fmt.Sprintf("%s %s", method, routePath),
							BodyContent: sliceLines(lines, start, end),
						})
					}
				}
			}
		}
		return true
	})

	return res
}

func parseOpenAPISpec(filePath, content string) (*CodeParseResult, bool) {
	// Try parsing JSON or YAML-derived JSON
	var doc struct {
		OpenAPI string `json:"openapi"`
		Swagger string `json:"swagger"`
		Info    struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"info"`
		Paths map[string]map[string]struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}

	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		// If direct JSON fails, use regex for paths
		return parseOpenAPIRegex(filePath, content)
	}

	res := &CodeParseResult{
		Language: "openapi",
		Package:  doc.Info.Title,
		Symbols:  make([]ParsedSymbol, 0),
	}

	for path, methods := range doc.Paths {
		for method, op := range methods {
			methodUpper := strings.ToUpper(method)
			if !isHTTPMethod(methodUpper) {
				continue
			}
			name := fmt.Sprintf("%s %s", methodUpper, path)
			body := fmt.Sprintf("Endpoint: %s\nSummary: %s\nOperationID: %s\nDescription: %s",
				name, op.Summary, op.OperationID, op.Description)
			res.Symbols = append(res.Symbols, ParsedSymbol{
				Name:        name,
				Kind:        "spec_route",
				Signature:   name,
				Docstring:   op.Summary,
				BodyContent: body,
			})
		}
	}

	return res, len(res.Symbols) > 0
}

var openAPIPathRegex = regexp.MustCompile(`(?m)^\s*['"]?(/[\w\-{}/.]+)['"]?:\s*$`)
var openAPIMethodRegex = regexp.MustCompile(`(?m)^\s*(get|post|put|delete|patch|options|head):\s*$`)

func parseOpenAPIRegex(filePath, content string) (*CodeParseResult, bool) {
	lines := strings.Split(content, "\n")
	res := &CodeParseResult{
		Language: "openapi",
		Package:  filepath.Base(filePath),
		Symbols:  make([]ParsedSymbol, 0),
	}

	var currentPath string
	for lineIdx, line := range lines {
		if m := openAPIPathRegex.FindStringSubmatch(line); len(m) > 1 {
			currentPath = m[1]
			continue
		}
		if currentPath != "" {
			if mm := openAPIMethodRegex.FindStringSubmatch(line); len(mm) > 1 {
				method := strings.ToUpper(mm[1])
				name := fmt.Sprintf("%s %s", method, currentPath)
				end := lineIdx + 15
				if end > len(lines) {
					end = len(lines)
				}
				res.Symbols = append(res.Symbols, ParsedSymbol{
					Name:        name,
					Kind:        "spec_route",
					LineStart:   lineIdx + 1,
					LineEnd:     end,
					Signature:   name,
					BodyContent: strings.Join(lines[lineIdx:end], "\n"),
				})
			}
		}
	}
	return res, len(res.Symbols) > 0
}

var genericSymbolRegex = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?(class|interface|type|enum|function|def|fn|func|struct)\s+([A-Za-z0-9_]+)`)
var routeRegex = regexp.MustCompile(`(?m)(?:app|router|r)\.(get|post|put|delete|patch)\(\s*['"]([^'"]+)['"]`)

func parseGenericCode(filePath, content, ext string) *CodeParseResult {
	lines := strings.Split(content, "\n")
	res := &CodeParseResult{
		Language: strings.TrimPrefix(ext, "."),
		Package:  filepath.Base(filepath.Dir(filePath)),
		Symbols:  make([]ParsedSymbol, 0),
	}

	for lineIdx, line := range lines {
		if matches := genericSymbolRegex.FindStringSubmatch(line); len(matches) > 2 {
			kind := matches[1]
			name := matches[2]
			start := lineIdx + 1
			end := start + 30
			if end > len(lines) {
				end = len(lines)
			}
			res.Symbols = append(res.Symbols, ParsedSymbol{
				Name:        name,
				Kind:        kind,
				LineStart:   start,
				LineEnd:     end,
				Signature:   strings.TrimSpace(line),
				BodyContent: sliceLines(lines, start, end),
			})
		} else if rMatches := routeRegex.FindStringSubmatch(line); len(rMatches) > 2 {
			method := strings.ToUpper(rMatches[1])
			path := rMatches[2]
			name := fmt.Sprintf("%s %s", method, path)
			start := lineIdx + 1
			end := start + 15
			if end > len(lines) {
				end = len(lines)
			}
			res.Symbols = append(res.Symbols, ParsedSymbol{
				Name:        name,
				Kind:        "endpoint",
				LineStart:   start,
				LineEnd:     end,
				Signature:   name,
				BodyContent: sliceLines(lines, start, end),
			})
		}
	}

	return res
}

func isHTTPMethod(m string) bool {
	switch m {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func sliceLines(lines []string, start, end int) string {
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

func extractDoc(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}
