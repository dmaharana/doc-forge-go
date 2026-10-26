package service

import (
	"net/url"
	"strings"
	"testing"

	"docforge/internal/config"
	"docforge/internal/models"
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

func TestFormatConfluencePageURL(t *testing.T) {
	tests := []struct {
		name     string
		siteURL  string
		spaceKey string
		pageID   string
		webUI    string
		expected string
	}{
		{
			name:     "mock site with spaceKey and pageID",
			siteURL:  "https://mock-company.atlassian.net/wiki",
			spaceKey: "ENG",
			pageID:   "conf_12345",
			webUI:    "",
			expected: "https://mock-company.atlassian.net/wiki/spaces/ENG/pages/conf_12345",
		},
		{
			name:     "site without /wiki suffix and v2 webUI relative path",
			siteURL:  "https://acme.atlassian.net",
			spaceKey: "PROJ",
			pageID:   "9999",
			webUI:    "/spaces/PROJ/pages/9999/Architecture+Spec",
			expected: "https://acme.atlassian.net/wiki/spaces/PROJ/pages/9999/Architecture+Spec",
		},
		{
			name:     "site with /wiki and webUI having /wiki prefix",
			siteURL:  "https://acme.atlassian.net/wiki",
			spaceKey: "PROJ",
			pageID:   "9999",
			webUI:    "/wiki/spaces/PROJ/pages/9999/Architecture+Spec",
			expected: "https://acme.atlassian.net/wiki/spaces/PROJ/pages/9999/Architecture+Spec",
		},
		{
			name:     "webUI is absolute URL",
			siteURL:  "https://acme.atlassian.net",
			spaceKey: "PROJ",
			pageID:   "9999",
			webUI:    "https://custom-domain.com/pages/viewpage.action?pageId=9999",
			expected: "https://custom-domain.com/pages/viewpage.action?pageId=9999",
		},
		{
			name:     "no spaceKey falls back to viewpage permalink",
			siteURL:  "https://myteam.atlassian.net",
			spaceKey: "",
			pageID:   "54321",
			webUI:    "",
			expected: "https://myteam.atlassian.net/wiki/pages/viewpage.action?pageId=54321",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := models.FormatConfluencePageURL(tc.siteURL, tc.spaceKey, tc.pageID, tc.webUI)
			if actual != tc.expected {
				t.Errorf("FormatConfluencePageURL() = %q, want %q", actual, tc.expected)
			}
		})
	}
}

