package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"docforge/internal/config"
	"docforge/internal/crypto"
	"docforge/internal/db"
	"docforge/internal/models"
	"docforge/internal/repository"
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

func TestResolveConfluenceURL(t *testing.T) {
	cloudID := "cloud-uuid-123"
	tests := []struct {
		name     string
		rawLink  string
		expected string
	}{
		{
			name:     "empty link",
			rawLink:  "",
			expected: "",
		},
		{
			name:     "relative v2 cursor link with /wiki prefix",
			rawLink:  "/wiki/api/v2/spaces?cursor=eyJpZCI6IjEyMyJ9&limit=250",
			expected: "https://api.atlassian.com/ex/confluence/cloud-uuid-123/wiki/api/v2/spaces?cursor=eyJpZCI6IjEyMyJ9&limit=250",
		},
		{
			name:     "relative v1 pagination link with /rest prefix",
			rawLink:  "/rest/api/space?limit=100&start=100",
			expected: "https://api.atlassian.com/ex/confluence/cloud-uuid-123/wiki/rest/api/space?limit=100&start=100",
		},
		{
			name:     "relative link with full /ex/confluence/ path",
			rawLink:  "/ex/confluence/cloud-uuid-123/wiki/api/v2/spaces?cursor=abc",
			expected: "https://api.atlassian.com/ex/confluence/cloud-uuid-123/wiki/api/v2/spaces?cursor=abc",
		},
		{
			name:     "absolute api.atlassian.com url",
			rawLink:  "https://api.atlassian.com/ex/confluence/cloud-uuid-123/wiki/api/v2/spaces?cursor=abc&limit=250",
			expected: "https://api.atlassian.com/ex/confluence/cloud-uuid-123/wiki/api/v2/spaces?cursor=abc&limit=250",
		},
		{
			name:     "absolute tenant site url converted to proxy url",
			rawLink:  "https://company.atlassian.net/wiki/rest/api/space?limit=100&start=200",
			expected: "https://api.atlassian.com/ex/confluence/cloud-uuid-123/wiki/rest/api/space?limit=100&start=200",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := resolveConfluenceURL(cloudID, tc.rawLink)
			if actual != tc.expected {
				t.Errorf("resolveConfluenceURL() = %q, want %q", actual, tc.expected)
			}
		})
	}
}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestListSpaces_PaginationMultiPage(t *testing.T) {
	cfg := &config.Config{
		DBPath:            ":memory:",
		EncryptionKey:     []byte("01234567890123456789012345678901"),
		AtlassianClientID: "real-test-client-id",
	}
	database, err := db.Connect(cfg)
	if err != nil {
		t.Fatalf("db error: %v", err)
	}
	defer database.Close()
	repo := repository.New(database)

	user := &models.User{
		Email:        "pagination@test.local",
		Name:         "Pagination Tester",
		Role:         models.RoleUser,
		PasswordHash: "h",
		IsActive:     true,
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	encToken, _ := crypto.Encrypt("test-access-token", cfg.EncryptionKey)
	expiry := time.Now().Add(1 * time.Hour)
	conn := &models.AtlassianConnection{
		UserID:               user.ID,
		CloudID:              "cloud-12345",
		SiteURL:              "https://myteam.atlassian.net/wiki",
		AccessTokenEncrypted: encToken,
		TokenExpiry:          &expiry,
	}
	if err := repo.UpsertAtlassianConnection(conn); err != nil {
		t.Fatalf("failed to insert connection: %v", err)
	}

	oldTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = oldTransport }()

	var calls []string
	http.DefaultTransport = roundTripFunc(func(req *http.Request) *http.Response {
		u := req.URL.String()
		calls = append(calls, u)

		// V1 returns 404 to fall back to V2
		if strings.Contains(u, "/wiki/rest/api/space") {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString(`{"error": "not found"}`)),
				Header:     make(http.Header),
			}
		}

		// V2 initial page
		if strings.Contains(u, "/wiki/api/v2/spaces") && !strings.Contains(u, "cursor=") {
			body := `{
				"results": [
					{"id": "1", "key": "ZETA", "name": "Zeta Operations", "type": "global"},
					{"id": "2", "key": "BETA", "name": "Beta Research", "type": "global"}
				],
				"_links": {
					"next": "/wiki/api/v2/spaces?cursor=cur_page_2&limit=250"
				}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}
		}

		// V2 second page
		if strings.Contains(u, "cursor=cur_page_2") {
			body := `{
				"results": [
					{"id": "3", "key": "ALPHA", "name": "Alpha Platform", "type": "global"}
				],
				"_links": {}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}
		}

		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
			Header:     make(http.Header),
		}
	})

	svc := NewAtlassianService(cfg, repo)
	spaces, err := svc.ListSpaces(context.Background(), user.ID, "cloud-12345")
	if err != nil {
		t.Fatalf("ListSpaces failed: %v", err)
	}

	if len(spaces) != 3 {
		t.Fatalf("expected 3 spaces across both pages, got %d", len(spaces))
	}

	// Verify they are sorted alphabetically by Name
	if spaces[0].Key != "ALPHA" || spaces[1].Key != "BETA" || spaces[2].Key != "ZETA" {
		t.Errorf("spaces not sorted by Name as expected: got %+v", spaces)
	}
}

