-- Migration: Project-Scoped Confluence Integration & Project Membership

CREATE TABLE IF NOT EXISTS project_members (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    user_email TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('owner', 'member')),
    status TEXT NOT NULL CHECK(status IN ('active', 'pending')),
    created_at DATETIME NOT NULL,
    UNIQUE(project_id, user_email)
);

CREATE INDEX IF NOT EXISTS idx_project_members_project ON project_members(project_id);
CREATE INDEX IF NOT EXISTS idx_project_members_user ON project_members(user_id);
CREATE INDEX IF NOT EXISTS idx_project_members_email ON project_members(user_email);

-- Ensure all existing projects have their creator as active owner
INSERT OR IGNORE INTO project_members (id, project_id, user_id, user_email, role, status, created_at)
SELECT 'pm_' || p.id, p.id, p.owner_id, COALESCE(u.email, 'admin@docforge.local'), 'owner', 'active', p.created_at
FROM projects p
LEFT JOIN users u ON p.owner_id = u.id;

-- Extend confluence_targets with parent_page_title, updated_by, updated_at
ALTER TABLE confluence_targets ADD COLUMN parent_page_title TEXT;
ALTER TABLE confluence_targets ADD COLUMN updated_by TEXT;
ALTER TABLE confluence_targets ADD COLUMN updated_at DATETIME;
