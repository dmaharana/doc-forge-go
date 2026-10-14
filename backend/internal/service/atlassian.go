package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"docforge/internal/config"
	"docforge/internal/crypto"
	"docforge/internal/models"
	"docforge/internal/repository"

	"github.com/golang-jwt/jwt/v5"
)

type AtlassianSpace struct {
	ID   string `json:"id,omitempty"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type AtlassianPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type AtlassianService struct {
	cfg  *config.Config
	repo *repository.Repository
}

func NewAtlassianService(cfg *config.Config, repo *repository.Repository) *AtlassianService {
	return &AtlassianService{cfg: cfg, repo: repo}
}

func (s *AtlassianService) IsMockMode() bool {
	return s.cfg.MockAtlassian || s.cfg.AtlassianClientID == ""
}

func (s *AtlassianService) getScopes() string {
	if s.cfg.AtlassianScopes != "" {
		return s.cfg.AtlassianScopes
	}
	return "read:me offline_access write:page:confluence read:page:confluence read:space:confluence write:confluence-content read:confluence-space.summary write:confluence-file read:confluence-content.all read:confluence-content.summary search:confluence readonly:content.attachment:confluence"
}

func (s *AtlassianService) GetAuthorizationURL(state string) string {
	if s.IsMockMode() {
		// Mock authorization redirect directly to callback
		return fmt.Sprintf("/api/v1/atlassian/callback?code=mock_atlassian_code&state=%s", url.QueryEscape(state))
	}

	scopes := s.getScopes()
	return fmt.Sprintf(
		"https://auth.atlassian.com/authorize?audience=api.atlassian.com&client_id=%s&scope=%s&redirect_uri=%s&state=%s&response_type=code&prompt=consent",
		url.QueryEscape(s.cfg.AtlassianClientID),
		url.QueryEscape(scopes),
		url.QueryEscape(s.cfg.AtlassianRedirectURI),
		url.QueryEscape(state),
	)
}

func (s *AtlassianService) GetSSOAuthorizationURL(state string) string {
	if s.IsMockMode() {
		return fmt.Sprintf("/api/v1/auth/atlassian/callback?code=mock_atlassian_code&state=%s", url.QueryEscape(state))
	}

	scopes := s.getScopes()
	redirectURI := s.cfg.AtlassianRedirectURI
	if strings.HasSuffix(redirectURI, "/api/v1/atlassian/callback") {
		redirectURI = strings.TrimSuffix(redirectURI, "/api/v1/atlassian/callback") + "/api/v1/auth/atlassian/callback"
	}

	return fmt.Sprintf(
		"https://auth.atlassian.com/authorize?audience=api.atlassian.com&client_id=%s&scope=%s&redirect_uri=%s&state=%s&response_type=code&prompt=consent",
		url.QueryEscape(s.cfg.AtlassianClientID),
		url.QueryEscape(scopes),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)
}

func (s *AtlassianService) ExchangeSSOCode(ctx context.Context, code string) (*models.User, *models.AtlassianConnection, error) {
	if s.IsMockMode() || strings.HasPrefix(code, "mock_atlassian_code") {
		email := "admin@docforge.local"
		name := "System Administrator"

		parts := strings.Split(code, ":")
		if len(parts) >= 2 && parts[1] != "" {
			email = parts[1]
			name = parts[1]
		}
		if len(parts) >= 3 && parts[2] != "" {
			name = parts[2]
		}

		user, err := s.repo.GetUserByEmail(email)
		if err != nil || user == nil {
			// Auto provision
			user = &models.User{
				Email:    email,
				Name:     name,
				Role:     models.RoleUser,
				IsActive: true,
			}
			users, _ := s.repo.ListUsers()
			if len(users) == 0 {
				user.Role = models.RoleAdmin
			}
			if err := s.repo.CreateUser(user); err != nil {
				return nil, nil, fmt.Errorf("failed to auto-provision user: %w", err)
			}
		}

		conn, err := s.ExchangeCode(ctx, user.ID, "mock_atlassian_code")
		if err != nil {
			return nil, nil, err
		}

		_ = s.repo.ClaimPendingProjectMemberships(user.Email, user.ID)
		return user, conn, nil
	}

	// Real 3LO flow
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", s.cfg.AtlassianClientID)
	data.Set("client_secret", s.cfg.AtlassianClientSecret)
	data.Set("code", code)

	redirectURI := s.cfg.AtlassianRedirectURI
	if strings.HasSuffix(redirectURI, "/api/v1/atlassian/callback") {
		redirectURI = strings.TrimSuffix(redirectURI, "/api/v1/atlassian/callback") + "/api/v1/auth/atlassian/callback"
	}
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://auth.atlassian.com/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("failed token exchange: %s", string(b))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, nil, err
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	cloudID, siteURL := s.fetchAccessibleResources(ctx, tokenResp.AccessToken)

	var profile struct {
		AccountID string `json:"account_id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		Picture   string `json:"picture"`
	}

	// 1. Try fetching profile from /me (works when read:me scope is configured)
	profileReq, err := http.NewRequestWithContext(ctx, "GET", "https://api.atlassian.com/me", nil)
	if err == nil {
		profileReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		profileReq.Header.Set("Accept", "application/json")
		if profileResp, pErr := client.Do(profileReq); pErr == nil {
			defer profileResp.Body.Close()
			if profileResp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(profileResp.Body).Decode(&profile)
			}
		}
	}

	// 2. Fallback to Confluence current user if /me is unauthorized (e.g. only Confluence scopes available)
	if (profile.Email == "" || profile.Name == "") && cloudID != "" {
		uURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/user/current", cloudID)
		uReq, uErr := http.NewRequestWithContext(ctx, "GET", uURL, nil)
		if uErr == nil {
			uReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
			uReq.Header.Set("Accept", "application/json")
			if uResp, uDoErr := client.Do(uReq); uDoErr == nil {
				defer uResp.Body.Close()
				if uResp.StatusCode == http.StatusOK {
					var confUser struct {
						AccountID   string `json:"accountId"`
						Email       string `json:"email"`
						DisplayName string `json:"displayName"`
						PublicName  string `json:"publicName"`
					}
					if json.NewDecoder(uResp.Body).Decode(&confUser) == nil {
						if profile.AccountID == "" {
							profile.AccountID = confUser.AccountID
						}
						if profile.Name == "" {
							if confUser.DisplayName != "" {
								profile.Name = confUser.DisplayName
							} else {
								profile.Name = confUser.PublicName
							}
						}
						if profile.Email == "" {
							profile.Email = confUser.Email
						}
					}
				}
			}
		}
	}

	// 3. Fallback: extract Atlassian account ID from the access token JWT if still missing
	if profile.AccountID == "" && tokenResp.AccessToken != "" {
		parser := jwt.NewParser()
		claims := jwt.MapClaims{}
		if _, _, err := parser.ParseUnverified(tokenResp.AccessToken, claims); err == nil {
			if sub, ok := claims["sub"].(string); ok && sub != "" {
				profile.AccountID = sub
			}
		}
	}

	// 4. Fallback: if email is hidden by user privacy settings but accountId exists
	if profile.Email == "" && profile.AccountID != "" {
		profile.Email = fmt.Sprintf("%s@atlassian.user", profile.AccountID)
		if profile.Name == "" {
			profile.Name = "Atlassian User"
		}
	}

	if profile.Email == "" {
		return nil, nil, fmt.Errorf("atlassian profile did not return an email address: please ensure 'User identity API (read:me)' is enabled on your OAuth app or connect via DocForge account settings")
	}

	if profile.Name == "" {
		profile.Name = strings.Split(profile.Email, "@")[0]
	}

	user, err := s.repo.GetUserByEmail(profile.Email)
	if err != nil || user == nil {
		user = &models.User{
			Email:    profile.Email,
			Name:     profile.Name,
			Role:     models.RoleUser,
			IsActive: true,
		}
		users, _ := s.repo.ListUsers()
		if len(users) == 0 {
			user.Role = models.RoleAdmin
		}
		if err := s.repo.CreateUser(user); err != nil {
			return nil, nil, fmt.Errorf("failed to auto-provision user: %w", err)
		}
	}

	encAccess, err := crypto.Encrypt(tokenResp.AccessToken, s.cfg.EncryptionKey)
	if err != nil {
		return nil, nil, err
	}
	encRefresh, err := crypto.Encrypt(tokenResp.RefreshToken, s.cfg.EncryptionKey)
	if err != nil {
		return nil, nil, err
	}

	conn := &models.AtlassianConnection{
		UserID:                user.ID,
		AccessTokenEncrypted:  encAccess,
		RefreshTokenEncrypted: encRefresh,
		TokenExpiry:           &expiry,
		CloudID:               cloudID,
		SiteURL:               siteURL,
	}
	if err := s.repo.UpsertAtlassianConnection(conn); err != nil {
		return nil, nil, err
	}

	_ = s.repo.ClaimPendingProjectMemberships(user.Email, user.ID)
	return user, conn, nil
}

