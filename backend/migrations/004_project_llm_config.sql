-- Migration 004: Add llm_config_id to projects table
ALTER TABLE projects ADD COLUMN llm_config_id TEXT REFERENCES llm_configs(id);
