-- Migration: Add confluence_page_url to documents table
ALTER TABLE documents ADD COLUMN confluence_page_url TEXT;