func TestListSpacePages_PaginationMultiPage(t *testing.T) {
	cfg := &config.Config{
		DBPath:            ":memory:",
		EncryptionKey:     []byte("01234567890123456789012345678901"),
		AtlassianClientID: "real-test-client-id",
	}
	database, err := db.Connect(cfg)
	if err != nil {
		t.Fatalf("db error: %v", err)
	}
	defer database.Close()
	repo := repository.New(database)

	user := &models.User{
		Email:        "pages@test.local",
		Name:         "Pages Tester",
		Role:         models.RoleUser,
		PasswordHash: "h",
		IsActive:     true,
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	encToken, _ := crypto.Encrypt("test-access-token", cfg.EncryptionKey)
	expiry := time.Now().Add(1 * time.Hour)
	conn := &models.AtlassianConnection{
		UserID:               user.ID,
		CloudID:              "cloud-12345",
		SiteURL:              "https://myteam.atlassian.net/wiki",
		AccessTokenEncrypted: encToken,
		TokenExpiry:          &expiry,
	}
	if err := repo.UpsertAtlassianConnection(conn); err != nil {
		t.Fatalf("failed to insert connection: %v", err)
	}

	oldTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = oldTransport }()

	http.DefaultTransport = roundTripFunc(func(req *http.Request) *http.Response {
		u := req.URL.String()

		// CQL Content search page 1
		if strings.Contains(u, "/wiki/rest/api/content/search") && !strings.Contains(u, "start=2") {
			body := `{
				"results": [
					{"id": "pg-2", "title": "Zulu Deployment Guide"},
					{"id": "pg-1", "title": "Bravo Architecture"}
				],
				"_links": {
					"next": "/wiki/rest/api/content/search?cql=space%20%3D%20%22ENG%22&limit=100&start=2"
				}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}
		}

		// CQL Content search page 2
		if strings.Contains(u, "start=2") {
			body := `{
				"results": [
					{"id": "pg-3", "title": "Alpha Overview"}
				],
				"_links": {}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}
		}

		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
			Header:     make(http.Header),
		}
	})

	svc := NewAtlassianService(cfg, repo)
	pages, err := svc.ListSpacePages(context.Background(), user.ID, "ENG", "cloud-12345")
	if err != nil {
		t.Fatalf("ListSpacePages failed: %v", err)
	}

	if len(pages) != 3 {
		t.Fatalf("expected 3 pages across both pages, got %d", len(pages))
	}

	// Verify they are sorted alphabetically by Title
	if pages[0].Title != "Alpha Overview" || pages[1].Title != "Bravo Architecture" || pages[2].Title != "Zulu Deployment Guide" {
		t.Errorf("pages not sorted by Title as expected: got %+v", pages)
	}
}



