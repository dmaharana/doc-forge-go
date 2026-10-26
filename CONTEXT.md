# DocForge

DocForge synthesizes product and engineering documentation from heterogeneous sources using language models, enabling structured human review and Confluence publication.

## Language

**Source**:
An ingested file or remote reference that supplies raw technical knowledge for documentation generation.
_Avoid_: Input, file, context document, upload

**Project**:
A workspace container grouping sources, generation requests, documents, and publication targets for a product.
_Avoid_: Workspace, repository, folder

**Generation**:
An asynchronous execution job that analyzes selected Sources using a configured LLM to synthesize draft Documents.
_Avoid_: Job, task, build, run

**Document**:
A synthesized technical documentation artifact adhering to a specific template type.
_Avoid_: Page, draft, output, article

**Document Version**:
An immutable historical snapshot of a Document's Markdown content and change summary.
_Avoid_: Revision, commit, diff

**Feedback**:
A structured review comment attached to a Document section indicating a correction, missing information, clarification, or style issue.
_Avoid_: Note, comment, review item, issue

**Blocking Feedback**:
A feedback item that must be resolved before a Document can be approved.
_Avoid_: Blocker, gate, critical issue

**Confluence Target**:
The destination Confluence Cloud site, space, and parent page mapped to a Project for publishing approved Documents.
_Avoid_: Export destination, wiki link

**Project Member**:
An authorized user assigned to a Project with defined permissions (Owner or Member) to view, collaborate, and export documentation.
_Avoid_: Collaborator, team member, participant

**Atlassian Session**:
An authenticated OAuth 2.0 session established via Atlassian SSO that supplies user identity claims and personal Confluence Cloud access tokens.
_Avoid_: Login token, Confluence account, SSO profile

**Source Chunk**:
A semantically bounded, enriched fragment of a Source carrying structural metadata (originating file, line range, AST symbol, or section).
_Avoid_: Snippet, text slice, segment, block

**Retrieval Index**:
An SQLite FTS5 full-text search index over Source Chunks scoped to a Project that provides deterministic BM25 ranking and structural filtering.
_Avoid_: Vector store, database, cache, catalog

**System Map**:
A synthesized architectural overview (component hierarchy, data flow, configurations) generated before document population to serve as the structural anchor.
_Avoid_: Summary, outline, blueprint

**Generation Stage**:
A discrete, auditable step within a multi-pass Generation pipeline (such as system mapping, section population, or pre-review verification) with persisted inputs and outputs.
_Avoid_: Sub-job, phase, task, worker run

**Pre-Review Check**:
An automated deterministic or model-based verification rule executed on a synthesized Document prior to human review, which may auto-correct issues or create blocking Feedback.
_Avoid_: Linter, test, automated test, validation step

**Associated LLM**:
The specific configured Language Model provider and model setting assigned to a Project as its default AI synthesis engine, which overrides the global default.
_Avoid_: Project model, default AI, local model

**Token Usage**:
An immutable audit ledger record capturing prompt/input tokens, completion/output tokens, model, and operation type for all AI synthesis and revision events within a Project, used for financial chargeback and quota tracking.
_Avoid_: Credit, cost metric, billing counter


