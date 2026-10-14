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
The destination Confluence Cloud space and parent page mapped to a Project for publishing approved Documents.
_Avoid_: Export destination, wiki link

**Project Member**:
An authorized user assigned to a Project with defined permissions (Owner or Member) to view, collaborate, and export documentation.
_Avoid_: Collaborator, team member, participant

**Atlassian Session**:
An authenticated OAuth 2.0 session established via Atlassian SSO that supplies user identity claims and personal Confluence Cloud access tokens.
_Avoid_: Login token, Confluence account, SSO profile

