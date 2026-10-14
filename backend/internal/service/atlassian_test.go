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
