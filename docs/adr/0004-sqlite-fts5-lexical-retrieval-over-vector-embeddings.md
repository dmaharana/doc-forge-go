# SQLite FTS5 Lexical Retrieval Over Vector Embeddings

We chose SQLite FTS5 (full-text search with BM25 ranking) and AST-enriched structural chunking rather than vector embeddings for source retrieval.

Embedding models face deprecation, version drift, and API availability issues over long document lifecycles, and local embedding models require CGO or heavy runtimes that violate DocForge's single-binary zero-dependency architecture. SQLite FTS5 runs in pure Go via `modernc.org/sqlite`, incurs zero external API costs, works offline, and delivers deterministic keyword and symbol matching when enriched with AST metadata prefixes.
