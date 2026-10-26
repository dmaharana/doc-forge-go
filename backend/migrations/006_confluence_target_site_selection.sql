-- Migration: Confluence Target Atlassian Site Selection
ALTER TABLE confluence_targets ADD COLUMN cloud_id TEXT;
ALTER TABLE confluence_targets ADD COLUMN site_url TEXT;
ALTER TABLE confluence_targets ADD COLUMN site_name TEXT;