func (s *AtlassianService) ExchangeCode(ctx context.Context, userID, code string) (*models.AtlassianConnection, error) {
	if s.IsMockMode() || code == "mock_atlassian_code" {
		expiry := time.Now().Add(30 * 24 * time.Hour)
		encAccess, _ := crypto.Encrypt("mock_access_token_12345", s.cfg.EncryptionKey)
		encRefresh, _ := crypto.Encrypt("mock_refresh_token_67890", s.cfg.EncryptionKey)

		conn := &models.AtlassianConnection{
			UserID:                userID,
			AccessTokenEncrypted:  encAccess,
			RefreshTokenEncrypted: encRefresh,
			TokenExpiry:           &expiry,
			CloudID:               "mock-cloud-id-123",
			SiteURL:               "https://mock-company.atlassian.net/wiki",
		}
		if err := s.repo.UpsertAtlassianConnection(conn); err != nil {
			return nil, err
		}
		return conn, nil
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", s.cfg.AtlassianClientID)
	data.Set("client_secret", s.cfg.AtlassianClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", s.cfg.AtlassianRedirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://auth.atlassian.com/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed token exchange: %s", string(b))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Fetch accessible resources
	cloudID, siteURL := s.fetchAccessibleResources(ctx, tokenResp.AccessToken)

	encAccess, err := crypto.Encrypt(tokenResp.AccessToken, s.cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	encRefresh, err := crypto.Encrypt(tokenResp.RefreshToken, s.cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}

	conn := &models.AtlassianConnection{
		UserID:                userID,
		AccessTokenEncrypted:  encAccess,
		RefreshTokenEncrypted: encRefresh,
		TokenExpiry:           &expiry,
		CloudID:               cloudID,
		SiteURL:               siteURL,
	}

	if err := s.repo.UpsertAtlassianConnection(conn); err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *AtlassianService) RefreshAccessToken(ctx context.Context, conn *models.AtlassianConnection) (string, error) {
	if conn == nil || conn.RefreshTokenEncrypted == "" {
		return "", fmt.Errorf("no refresh token available")
	}

	refreshToken, err := crypto.Decrypt(conn.RefreshTokenEncrypted, s.cfg.EncryptionKey)
	if err != nil || refreshToken == "" {
		return "", fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", s.cfg.AtlassianClientID)
	data.Set("client_secret", s.cfg.AtlassianClientSecret)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://auth.atlassian.com/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token refresh network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bErr, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token refresh rejected by Atlassian (%d): %s", resp.StatusCode, strings.TrimSpace(string(bErr)))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
		return "", fmt.Errorf("invalid token refresh response")
	}

	encAccess, err := crypto.Encrypt(tokenResp.AccessToken, s.cfg.EncryptionKey)
	if err != nil {
		return "", err
	}
	conn.AccessTokenEncrypted = encAccess

	if tokenResp.RefreshToken != "" {
		if encRefresh, err := crypto.Encrypt(tokenResp.RefreshToken, s.cfg.EncryptionKey); err == nil {
			conn.RefreshTokenEncrypted = encRefresh
		}
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	conn.TokenExpiry = &expiry

	if err := s.repo.UpsertAtlassianConnection(conn); err != nil {
		log.Printf("[ATLASSIAN ERROR] Failed to save rotated tokens: %v", err)
	} else {
		log.Printf("[ATLASSIAN] Successfully refreshed access token and rotated refresh token")
	}

	return tokenResp.AccessToken, nil
}

func (s *AtlassianService) getValidAccessToken(ctx context.Context, conn *models.AtlassianConnection) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("no atlassian connection")
	}

	if s.IsMockMode() {
		return crypto.Decrypt(conn.AccessTokenEncrypted, s.cfg.EncryptionKey)
	}

	needsRefresh := false
	if conn.TokenExpiry == nil {
		needsRefresh = true
	} else if time.Now().After(conn.TokenExpiry.Add(-2 * time.Minute)) {
		needsRefresh = true
	}

	if needsRefresh && conn.RefreshTokenEncrypted != "" {
		newToken, err := s.RefreshAccessToken(ctx, conn)
		if err == nil && newToken != "" {
			return newToken, nil
		}
		log.Printf("[ATLASSIAN] Token auto-refresh failed: %v", err)
		if conn.TokenExpiry != nil && time.Now().After(*conn.TokenExpiry) {
			return "", fmt.Errorf("your Atlassian session has expired. Please click 'Reconnect Atlassian Session' in Settings or on the review page")
		}
	}

	return crypto.Decrypt(conn.AccessTokenEncrypted, s.cfg.EncryptionKey)
}

func (s *AtlassianService) fetchAccessibleResources(ctx context.Context, accessToken string) (string, string) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	var resources []struct {
		ID     string   `json:"id"`
		URL    string   `json:"url"`
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resources); err == nil && len(resources) > 0 {
		for _, r := range resources {
			for _, sc := range r.Scopes {
				if strings.Contains(sc, "confluence") || strings.Contains(sc, "space") || strings.Contains(sc, "page") {
					return r.ID, r.URL
				}
			}
		}
		return resources[0].ID, resources[0].URL
	}
	return "", ""
}

