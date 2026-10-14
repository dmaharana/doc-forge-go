# DocForge

**DocForge** is an enterprise web application designed to synthesize high-quality initial technical documentation from heterogeneous sources (codebases, PDFs, Excel sheets, images, Markdown files, and remote repositories) using Large Language Models (OpenAI-compatible APIs, Anthropic, or local mock).

Humans remain firmly in the loop: reviewers inspect synthesized drafts in a dedicated side-by-side workspace, capture structured feedback (with hard-gated blocking items), trigger AI revisions, approve documents, and export directly into Atlassian Confluence spaces or download as Markdown.

---

## 🌟 Key Features

1. **Heterogeneous Ingestion Pipeline**:
   - Ingests source code, PDFs (pure-Go parser), Excel `.xlsx` spreadsheets, ZIP archives, Markdown, text, and images.
   - Remote source fetching from HTTP/HTTPS, Git repositories, or local file references.

2. **Embedded Documentation Templates**:
   - Product & Feature Specification (`product_spec`)
   - Architecture Decision Record (`adr`)
   - Release Notes (`release_notes`)
   - Operations Runbook (`runbook`)

3. **Human Review & Feedback Loop**:
   - Side-by-side Markdown editor with rendered preview (`remark-gfm`).
   - Version history inspection with changelogs.
   - Right-side feedback panel capturing structured review comments:
     - Categories: `Correction`, `Missing Information`, `Additional Fact`, `Clarification`, `Style`, `Other`.
     - Section anchors.
     - **Hard-gated Blocking Items**: Enforces that documents cannot transition to Approved while blocking feedback is open.
   - **Update with Feedback**: Background AI revision loop that applies open feedback items and updates document versions immutably.

4. **Publication & Export**:
   - Atlassian Confluence Cloud OAuth 2.0 (3LO) export with space and parent page target selection.
   - 1-click Markdown file download fallback when offline or without Confluence credentials.

5. **Multi-Channel Notifications**:
   - Email notifications dispatched to account identity.
   - Microsoft Teams Incoming Webhooks with formatted MessageCard alerts.

6. **Enterprise Architecture & Security**:
   - **Zero CGO**: Compiles with `CGO_ENABLED=0` to a single statically linked binary using `modernc.org/sqlite`.
   - **Secrets Encryption at Rest**: All API keys, Atlassian tokens, and Teams webhook URLs are encrypted using **AES-256 GCM**.
   - **Identity Ready**: JWT Bearer authentication + Microsoft SSO (Azure AD OpenID Connect) integration point.

---

## 🏗️ Monorepo Structure

```
doc-forge-go/
├── backend/
│   ├── cmd/server/main.go       # Server entrypoint and worker launcher
│   ├── internal/
│   │   ├── api/                 # REST handlers, JWT auth, middleware, router
│   │   ├── config/              # Environment config loader (.env)
│   │   ├── crypto/              # AES-256 GCM encryption helpers
│   │   ├── db/                  # SQLite connector, pure-Go WAL, schema migrations
│   │   ├── ingestion/           # File storage and text extractors (PDF, XLSX, ZIP, Git)
│   │   ├── llm/                 # LLM provider abstraction (OpenAI, Anthropic, Mock)
│   │   ├── models/              # SQLite data models
│   │   ├── repository/          # Transaction-safe database repositories
│   │   ├── service/             # Generation worker, Atlassian OAuth, notifications
│   │   └── templates/           # Embedded Go text/templates (.tmpl)
│   ├── migrations/              # 001_initial_schema.sql
│   ├── go.mod
│   └── .env.example
├── frontend/                    # React 19 + TypeScript + Vite + Tailwind CSS
│   ├── src/
│   │   ├── components/          # UI components (Navbar, dialogs, buttons)
│   │   ├── context/             # AuthContext (JWT management)
│   │   ├── lib/                 # API client (fetch wrapper)
│   │   ├── pages/               # Login, Dashboard, Projects, Review, Admin, Settings
│   │   └── types/               # TypeScript domain interfaces
│   ├── package.json
│   └── vite.config.ts
├── docs/
│   └── adr/                     # Architectural Decision Records
├── CONTEXT.md                   # Bounded domain glossary
├── .env.example
└── README.md
```

