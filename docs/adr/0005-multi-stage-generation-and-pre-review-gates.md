# Multi-Stage Generation and Pre-Review Verification Gates

We chose a multi-stage pipeline (System Mapping → Section-by-Section FTS5 Retrieval → Cross-Reference → Pre-Review Verification) with automated blocking feedback rather than single-shot prompt generation.

Single-shot generation dumps unbounded source text into a monolithic prompt, exceeding token budgets, dropping crucial implementation details, and hallucinating missing features. By decoupling generation into discrete persisted stages, DocForge generates a structural System Map first, populates individual sections using targeted BM25-retrieved Source Chunks with strict citation rules, and executes automated Pre-Review Checks that auto-correct formatting/glossary issues while writing unverified code claims and missing modules as blocking Feedback items.