func (s *AtlassianService) ListSpaces(ctx context.Context, userID string) ([]AtlassianSpace, error) {
	conn, err := s.repo.GetAtlassianConnectionByUserID(userID)
	if err != nil || conn == nil {
		if s.IsMockMode() {
			return []AtlassianSpace{
				{Key: "ENG", Name: "Engineering & Architecture", Type: "global"},
				{Key: "PROD", Name: "Product Management", Type: "global"},
				{Key: "OPS", Name: "Platform Operations", Type: "global"},
				{Key: "DOCS", Name: "Technical Documentation", Type: "global"},
			}, nil
		}
		return nil, fmt.Errorf("no atlassian connection found for user")
	}

	if s.IsMockMode() || conn.CloudID == "mock-cloud-id-123" {
		return []AtlassianSpace{
			{Key: "ENG", Name: "Engineering & Architecture", Type: "global"},
			{Key: "PROD", Name: "Product Management", Type: "global"},
			{Key: "OPS", Name: "Platform Operations", Type: "global"},
			{Key: "DOCS", Name: "Technical Documentation", Type: "global"},
		}, nil
	}

	if strings.TrimSpace(conn.CloudID) == "" {
		return nil, fmt.Errorf("no Confluence Cloud site is linked to your Atlassian session. Please reconnect your Atlassian account with site access")
	}

	accessToken, err := s.getValidAccessToken(ctx, conn)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Try Confluence REST API v1: GET /wiki/rest/api/space (works with read:confluence-space.summary)
	v1URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/space?limit=100", conn.CloudID)
	req1, err := http.NewRequestWithContext(ctx, "GET", v1URL, nil)
	if err == nil {
		req1.Header.Set("Authorization", "Bearer "+accessToken)
		req1.Header.Set("Accept", "application/json")
		resp1, err1 := client.Do(req1)
		if err1 == nil {
			defer resp1.Body.Close()
			if resp1.StatusCode == http.StatusOK {
				var data1 struct {
					Results []struct {
						Key  string `json:"key"`
						Name string `json:"name"`
						Type string `json:"type"`
					} `json:"results"`
				}
				if json.NewDecoder(resp1.Body).Decode(&data1) == nil {
					spaces := make([]AtlassianSpace, 0, len(data1.Results))
					for _, r := range data1.Results {
						spaces = append(spaces, AtlassianSpace{Key: r.Key, Name: r.Name, Type: r.Type})
					}
					return spaces, nil
				}
			} else if resp1.StatusCode == http.StatusUnauthorized {
				// Missing scope: fall through to v2 attempt
			} else if resp1.StatusCode == http.StatusGone {
				// v1 deprecated - fall through to v2
			}
		}
	}

	// 2. Try Confluence REST API v2: GET /wiki/api/v2/spaces?limit=100 (requires read:space:confluence)
	v2URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces?limit=100", conn.CloudID)
	req2, err := http.NewRequestWithContext(ctx, "GET", v2URL, nil)
	if err == nil {
		req2.Header.Set("Authorization", "Bearer "+accessToken)
		req2.Header.Set("Accept", "application/json")
		resp2, err2 := client.Do(req2)
		if err2 == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode == http.StatusOK {
				var data2 struct {
					Results []struct {
						ID   string `json:"id"`
						Key  string `json:"key"`
						Name string `json:"name"`
						Type string `json:"type"`
					} `json:"results"`
				}
				if json.NewDecoder(resp2.Body).Decode(&data2) == nil {
					spaces := make([]AtlassianSpace, 0, len(data2.Results))
					for _, r := range data2.Results {
						spaces = append(spaces, AtlassianSpace{
							ID:   r.ID,
							Key:  r.Key,
							Name: r.Name,
							Type: r.Type,
						})
					}
					return spaces, nil
				}
			}
		}
	}

	// 3. Fallback: discover spaces via CQL Search: GET /wiki/rest/api/search?cql=type=page (works with search:confluence)
	sURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/search?cql=%s&limit=50", conn.CloudID, url.QueryEscape("type = page"))
	sReq, sErr := http.NewRequestWithContext(ctx, "GET", sURL, nil)
	if sErr == nil {
		sReq.Header.Set("Authorization", "Bearer "+accessToken)
		sReq.Header.Set("Accept", "application/json")
		sResp, sDoErr := client.Do(sReq)
		if sDoErr == nil {
			defer sResp.Body.Close()
			if sResp.StatusCode == http.StatusOK {
				var sData struct {
					Results []struct {
						ResultGlobalContainer struct {
							Title      string `json:"title"`
							DisplayURL string `json:"displayUrl"`
						} `json:"resultGlobalContainer"`
					} `json:"results"`
				}
				if json.NewDecoder(sResp.Body).Decode(&sData) == nil && len(sData.Results) > 0 {
					seen := make(map[string]bool)
					spaces := make([]AtlassianSpace, 0)
					for _, r := range sData.Results {
						disp := r.ResultGlobalContainer.DisplayURL
						if strings.HasPrefix(disp, "/spaces/") {
							key := strings.TrimPrefix(disp, "/spaces/")
							if !seen[key] && !strings.HasPrefix(key, "~") && key != "" {
								seen[key] = true
								name := r.ResultGlobalContainer.Title
								if name == "" {
									name = key
								}
								spaces = append(spaces, AtlassianSpace{
									Key:  key,
									Name: name,
									Type: "global",
								})
							}
						}
					}
					if len(spaces) > 0 {
						return spaces, nil
					}
				}
			}
		}
	}

	// APIs failed / returned 401 - return empty gracefully so manual space key input activates
	return []AtlassianSpace{}, nil
}