---

## 🚀 Getting Started

### Prerequisites
- **Go 1.22+**
- **Node.js 18+** & **pnpm** (or npm/yarn)

### Quick Start with Make

```bash
# Build complete self-contained binary (frontend embedded into Go binary)
make build

# Run automated backend test suite
make test

# Start the single binary server (serves both Web UI & API at http://localhost:8080)
make run-backend
```

*Once built, `./backend/server` has ZERO external runtime dependencies—no Node.js or CGO required in production!*

### Optional: Frontend Development with HMR

If modifying React components with Vite Hot Module Reloading:

```bash
# In a separate terminal, launch the Vite dev server with proxy
make run-frontend
```

```bash
cd frontend

# Install dependencies
pnpm install

# Build production bundle
pnpm run build

# Start development server (defaults to http://localhost:5173)
pnpm run dev
```

Visit **http://localhost:5173** in your browser.

---

## 📖 End-to-End Walkthrough

1. **Sign In**:
   - Access http://localhost:5173 and log in with `admin@docforge.local` / `Admin123!`.
2. **Configure AI Engine (Admin)**:
   - Navigate to **LLM Config** in the top navigation bar.
   - Choose your provider:
     - **Offline Mock**: Zero external dependencies (great for testing).
     - **OpenAI**: Input your API key (encrypted with AES-GCM) and model (e.g. `gpt-4o`).
     - **Anthropic**: Input your Claude API key and model (e.g. `claude-3-5-sonnet-20241022`).
3. **Create a Project**:
   - Navigate to **Projects** → click **"+ New Project"**.
   - Enter a name (e.g., `Billing & Invoicing Service`) and domain description.
4. **Ingest Sources**:
   - Under the project's **Sources** tab:
     - Click **"Upload File"** to attach a PDF, `.xlsx` spreadsheet, code snippet, or ZIP archive.
     - Or click **"Add Remote Location"** to ingest from a public HTTP URL or Git repository.
     - Click **"View Extracted Text"** to verify extracted grounding text.
5. **Request Documentation Generation**:
   - Click **"Request Generation"**.
   - Select your desired templates (Feature Spec, ADR, Release Notes, Runbook) and sources.
   - Click **"Submit Generation Request"**. The in-process worker claims the job, queries the LLM, and creates draft documents.
6. **Review Document**:
   - Open the **Documents** tab and click **"Review Document"** on any synthesized draft.
   - Toggle between **Preview** (rich rendered Markdown) and **Edit Raw**.
   - Browse **History** to view versions and changelogs.
7. **Add Structured Feedback & Blocking Gate**:
   - In the right-side **Feedback panel**, click **"+ Add Feedback"**.
   - Select a category (`Correction`, `Missing Information`, etc.) and optionally check **"Blocking – must be resolved before approval"**.
   - Click **Approve Document**: Notice the **Hard Gate** blocks approval if any blocking item is open!
8. **Update Document with Feedback**:
   - Click **"Update with Feedback"** in the bottom action bar.
   - The AI revises the document addressing all open feedback items, creates Version 2, and automatically marks feedback items addressed!
9. **Final Sign-off & Export**:
   - Click **"Approve Document"** (now unblocked).
   - Click **"Export to Confluence"** to publish to your configured Confluence space, or click **"Download Markdown"** to save locally.

---

## 🔒 Security & Data Model

- **Database**: SQLite WAL mode with 5000ms busy timeout and single writer serialization.
- **Crypto**: AES-256 GCM using a 32-byte key defined in `ENCRYPTION_KEY`.
- **RBAC**: Administrator (`admin`) and standard user (`user`) roles enforced via JWT middleware.

---

## 🧪 Testing

Run the Go test suite:
```bash
cd backend
go test -v ./...
```
Runs comprehensive integration tests validating authentication, project creation, source ingestion, background generation worker, hard gate approval checks, and Confluence publishing.
