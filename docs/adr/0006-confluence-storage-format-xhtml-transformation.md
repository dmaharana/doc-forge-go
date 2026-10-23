# Confluence Storage Format XHTML Transformation and Native Macro Mapping

We chose to transform Markdown to strict Confluence Storage Format XHTML using an AST-driven Goldmark parser and Atlassian macros rather than publishing raw Markdown inside preformatted `<pre>` blocks.

Confluence Cloud REST v2 strictly expects `representation: "storage"` containing valid XML. Wrapping Markdown in `<pre>` preserved text but produced unformatted, unstyled monospace blocks without interactive tables, collapsible code snippets, or callout panels. By using an AST walker with GFM extensions, DocForge converts Markdown headings, wrapped tables, and inline styles to native Confluence XHTML, maps GitHub-style alerts (`[!NOTE]`, `[!WARNING]`, `[!TIP]`) to native Confluence status macros, and wraps code blocks and Mermaid Architectural Diagrams in `<ac:structured-macro ac:name="code">` with CDATA escaping. This ensures exported documentation renders natively and seamlessly inside Confluence spaces.