func (s *AtlassianService) CheckSpaceAccess(ctx context.Context, userID, spaceKey string) (bool, error) {
	spaces, err := s.ListSpaces(ctx, userID)
	if err == nil && len(spaces) > 0 {
		for _, sp := range spaces {
			if strings.EqualFold(sp.Key, spaceKey) {
				return true, nil
			}
		}
		// Got a non-empty list but space wasn't found
		return false, nil
	}
	// ListSpaces returned empty (401 scope mismatch) or error — cannot enumerate spaces.
	// Allow the user to proceed; actual write will fail with a clear error if the space is inaccessible.
	return true, nil
}

func (s *AtlassianService) ListSpacePages(ctx context.Context, userID, spaceKey string) ([]AtlassianPage, error) {
	if s.IsMockMode() {
		return []AtlassianPage{
			{ID: "mock-page-1", Title: "Architecture & Standards"},
			{ID: "mock-page-2", Title: "Product Specifications"},
			{ID: "mock-page-3", Title: "Operations & Runbooks"},
			{ID: "mock-page-4", Title: "Engineering Guides"},
		}, nil
	}

	conn, err := s.repo.GetAtlassianConnectionByUserID(userID)
	if err != nil || conn == nil {
		return nil, fmt.Errorf("no atlassian connection found for user")
	}

	if conn.CloudID == "mock-cloud-id-123" {
		return []AtlassianPage{
			{ID: "mock-page-1", Title: "Architecture & Standards"},
			{ID: "mock-page-2", Title: "Product Specifications"},
			{ID: "mock-page-3", Title: "Operations & Runbooks"},
			{ID: "mock-page-4", Title: "Engineering Guides"},
		}, nil
	}

	if strings.TrimSpace(conn.CloudID) == "" {
		return nil, fmt.Errorf("no Confluence Cloud site is linked to your Atlassian session")
	}

	accessToken, err := s.getValidAccessToken(ctx, conn)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Try CQL Content Search: GET /wiki/rest/api/content/search?cql=space = %q AND type = page (works with search:confluence)
	cqlQuery := fmt.Sprintf(`space = %q AND type = page`, spaceKey)
	cqlURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/search?cql=%s&limit=50", conn.CloudID, url.QueryEscape(cqlQuery))
	cReq, cErr := http.NewRequestWithContext(ctx, "GET", cqlURL, nil)
	if cErr == nil {
		cReq.Header.Set("Authorization", "Bearer "+accessToken)
		cReq.Header.Set("Accept", "application/json")
		cResp, cDoErr := client.Do(cReq)
		if cDoErr == nil {
			defer cResp.Body.Close()
			if cResp.StatusCode == http.StatusOK {
				var data struct {
					Results []struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"results"`
				}
				if json.NewDecoder(cResp.Body).Decode(&data) == nil && len(data.Results) > 0 {
					pages := make([]AtlassianPage, 0, len(data.Results))
					for _, p := range data.Results {
						pages = append(pages, AtlassianPage{ID: p.ID, Title: p.Title})
					}
					return pages, nil
				}
			}
		}
	}

	// 2. Try v1 API: GET /wiki/rest/api/content?spaceKey=...&type=page (works with read:confluence-content.all)
	v1URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content?spaceKey=%s&type=page&limit=50", conn.CloudID, url.QueryEscape(spaceKey))
	fReq, fErr := http.NewRequestWithContext(ctx, "GET", v1URL, nil)
	if fErr == nil {
		fReq.Header.Set("Authorization", "Bearer "+accessToken)
		fReq.Header.Set("Accept", "application/json")
		fResp, fDoErr := client.Do(fReq)
		if fDoErr == nil {
			defer fResp.Body.Close()
			if fResp.StatusCode == http.StatusOK {
				var data struct {
					Results []struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"results"`
				}
				if json.NewDecoder(fResp.Body).Decode(&data) == nil {
					pages := make([]AtlassianPage, 0, len(data.Results))
					for _, p := range data.Results {
						pages = append(pages, AtlassianPage{ID: p.ID, Title: p.Title})
					}
					return pages, nil
				}
			}
			// 401/410: fall through to v2
		}
	}

	// 2. Try v2 API: GET /wiki/api/v2/spaces/{id}/pages (requires read:page:confluence)
	// Resolve space ID first
	spaceID := spaceKey
	sReq, sErr := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces?keys=%s", conn.CloudID, url.QueryEscape(spaceKey)), nil)
	if sErr == nil {
		sReq.Header.Set("Authorization", "Bearer "+accessToken)
		sReq.Header.Set("Accept", "application/json")
		if sResp, sDoErr := client.Do(sReq); sDoErr == nil {
			defer sResp.Body.Close()
			if sResp.StatusCode == http.StatusOK {
				var sData struct {
					Results []struct {
						ID string `json:"id"`
					} `json:"results"`
				}
				if json.NewDecoder(sResp.Body).Decode(&sData) == nil && len(sData.Results) > 0 {
					spaceID = sData.Results[0].ID
				}
			}
		}
	}

	pagesURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces/%s/pages?limit=50", conn.CloudID, url.PathEscape(spaceID))
	pReq, err := http.NewRequestWithContext(ctx, "GET", pagesURL, nil)
	if err == nil {
		pReq.Header.Set("Authorization", "Bearer "+accessToken)
		pReq.Header.Set("Accept", "application/json")
		pResp, pDoErr := client.Do(pReq)
		if pDoErr == nil {
			defer pResp.Body.Close()
			if pResp.StatusCode == http.StatusOK {
				var pData struct {
					Results []struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"results"`
				}
				if json.NewDecoder(pResp.Body).Decode(&pData) == nil {
					pages := make([]AtlassianPage, 0, len(pData.Results))
					for _, p := range pData.Results {
						pages = append(pages, AtlassianPage{ID: p.ID, Title: p.Title})
					}
					return pages, nil
				}
			}
		}
	}

	return []AtlassianPage{}, nil
}

func (s *AtlassianService) PublishDocument(ctx context.Context, userID, projectID, documentID, overrideSpaceKey, overrideParentPageID string) (string, error) {
	doc, err := s.repo.GetDocumentByID(documentID)
	if err != nil {
		return "", fmt.Errorf("document not found: %w", err)
	}

	spaceKey := overrideSpaceKey
	parentPageID := overrideParentPageID

	if spaceKey == "" {
		target, err := s.repo.GetConfluenceTargetByProject(projectID)
		if err != nil || target == nil {
			return "", fmt.Errorf("no confluence target configured for project")
		}
		spaceKey = target.SpaceKey
		parentPageID = target.ParentPageID
	}

	// Verify that user has access to this space
	hasAccess, err := s.CheckSpaceAccess(ctx, userID, spaceKey)
	if err == nil && !hasAccess {
		return "", fmt.Errorf("access denied: user does not have permission to publish to Confluence space '%s'", spaceKey)
	}

	conn, err := s.repo.GetAtlassianConnectionByUserID(userID)
	if err != nil || conn == nil {
		if !s.IsMockMode() {
			return "", fmt.Errorf("no active atlassian connection for user")
		}
	}

	if s.IsMockMode() || (conn != nil && conn.CloudID == "mock-cloud-id-123") {
		pageID := fmt.Sprintf("conf_%d", time.Now().Unix())
		_ = s.repo.UpdateDocumentConfluencePage(documentID, pageID)
		return pageID, nil
	}

	if strings.TrimSpace(conn.CloudID) == "" {
		return "", fmt.Errorf("no Confluence Cloud site is linked to your Atlassian session")
	}

	accessToken, err := s.getValidAccessToken(ctx, conn)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Build Confluence storage format body from markdown content
	storageBody := fmt.Sprintf("<pre>%s</pre>", html.EscapeString(doc.ContentMD))

	// 1. Try Confluence REST API v2: POST /wiki/api/v2/pages (Standard modern Cloud API)
	// First resolve numeric spaceId
	spaceID := spaceKey
	sReq, sErr := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces?keys=%s", conn.CloudID, url.QueryEscape(spaceKey)), nil)
	if sErr == nil {
		sReq.Header.Set("Authorization", "Bearer "+accessToken)
		sReq.Header.Set("Accept", "application/json")
		if sResp, sDoErr := client.Do(sReq); sDoErr == nil {
			defer sResp.Body.Close()
			if sResp.StatusCode == http.StatusOK {
				var sData struct {
					Results []struct {
						ID string `json:"id"`
					} `json:"results"`
				}
				if json.NewDecoder(sResp.Body).Decode(&sData) == nil && len(sData.Results) > 0 {
					spaceID = sData.Results[0].ID
				}
			}
		}
	}

	if spaceID == spaceKey {
		// Resolve numeric space ID via CQL search (works with search:confluence)
		cqlQ := fmt.Sprintf(`space = %q AND type = page`, spaceKey)
		cqlSearchURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/search?cql=%s&expand=space&limit=1", conn.CloudID, url.QueryEscape(cqlQ))
		if csReq, csErr := http.NewRequestWithContext(ctx, "GET", cqlSearchURL, nil); csErr == nil {
			csReq.Header.Set("Authorization", "Bearer "+accessToken)
			csReq.Header.Set("Accept", "application/json")
			if csResp, csDoErr := client.Do(csReq); csDoErr == nil {
				defer csResp.Body.Close()
				if csResp.StatusCode == http.StatusOK {
					var csData struct {
						Results []struct {
							Space struct {
								ID int64 `json:"id"`
							} `json:"space"`
						} `json:"results"`
					}
					if json.NewDecoder(csResp.Body).Decode(&csData) == nil && len(csData.Results) > 0 && csData.Results[0].Space.ID > 0 {
						spaceID = fmt.Sprintf("%d", csData.Results[0].Space.ID)
					}
				}
			}
		}
	}

	if spaceID == spaceKey {
		// Resolve numeric space ID via v1 space endpoint (works with read:confluence-space.summary)
		v1sURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/space/%s", conn.CloudID, url.PathEscape(spaceKey))
		if sReq1, sErr1 := http.NewRequestWithContext(ctx, "GET", v1sURL, nil); sErr1 == nil {
			sReq1.Header.Set("Authorization", "Bearer "+accessToken)
			sReq1.Header.Set("Accept", "application/json")
			if sResp1, sDoErr1 := client.Do(sReq1); sDoErr1 == nil {
				defer sResp1.Body.Close()
				if sResp1.StatusCode == http.StatusOK {
					var s1Data struct {
						ID int64 `json:"id"`
					}
					if json.NewDecoder(sResp1.Body).Decode(&s1Data) == nil && s1Data.ID > 0 {
						spaceID = fmt.Sprintf("%d", s1Data.ID)
					}
				}
			}
		}
	}

	// If spaceID is still unresolved, try resolving via parentPageID
	if spaceID == spaceKey && parentPageID != "" {
		// Try v2 page lookup
		pURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/pages/%s", conn.CloudID, url.PathEscape(parentPageID))
		if cpReq, cpErr := http.NewRequestWithContext(ctx, "GET", pURL, nil); cpErr == nil {
			cpReq.Header.Set("Authorization", "Bearer "+accessToken)
			cpReq.Header.Set("Accept", "application/json")
			if cpResp, cpDoErr := client.Do(cpReq); cpDoErr == nil {
				defer cpResp.Body.Close()
				if cpResp.StatusCode == http.StatusOK {
					var pData struct {
						SpaceID string `json:"spaceId"`
					}
					if json.NewDecoder(cpResp.Body).Decode(&pData) == nil && pData.SpaceID != "" {
						spaceID = pData.SpaceID
					}
				}
			}
		}
		// Fallback: Try content search expand=space
		if spaceID == spaceKey {
			cURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/%s?expand=space", conn.CloudID, url.PathEscape(parentPageID))
			if cpReq, cpErr := http.NewRequestWithContext(ctx, "GET", cURL, nil); cpErr == nil {
				cpReq.Header.Set("Authorization", "Bearer "+accessToken)
				cpReq.Header.Set("Accept", "application/json")
				if cpResp, cpDoErr := client.Do(cpReq); cpDoErr == nil {
					defer cpResp.Body.Close()
					if cpResp.StatusCode == http.StatusOK {
						var cpData struct {
							Space struct {
								ID int64 `json:"id"`
							} `json:"space"`
						}
						if json.NewDecoder(cpResp.Body).Decode(&cpData) == nil && cpData.Space.ID > 0 {
							spaceID = fmt.Sprintf("%d", cpData.Space.ID)
						}
					}
				}
			}
		}
	}

	// 1. Try Confluence REST API v2: POST /wiki/api/v2/pages (Standard modern Cloud API, requires numeric spaceId and write:page:confluence)
	var v2StatusCode int
	var v2ErrBody string
	if _, numErr := strconv.ParseInt(spaceID, 10, 64); numErr == nil {
		v2Payload := map[string]any{
			"spaceId": spaceID,
			"status":  "current",
			"title":   doc.Title,
			"body": map[string]any{
				"representation": "storage",
				"value":          storageBody,
			},
		}
		if parentPageID != "" {
			v2Payload["parentId"] = parentPageID
		}

		v2Bytes, _ := json.Marshal(v2Payload)
		v2URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/pages", conn.CloudID)
		if req2, reqErr2 := http.NewRequestWithContext(ctx, "POST", v2URL, bytes.NewReader(v2Bytes)); reqErr2 == nil {
			req2.Header.Set("Authorization", "Bearer "+accessToken)
			req2.Header.Set("Content-Type", "application/json")
			req2.Header.Set("Accept", "application/json")
			if resp2, doErr2 := client.Do(req2); doErr2 == nil {
				defer resp2.Body.Close()
				v2StatusCode = resp2.StatusCode
				b2, _ := io.ReadAll(resp2.Body)
				v2ErrBody = string(b2)
				if resp2.StatusCode < 300 {
					var res2 struct {
						ID string `json:"id"`
					}
					if json.Unmarshal(b2, &res2) == nil && res2.ID != "" {
						_ = s.repo.UpdateDocumentConfluencePage(documentID, res2.ID)
						return res2.ID, nil
					}
				}
			}
		}
	}

	// 2. Fallback: Try Confluence REST API v1: POST /wiki/rest/api/content (for apps with classic scope write:confluence-content or Server/DC)
	var v1StatusCode int
	var v1ErrBody string
	v1Payload := map[string]any{
		"type":  "page",
		"title": doc.Title,
		"space": map[string]string{"key": spaceKey},
		"body": map[string]any{
			"storage": map[string]string{
				"value":          storageBody,
				"representation": "storage",
			},
		},
	}
	if parentPageID != "" {
		v1Payload["ancestors"] = []map[string]string{{"id": parentPageID}}
	}
	v1Bytes, _ := json.Marshal(v1Payload)
	v1URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content", conn.CloudID)
	if req1, reqErr1 := http.NewRequestWithContext(ctx, "POST", v1URL, bytes.NewReader(v1Bytes)); reqErr1 == nil {
		req1.Header.Set("Authorization", "Bearer "+accessToken)
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Accept", "application/json")
		if resp1, doErr1 := client.Do(req1); doErr1 == nil {
			defer resp1.Body.Close()
			v1StatusCode = resp1.StatusCode
			b1, _ := io.ReadAll(resp1.Body)
			v1ErrBody = string(b1)
			if resp1.StatusCode < 300 {
				var res1 struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(b1, &res1) == nil && res1.ID != "" {
					_ = s.repo.UpdateDocumentConfluencePage(documentID, res1.ID)
					return res1.ID, nil
				}
			}
		}
	}

	// 3. Handle specific common errors with actionable guidance
	if v2StatusCode == http.StatusConflict || v1StatusCode == http.StatusConflict {
		return "", fmt.Errorf("confluence publish error (409 Conflict): a page titled %q already exists in space %q. Please choose a different title or delete the existing page in Confluence", doc.Title, spaceKey)
	}
	if v2StatusCode == http.StatusForbidden || v1StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("confluence publish error (403 Forbidden): your Atlassian user account does not have permission to add pages in space %q. Please check your Confluence space permissions", spaceKey)
	}
	if (v1StatusCode == http.StatusGone || v1StatusCode == 0) && (v2StatusCode == http.StatusUnauthorized || v2StatusCode == 0) {
		return "", fmt.Errorf("confluence publish error: Atlassian Cloud has permanently retired the legacy v1 content creation API (HTTP 410 Gone) and strictly requires the granular scope 'write:page:confluence' to create pages via the modern v2 API. Please enable 'write:page:confluence' (and 'read:page:confluence', 'read:space:confluence') under Permissions -> Confluence API -> Granular scopes in your app at developer.atlassian.com/console/myapps, then reconnect Atlassian in DocForge")
	}

	if v2StatusCode > 0 {
		return "", fmt.Errorf("confluence publish error (%d): %s", v2StatusCode, strings.TrimSpace(v2ErrBody))
	}
	if v1StatusCode > 0 {
		return "", fmt.Errorf("confluence publish error (%d): %s", v1StatusCode, strings.TrimSpace(v1ErrBody))
	}

	return "", fmt.Errorf("confluence publish failed: could not connect to Confluence Cloud API")
}


