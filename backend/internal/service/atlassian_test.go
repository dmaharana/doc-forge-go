package service

import (
	"net/url"
	"strings"
	"testing"

	"docforge/internal/config"
)

func TestAtlassianScopes(t *testing.T) {
	cfg := &config.Config{
		AtlassianClientID: "test-client-id",
		AtlassianRedirectURI: "http://localhost:8080/api/v1/atlassian/callback",
	}
	svc := NewAtlassianService(cfg, nil)

	scopes := svc.getScopes()
	requiredScopes := []string{
		"write:page:confluence",
		"read:page:confluence",
		"read:space:confluence",
		"read:me",
		"offline_access",
	}

	for _, sc := range requiredScopes {
		if !strings.Contains(scopes, sc) {
			t.Errorf("expected default scopes to contain %q, but got: %s", sc, scopes)
		}
	}

	authURL := svc.GetAuthorizationURL("test-state")
	for _, sc := range requiredScopes {
		if !strings.Contains(authURL, url.QueryEscape(sc)) {
			t.Errorf("expected authorization URL to contain %q, but got: %s", url.QueryEscape(sc), authURL)
		}
	}
}

func TestGetAccessibleSites_Mock(t *testing.T) {
	cfg := &config.Config{
		MockAtlassian: true,
	}
	svc := NewAtlassianService(cfg, nil)

	sites, err := svc.GetAccessibleSites(nil, "any-user")
	if err != nil {
		t.Fatalf("unexpected error getting accessible sites: %v", err)
	}
	if len(sites) < 2 {
		t.Fatalf("expected at least 2 accessible sites in mock mode, got %d", len(sites))
	}

	site1 := sites[0]
	site2 := sites[1]
	if site1.ID != "mock-cloud-id-123" || site2.ID != "mock-cloud-id-456" {
		t.Errorf("expected mock cloud IDs 'mock-cloud-id-123' and 'mock-cloud-id-456', got %s and %s", site1.ID, site2.ID)
	}
}

func TestListSpaces_MultiSite(t *testing.T) {
	cfg := &config.Config{
		MockAtlassian: true,
	}
	svc := NewAtlassianService(cfg, nil)

	// Test spaces for site 1
	spaces1, err := svc.ListSpaces(nil, "any-user", "mock-cloud-id-123")
	if err != nil {
		t.Fatalf("unexpected error listing spaces for site 1: %v", err)
	}
	foundENG := false
	for _, sp := range spaces1 {
		if sp.Key == "ENG" {
			foundENG = true
			break
		}
	}
	if !foundENG {
		t.Errorf("expected to find ENG space in site 1")
	}

	// Test spaces for site 2
	spaces2, err := svc.ListSpaces(nil, "any-user", "mock-cloud-id-456")
	if err != nil {
		t.Fatalf("unexpected error listing spaces for site 2: %v", err)
	}
	foundINFRA := false
	for _, sp := range spaces2 {
		if sp.Key == "INFRA" {
			foundINFRA = true
			break
		}
	}
	if !foundINFRA {
		t.Errorf("expected to find INFRA space in site 2")
	}
}
