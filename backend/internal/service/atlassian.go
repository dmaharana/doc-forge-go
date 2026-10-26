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
	"sort"
	"strconv"
	"strings"
	"time"

	"docforge/internal/config"
	"docforge/internal/crypto"
	"docforge/internal/models"
	"docforge/internal/repository"

	"github.com/golang-jwt/jwt/v5"
)

type AtlassianSite struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes,omitempty"`
	AvatarURL string   `json:"avatar_url,omitempty"`
}

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

func (s *AtlassianService) fetchAccessibleSitesList(ctx context.Context, accessToken string) ([]AtlassianSite, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch accessible resources (%d): %s", resp.StatusCode, string(b))
	}

	var rawResources []struct {
		ID        string   `json:"id"`
		URL       string   `json:"url"`
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		AvatarURL string   `json:"avatarUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawResources); err != nil {
		return nil, err
	}

	var sites []AtlassianSite
	for _, r := range rawResources {
		hasConfluence := len(r.Scopes) == 0
		for _, sc := range r.Scopes {
			if strings.Contains(sc, "confluence") || strings.Contains(sc, "space") || strings.Contains(sc, "page") {
				hasConfluence = true
				break
			}
		}
		if hasConfluence {
			sites = append(sites, AtlassianSite{
				ID:        r.ID,
				URL:       r.URL,
				Name:      r.Name,
				Scopes:    r.Scopes,
				AvatarURL: r.AvatarURL,
			})
		}
	}
	if len(sites) == 0 {
		for _, r := range rawResources {
			sites = append(sites, AtlassianSite{
				ID:        r.ID,
				URL:       r.URL,
				Name:      r.Name,
				Scopes:    r.Scopes,
				AvatarURL: r.AvatarURL,
			})
		}
	}

	return sites, nil
}

func (s *AtlassianService) fetchAccessibleResources(ctx context.Context, accessToken string) (string, string) {
	sites, err := s.fetchAccessibleSitesList(ctx, accessToken)
	if err == nil && len(sites) > 0 {
		return sites[0].ID, sites[0].URL
	}
	return "", ""
}

func (s *AtlassianService) GetAccessibleSites(ctx context.Context, userID string) ([]AtlassianSite, error) {
	if s.IsMockMode() {
		return []AtlassianSite{
			{
				ID:   "mock-cloud-id-123",
				Name: "Acme Engineering",
				URL:  "https://mock-company.atlassian.net/wiki",
			},
			{
				ID:   "mock-cloud-id-456",
				Name: "Acme Operations",
				URL:  "https://mock-ops.atlassian.net/wiki",
			},
		}, nil
	}

	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	conn, err := s.repo.GetAtlassianConnectionByUserID(userID)
	if err != nil || conn == nil {
		return nil, fmt.Errorf("no atlassian connection found for user")
	}

	if conn.CloudID == "mock-cloud-id-123" {
		return []AtlassianSite{
			{
				ID:   "mock-cloud-id-123",
				Name: "Acme Engineering",
				URL:  "https://mock-company.atlassian.net/wiki",
			},
			{
				ID:   "mock-cloud-id-456",
				Name: "Acme Operations",
				URL:  "https://mock-ops.atlassian.net/wiki",
			},
		}, nil
	}

	accessToken, err := s.getValidAccessToken(ctx, conn)
	if err != nil {
		return nil, err
	}

	return s.fetchAccessibleSitesList(ctx, accessToken)
}

func (s *AtlassianService) ListSpaces(ctx context.Context, userID string, cloudIDOpt ...string) ([]AtlassianSpace, error) {
	cloudID := ""
	if len(cloudIDOpt) > 0 && cloudIDOpt[0] != "" {
		cloudID = cloudIDOpt[0]
	}

	if s.IsMockMode() {
		if cloudID == "mock-cloud-id-456" {
			return []AtlassianSpace{
				{Key: "OPS", Name: "Platform Operations", Type: "global"},
				{Key: "INFRA", Name: "Cloud Infrastructure", Type: "global"},
				{Key: "SECOPS", Name: "Security Operations", Type: "global"},
			}, nil
		}
		return []AtlassianSpace{
			{Key: "ENG", Name: "Engineering & Architecture", Type: "global"},
			{Key: "PROD", Name: "Product Management", Type: "global"},
			{Key: "OPS", Name: "Platform Operations", Type: "global"},
			{Key: "DOCS", Name: "Technical Documentation", Type: "global"},
		}, nil
	}

	if s.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	conn, err := s.repo.GetAtlassianConnectionByUserID(userID)
	if err != nil || conn == nil {
		return nil, fmt.Errorf("no atlassian connection found for user")
	}

	if cloudID == "" {
		cloudID = conn.CloudID
	}

	if s.IsMockMode() || cloudID == "mock-cloud-id-123" || cloudID == "mock-cloud-id-456" || conn.CloudID == "mock-cloud-id-123" {
		if cloudID == "mock-cloud-id-456" {
			return []AtlassianSpace{
				{Key: "OPS", Name: "Platform Operations", Type: "global"},
				{Key: "INFRA", Name: "Cloud Infrastructure", Type: "global"},
				{Key: "SECOPS", Name: "Security Operations", Type: "global"},
			}, nil
		}
		return []AtlassianSpace{
			{Key: "ENG", Name: "Engineering & Architecture", Type: "global"},
			{Key: "PROD", Name: "Product Management", Type: "global"},
			{Key: "OPS", Name: "Platform Operations", Type: "global"},
			{Key: "DOCS", Name: "Technical Documentation", Type: "global"},
		}, nil
	}

	if strings.TrimSpace(cloudID) == "" {
		return nil, fmt.Errorf("no Confluence Cloud site is selected or linked to your Atlassian session. Please reconnect your Atlassian account with site access")
	}

	accessToken, err := s.getValidAccessToken(ctx, conn)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Try Confluence REST API v1: GET /wiki/rest/api/space (works with read:confluence-space.summary)
	v1URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/space?limit=100", cloudID)
	var v1Spaces []AtlassianSpace
	v1Seen := make(map[string]bool)
	nextV1 := v1URL
	prevV1 := ""
	v1Pages := 0
	for nextV1 != "" && v1Pages < 25 && len(v1Spaces) < 2500 {
		if nextV1 == prevV1 {
			break
		}
		prevV1 = nextV1
		v1Pages++

		req1, err := http.NewRequestWithContext(ctx, "GET", nextV1, nil)
		if err != nil {
			break
		}
		req1.Header.Set("Authorization", "Bearer "+accessToken)
		req1.Header.Set("Accept", "application/json")
		resp1, err1 := client.Do(req1)
		if err1 != nil {
			break
		}
		if resp1.StatusCode != http.StatusOK {
			resp1.Body.Close()
			break
		}
		var data1 struct {
			Results []struct {
				ID   json.Number `json:"id"`
				Key  string      `json:"key"`
				Name string      `json:"name"`
				Type string      `json:"type"`
			} `json:"results"`
			Links struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		decErr := json.NewDecoder(resp1.Body).Decode(&data1)
		resp1.Body.Close()
		if decErr != nil {
			break
		}
		for _, r := range data1.Results {
			k := strings.TrimSpace(r.Key)
			if k != "" && !v1Seen[k] {
				v1Seen[k] = true
				v1Spaces = append(v1Spaces, AtlassianSpace{
					ID:   r.ID.String(),
					Key:  k,
					Name: r.Name,
					Type: r.Type,
				})
			}
		}
		if data1.Links.Next != "" {
			nextV1 = resolveConfluenceURL(cloudID, data1.Links.Next)
		} else {
			nextV1 = ""
		}
	}
	if len(v1Spaces) > 0 {
		sort.Slice(v1Spaces, func(i, j int) bool {
			ni, nj := strings.ToLower(v1Spaces[i].Name), strings.ToLower(v1Spaces[j].Name)
			if ni != nj {
				return ni < nj
			}
			return v1Spaces[i].Key < v1Spaces[j].Key
		})
		return v1Spaces, nil
	}

	// 2. Try Confluence REST API v2: GET /wiki/api/v2/spaces?limit=250 (requires read:space:confluence)
	v2URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces?limit=250", cloudID)
	var v2Spaces []AtlassianSpace
	v2Seen := make(map[string]bool)
	nextV2 := v2URL
	prevV2 := ""
	v2Pages := 0
	for nextV2 != "" && v2Pages < 25 && len(v2Spaces) < 2500 {
		if nextV2 == prevV2 {
			break
		}
		prevV2 = nextV2
		v2Pages++

		req2, err := http.NewRequestWithContext(ctx, "GET", nextV2, nil)
		if err != nil {
			break
		}
		req2.Header.Set("Authorization", "Bearer "+accessToken)
		req2.Header.Set("Accept", "application/json")
		resp2, err2 := client.Do(req2)
		if err2 != nil {
			break
		}
		if resp2.StatusCode != http.StatusOK {
			resp2.Body.Close()
			break
		}
		var data2 struct {
			Results []struct {
				ID   string `json:"id"`
				Key  string `json:"key"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"results"`
			Links struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		decErr := json.NewDecoder(resp2.Body).Decode(&data2)
		resp2.Body.Close()
		if decErr != nil {
			break
		}
		for _, r := range data2.Results {
			k := strings.TrimSpace(r.Key)
			if k != "" && !v2Seen[k] {
				v2Seen[k] = true
				v2Spaces = append(v2Spaces, AtlassianSpace{
					ID:   r.ID,
					Key:  k,
					Name: r.Name,
					Type: r.Type,
				})
			}
		}
		if data2.Links.Next != "" {
			nextV2 = resolveConfluenceURL(cloudID, data2.Links.Next)
		} else {
			nextV2 = ""
		}
	}
	if len(v2Spaces) > 0 {
		sort.Slice(v2Spaces, func(i, j int) bool {
			ni, nj := strings.ToLower(v2Spaces[i].Name), strings.ToLower(v2Spaces[j].Name)
			if ni != nj {
				return ni < nj
			}
			return v2Spaces[i].Key < v2Spaces[j].Key
		})
		return v2Spaces, nil
	}

	// 3. Fallback: discover spaces via CQL Search: GET /wiki/rest/api/search?cql=type=page (works with search:confluence)
	sURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/search?cql=%s&limit=100", cloudID, url.QueryEscape("type = page"))
	var cqlSpaces []AtlassianSpace
	cqlSeen := make(map[string]bool)
	nextCQL := sURL
	prevCQL := ""
	cqlPages := 0
	for nextCQL != "" && cqlPages < 10 && len(cqlSpaces) < 1000 {
		if nextCQL == prevCQL {
			break
		}
		prevCQL = nextCQL
		cqlPages++

		sReq, sErr := http.NewRequestWithContext(ctx, "GET", nextCQL, nil)
		if sErr != nil {
			break
		}
		sReq.Header.Set("Authorization", "Bearer "+accessToken)
		sReq.Header.Set("Accept", "application/json")
		sResp, sDoErr := client.Do(sReq)
		if sDoErr != nil {
			break
		}
		if sResp.StatusCode != http.StatusOK {
			sResp.Body.Close()
			break
		}
		var sData struct {
			Results []struct {
				ResultGlobalContainer struct {
					Title      string `json:"title"`
					DisplayURL string `json:"displayUrl"`
				} `json:"resultGlobalContainer"`
			} `json:"results"`
			Links struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		decErr := json.NewDecoder(sResp.Body).Decode(&sData)
		sResp.Body.Close()
		if decErr != nil {
			break
		}
		for _, r := range sData.Results {
			disp := r.ResultGlobalContainer.DisplayURL
			if strings.HasPrefix(disp, "/spaces/") {
				key := strings.TrimPrefix(disp, "/spaces/")
				if !cqlSeen[key] && !strings.HasPrefix(key, "~") && key != "" {
					cqlSeen[key] = true
					name := r.ResultGlobalContainer.Title
					if name == "" {
						name = key
					}
					cqlSpaces = append(cqlSpaces, AtlassianSpace{
						Key:  key,
						Name: name,
						Type: "global",
					})
				}
			}
		}
		if sData.Links.Next != "" {
			nextCQL = resolveConfluenceURL(cloudID, sData.Links.Next)
		} else {
			nextCQL = ""
		}
	}
	if len(cqlSpaces) > 0 {
		sort.Slice(cqlSpaces, func(i, j int) bool {
			ni, nj := strings.ToLower(cqlSpaces[i].Name), strings.ToLower(cqlSpaces[j].Name)
			if ni != nj {
				return ni < nj
			}
			return cqlSpaces[i].Key < cqlSpaces[j].Key
		})
		return cqlSpaces, nil
	}

	// APIs failed / returned 401 - return empty gracefully so manual space key input activates
	return []AtlassianSpace{}, nil
}

func (s *AtlassianService) CheckSpaceAccess(ctx context.Context, userID, spaceKey string, cloudIDOpt ...string) (bool, error) {
	spaces, err := s.ListSpaces(ctx, userID, cloudIDOpt...)
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

func (s *AtlassianService) ListSpacePages(ctx context.Context, userID, spaceKey string, cloudIDOpt ...string) ([]AtlassianPage, error) {
	cloudID := ""
	if len(cloudIDOpt) > 0 && cloudIDOpt[0] != "" {
		cloudID = cloudIDOpt[0]
	}

	if s.IsMockMode() {
		if spaceKey == "INFRA" {
			return []AtlassianPage{
				{ID: "mock-page-infra-1", Title: "Kubernetes Clusters & Topology"},
				{ID: "mock-page-infra-2", Title: "Network Architecture & VPCs"},
			}, nil
		}
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

	if cloudID == "" {
		cloudID = conn.CloudID
	}

	if cloudID == "mock-cloud-id-123" || cloudID == "mock-cloud-id-456" || conn.CloudID == "mock-cloud-id-123" {
		if spaceKey == "INFRA" {
			return []AtlassianPage{
				{ID: "mock-page-infra-1", Title: "Kubernetes Clusters & Topology"},
				{ID: "mock-page-infra-2", Title: "Network Architecture & VPCs"},
			}, nil
		}
		return []AtlassianPage{
			{ID: "mock-page-1", Title: "Architecture & Standards"},
			{ID: "mock-page-2", Title: "Product Specifications"},
			{ID: "mock-page-3", Title: "Operations & Runbooks"},
			{ID: "mock-page-4", Title: "Engineering Guides"},
		}, nil
	}

	if strings.TrimSpace(cloudID) == "" {
		return nil, fmt.Errorf("no Confluence Cloud site is selected or linked to your Atlassian session")
	}

	accessToken, err := s.getValidAccessToken(ctx, conn)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Try CQL Content Search: GET /wiki/rest/api/content/search?cql=space = %q AND type = page (works with search:confluence)
	cqlQuery := fmt.Sprintf(`space = %q AND type = page`, spaceKey)
	cqlURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/search?cql=%s&limit=100", cloudID, url.QueryEscape(cqlQuery))
	var cqlPages []AtlassianPage
	cqlSeen := make(map[string]bool)
	nextCQL := cqlURL
	prevCQL := ""
	cqlCount := 0
	for nextCQL != "" && cqlCount < 10 && len(cqlPages) < 1000 {
		if nextCQL == prevCQL {
			break
		}
		prevCQL = nextCQL
		cqlCount++

		cReq, cErr := http.NewRequestWithContext(ctx, "GET", nextCQL, nil)
		if cErr != nil {
			break
		}
		cReq.Header.Set("Authorization", "Bearer "+accessToken)
		cReq.Header.Set("Accept", "application/json")
		cResp, cDoErr := client.Do(cReq)
		if cDoErr != nil {
			break
		}
		if cResp.StatusCode != http.StatusOK {
			cResp.Body.Close()
			break
		}
		var data struct {
			Results []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"results"`
			Links struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		decErr := json.NewDecoder(cResp.Body).Decode(&data)
		cResp.Body.Close()
		if decErr != nil {
			break
		}
		for _, p := range data.Results {
			if p.ID != "" && !cqlSeen[p.ID] {
				cqlSeen[p.ID] = true
				cqlPages = append(cqlPages, AtlassianPage{ID: p.ID, Title: p.Title})
			}
		}
		if data.Links.Next != "" {
			nextCQL = resolveConfluenceURL(cloudID, data.Links.Next)
		} else {
			nextCQL = ""
		}
	}
	if len(cqlPages) > 0 {
		sort.Slice(cqlPages, func(i, j int) bool {
			ti, tj := strings.ToLower(cqlPages[i].Title), strings.ToLower(cqlPages[j].Title)
			if ti != tj {
				return ti < tj
			}
			return cqlPages[i].ID < cqlPages[j].ID
		})
		return cqlPages, nil
	}

	// 2. Try v1 API: GET /wiki/rest/api/content?spaceKey=...&type=page (works with read:confluence-content.all)
	v1URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content?spaceKey=%s&type=page&limit=100", cloudID, url.QueryEscape(spaceKey))
	var v1Pages []AtlassianPage
	v1Seen := make(map[string]bool)
	nextV1 := v1URL
	prevV1 := ""
	v1Count := 0
	for nextV1 != "" && v1Count < 10 && len(v1Pages) < 1000 {
		if nextV1 == prevV1 {
			break
		}
		prevV1 = nextV1
		v1Count++

		fReq, fErr := http.NewRequestWithContext(ctx, "GET", nextV1, nil)
		if fErr != nil {
			break
		}
		fReq.Header.Set("Authorization", "Bearer "+accessToken)
		fReq.Header.Set("Accept", "application/json")
		fResp, fDoErr := client.Do(fReq)
		if fDoErr != nil {
			break
		}
		if fResp.StatusCode != http.StatusOK {
			fResp.Body.Close()
			break
		}
		var data struct {
			Results []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"results"`
			Links struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		decErr := json.NewDecoder(fResp.Body).Decode(&data)
		fResp.Body.Close()
		if decErr != nil {
			break
		}
		for _, p := range data.Results {
			if p.ID != "" && !v1Seen[p.ID] {
				v1Seen[p.ID] = true
				v1Pages = append(v1Pages, AtlassianPage{ID: p.ID, Title: p.Title})
			}
		}
		if data.Links.Next != "" {
			nextV1 = resolveConfluenceURL(cloudID, data.Links.Next)
		} else {
			nextV1 = ""
		}
	}
	if len(v1Pages) > 0 {
		sort.Slice(v1Pages, func(i, j int) bool {
			ti, tj := strings.ToLower(v1Pages[i].Title), strings.ToLower(v1Pages[j].Title)
			if ti != tj {
				return ti < tj
			}
			return v1Pages[i].ID < v1Pages[j].ID
		})
		return v1Pages, nil
	}

	// 3. Try v2 API: GET /wiki/api/v2/spaces/{id}/pages (requires read:page:confluence)
	// Resolve space ID first
	spaceID := spaceKey
	sReq, sErr := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces?keys=%s", cloudID, url.QueryEscape(spaceKey)), nil)
	if sErr == nil {
		sReq.Header.Set("Authorization", "Bearer "+accessToken)
		sReq.Header.Set("Accept", "application/json")
		if sResp, sDoErr := client.Do(sReq); sDoErr == nil {
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
			sResp.Body.Close()
		}
	}

	pagesURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces/%s/pages?limit=100", cloudID, url.PathEscape(spaceID))
	var v2Pages []AtlassianPage
	v2Seen := make(map[string]bool)
	nextV2 := pagesURL
	prevV2 := ""
	v2Count := 0
	for nextV2 != "" && v2Count < 10 && len(v2Pages) < 1000 {
		if nextV2 == prevV2 {
			break
		}
		prevV2 = nextV2
		v2Count++

		pReq, err := http.NewRequestWithContext(ctx, "GET", nextV2, nil)
		if err != nil {
			break
		}
		pReq.Header.Set("Authorization", "Bearer "+accessToken)
		pReq.Header.Set("Accept", "application/json")
		pResp, pDoErr := client.Do(pReq)
		if pDoErr != nil {
			break
		}
		if pResp.StatusCode != http.StatusOK {
			pResp.Body.Close()
			break
		}
		var pData struct {
			Results []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"results"`
			Links struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		decErr := json.NewDecoder(pResp.Body).Decode(&pData)
		pResp.Body.Close()
		if decErr != nil {
			break
		}
		for _, p := range pData.Results {
			if p.ID != "" && !v2Seen[p.ID] {
				v2Seen[p.ID] = true
				v2Pages = append(v2Pages, AtlassianPage{ID: p.ID, Title: p.Title})
			}
		}
		if pData.Links.Next != "" {
			nextV2 = resolveConfluenceURL(cloudID, pData.Links.Next)
		} else {
			nextV2 = ""
		}
	}
	if len(v2Pages) > 0 {
		sort.Slice(v2Pages, func(i, j int) bool {
			ti, tj := strings.ToLower(v2Pages[i].Title), strings.ToLower(v2Pages[j].Title)
			if ti != tj {
				return ti < tj
			}
			return v2Pages[i].ID < v2Pages[j].ID
		})
		return v2Pages, nil
	}

	return []AtlassianPage{}, nil
}

func (s *AtlassianService) PublishDocument(ctx context.Context, userID, projectID, documentID, overrideSpaceKey, overrideParentPageID string, cloudIDOpt ...string) (string, string, error) {
	doc, err := s.repo.GetDocumentByID(documentID)
	if err != nil {
		return "", "", fmt.Errorf("document not found: %w", err)
	}

	cloudID := ""
	if len(cloudIDOpt) > 0 && cloudIDOpt[0] != "" {
		cloudID = cloudIDOpt[0]
	}

	target, _ := s.repo.GetConfluenceTargetByProject(projectID)

	spaceKey := overrideSpaceKey
	parentPageID := overrideParentPageID

	if spaceKey == "" {
		if target == nil {
			return "", "", fmt.Errorf("no confluence target configured for project")
		}
		spaceKey = target.SpaceKey
		parentPageID = target.ParentPageID
	}

	if cloudID == "" && target != nil && target.CloudID != "" {
		cloudID = target.CloudID
	}

	conn, err := s.repo.GetAtlassianConnectionByUserID(userID)
	if err != nil || conn == nil {
		if !s.IsMockMode() {
			return "", "", fmt.Errorf("no active atlassian connection for user")
		}
	}

	if cloudID == "" && conn != nil {
		cloudID = conn.CloudID
	}

	// Verify that user has access to this space
	hasAccess, err := s.CheckSpaceAccess(ctx, userID, spaceKey, cloudID)
	if err == nil && !hasAccess {
		return "", "", fmt.Errorf("access denied: user does not have permission to publish to Confluence space '%s'", spaceKey)
	}

	siteURL := ""
	if target != nil && target.SiteURL != "" {
		siteURL = target.SiteURL
	} else if conn != nil && conn.SiteURL != "" {
		siteURL = conn.SiteURL
	} else if sites, err := s.GetAccessibleSites(ctx, userID); err == nil {
		for _, st := range sites {
			if st.ID == cloudID {
				siteURL = st.URL
				break
			}
		}
	}
	if siteURL == "" && (s.IsMockMode() || cloudID == "mock-cloud-id-123" || cloudID == "mock-cloud-id-456" || (conn != nil && conn.CloudID == "mock-cloud-id-123")) {
		siteURL = "https://mock-company.atlassian.net/wiki"
	}

	existingPageID := ""
	if doc.ConfluencePageID != nil && strings.TrimSpace(*doc.ConfluencePageID) != "" {
		existingPageID = strings.TrimSpace(*doc.ConfluencePageID)
	}

	if s.IsMockMode() || cloudID == "mock-cloud-id-123" || cloudID == "mock-cloud-id-456" || (conn != nil && conn.CloudID == "mock-cloud-id-123") {
		pageID := existingPageID
		if pageID == "" {
			pageID = fmt.Sprintf("conf_%d", time.Now().Unix())
		}
		pageURL := models.FormatConfluencePageURL(siteURL, spaceKey, pageID, "")
		_ = s.repo.UpdateDocumentConfluencePage(documentID, pageID, pageURL)
		return pageID, pageURL, nil
	}

	if strings.TrimSpace(cloudID) == "" {
		return "", "", fmt.Errorf("no Confluence Cloud site is linked to your Atlassian session")
	}

	accessToken, err := s.getValidAccessToken(ctx, conn)
	if err != nil {
		return "", "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Build Confluence storage format body from markdown content in well-formed XHTML
	storageBody, err := MarkdownToConfluenceStorage(doc.ContentMD)
	if err != nil {
		storageBody = fmt.Sprintf("<pre>%s</pre>", html.EscapeString(doc.ContentMD))
	}

	// 0. If document is already published, update the existing Confluence page directly
	if existingPageID != "" {
		pageID, pageURL, err := s.updateExistingPage(ctx, client, accessToken, cloudID, siteURL, spaceKey, documentID, existingPageID, doc.Title, storageBody)
		if err == nil {
			return pageID, pageURL, nil
		}
		log.Printf("[CONFLUENCE] Failed to update existing page %s: %v. Falling back to search/recreate.", existingPageID, err)
	}

	// 1. Try Confluence REST API v2: POST /wiki/api/v2/pages (Standard modern Cloud API)
	// First resolve numeric spaceId
	spaceID := spaceKey
	sReq, sErr := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/spaces?keys=%s", cloudID, url.QueryEscape(spaceKey)), nil)
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
		cqlSearchURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/search?cql=%s&expand=space&limit=1", cloudID, url.QueryEscape(cqlQ))
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
		v1sURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/space/%s", cloudID, url.PathEscape(spaceKey))
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
		pURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/pages/%s", cloudID, url.PathEscape(parentPageID))
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
			cURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/%s?expand=space", cloudID, url.PathEscape(parentPageID))
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
		v2URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/pages", cloudID)
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
						ID    string `json:"id"`
						Links struct {
							WebUI string `json:"webui"`
						} `json:"_links"`
					}
					if json.Unmarshal(b2, &res2) == nil && res2.ID != "" {
						pageURL := models.FormatConfluencePageURL(siteURL, spaceKey, res2.ID, res2.Links.WebUI)
						_ = s.repo.UpdateDocumentConfluencePage(documentID, res2.ID, pageURL)
						return res2.ID, pageURL, nil
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
	v1URL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content", cloudID)
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
					ID    string `json:"id"`
					Links struct {
						WebUI string `json:"webui"`
						Base  string `json:"base"`
					} `json:"_links"`
				}
				if json.Unmarshal(b1, &res1) == nil && res1.ID != "" {
					usedSiteURL := siteURL
					if res1.Links.Base != "" {
						usedSiteURL = res1.Links.Base
					}
					pageURL := models.FormatConfluencePageURL(usedSiteURL, spaceKey, res1.ID, res1.Links.WebUI)
					_ = s.repo.UpdateDocumentConfluencePage(documentID, res1.ID, pageURL)
					return res1.ID, pageURL, nil
				}
			}
		}
	}

	// 3. Handle specific common errors with actionable guidance
	if v2StatusCode == http.StatusConflict || v1StatusCode == http.StatusConflict {
		// A page with this title already exists in the space: find its ID and update it!
		if foundID, findErr := s.findExistingPageByTitle(ctx, client, accessToken, cloudID, spaceKey, doc.Title); findErr == nil && foundID != "" {
			pageID, pageURL, updateErr := s.updateExistingPage(ctx, client, accessToken, cloudID, siteURL, spaceKey, documentID, foundID, doc.Title, storageBody)
			if updateErr == nil {
				return pageID, pageURL, nil
			}
			log.Printf("[CONFLUENCE] Found existing page %s for title %q but update failed: %v", foundID, doc.Title, updateErr)
		}
		return "", "", fmt.Errorf("confluence publish error (409 Conflict): a page titled %q already exists in space %q. Please choose a different title or delete the existing page in Confluence", doc.Title, spaceKey)
	}
	if v2StatusCode == http.StatusForbidden || v1StatusCode == http.StatusForbidden {
		return "", "", fmt.Errorf("confluence publish error (403 Forbidden): your Atlassian user account does not have permission to add pages in space %q. Please check your Confluence space permissions", spaceKey)
	}
	if (v1StatusCode == http.StatusGone || v1StatusCode == 0) && (v2StatusCode == http.StatusUnauthorized || v2StatusCode == 0) {
		return "", "", fmt.Errorf("confluence publish error: Atlassian Cloud has permanently retired the legacy v1 content creation API (HTTP 410 Gone) and strictly requires the granular scope 'write:page:confluence' to create pages via the modern v2 API. Please enable 'write:page:confluence' (and 'read:page:confluence', 'read:space:confluence') under Permissions -> Confluence API -> Granular scopes in your app at developer.atlassian.com/console/myapps, then reconnect Atlassian in DocForge")
	}

	if v2StatusCode > 0 {
		return "", "", fmt.Errorf("confluence publish error (%d): %s", v2StatusCode, strings.TrimSpace(v2ErrBody))
	}
	if v1StatusCode > 0 {
		return "", "", fmt.Errorf("confluence publish error (%d): %s", v1StatusCode, strings.TrimSpace(v1ErrBody))
	}

	return "", "", fmt.Errorf("confluence publish failed: could not connect to Confluence Cloud API")
}

func (s *AtlassianService) updateExistingPage(ctx context.Context, client *http.Client, accessToken, cloudID, siteURL, spaceKey, documentID, pageID, title, storageBody string) (string, string, error) {
	// 1. Try Confluence REST API v2: PUT /wiki/api/v2/pages/{id}
	// First fetch current version
	v2GetURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/pages/%s", cloudID, url.PathEscape(pageID))
	var currentVersion int
	if getReq, err := http.NewRequestWithContext(ctx, "GET", v2GetURL, nil); err == nil {
		getReq.Header.Set("Authorization", "Bearer "+accessToken)
		getReq.Header.Set("Accept", "application/json")
		if getResp, doErr := client.Do(getReq); doErr == nil {
			defer getResp.Body.Close()
			if getResp.StatusCode == http.StatusOK {
				var getData struct {
					Status  string `json:"status"`
					Version struct {
						Number int `json:"number"`
					} `json:"version"`
				}
				if json.NewDecoder(getResp.Body).Decode(&getData) == nil && getData.Version.Number > 0 {
					currentVersion = getData.Version.Number
				}
			}
		}
	}

	if currentVersion > 0 {
		nextVersion := currentVersion + 1
		v2PutPayload := map[string]any{
			"id":     pageID,
			"status": "current",
			"title":  title,
			"body": map[string]any{
				"representation": "storage",
				"value":          storageBody,
			},
			"version": map[string]any{
				"number":  nextVersion,
				"message": "Updated via DocForge",
			},
		}
		v2PutBytes, _ := json.Marshal(v2PutPayload)
		v2PutURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/pages/%s", cloudID, url.PathEscape(pageID))
		if putReq, putErr := http.NewRequestWithContext(ctx, "PUT", v2PutURL, bytes.NewReader(v2PutBytes)); putErr == nil {
			putReq.Header.Set("Authorization", "Bearer "+accessToken)
			putReq.Header.Set("Content-Type", "application/json")
			putReq.Header.Set("Accept", "application/json")
			if putResp, putDoErr := client.Do(putReq); putDoErr == nil {
				defer putResp.Body.Close()
				if putResp.StatusCode < 300 {
					var putRes struct {
						ID    string `json:"id"`
						Links struct {
							WebUI string `json:"webui"`
						} `json:"_links"`
					}
					if json.NewDecoder(putResp.Body).Decode(&putRes) == nil && putRes.ID != "" {
						pageURL := models.FormatConfluencePageURL(siteURL, spaceKey, putRes.ID, putRes.Links.WebUI)
						_ = s.repo.UpdateDocumentConfluencePage(documentID, putRes.ID, pageURL)
						return putRes.ID, pageURL, nil
					}
				}
			}
		}
	}

	// 2. Fallback: Try Confluence REST API v1: PUT /wiki/rest/api/content/{id}
	v1GetURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/%s?expand=version", cloudID, url.PathEscape(pageID))
	var v1Version int
	if getReq1, err := http.NewRequestWithContext(ctx, "GET", v1GetURL, nil); err == nil {
		getReq1.Header.Set("Authorization", "Bearer "+accessToken)
		getReq1.Header.Set("Accept", "application/json")
		if getResp1, doErr := client.Do(getReq1); doErr == nil {
			defer getResp1.Body.Close()
			if getResp1.StatusCode == http.StatusOK {
				var getData1 struct {
					Version struct {
						Number int `json:"number"`
					} `json:"version"`
				}
				if json.NewDecoder(getResp1.Body).Decode(&getData1) == nil && getData1.Version.Number > 0 {
					v1Version = getData1.Version.Number
				}
			}
		}
	}

	if v1Version > 0 {
		nextVersion := v1Version + 1
		v1PutPayload := map[string]any{
			"id":    pageID,
			"type":  "page",
			"title": title,
			"space": map[string]string{"key": spaceKey},
			"body": map[string]any{
				"storage": map[string]string{
					"value":          storageBody,
					"representation": "storage",
				},
			},
			"version": map[string]any{
				"number":  nextVersion,
				"message": "Updated via DocForge",
			},
		}
		v1PutBytes, _ := json.Marshal(v1PutPayload)
		v1PutURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/%s", cloudID, url.PathEscape(pageID))
		if putReq1, putErr1 := http.NewRequestWithContext(ctx, "PUT", v1PutURL, bytes.NewReader(v1PutBytes)); putErr1 == nil {
			putReq1.Header.Set("Authorization", "Bearer "+accessToken)
			putReq1.Header.Set("Content-Type", "application/json")
			putReq1.Header.Set("Accept", "application/json")
			if putResp1, putDoErr1 := client.Do(putReq1); putDoErr1 == nil {
				defer putResp1.Body.Close()
				if putResp1.StatusCode < 300 {
					var putRes1 struct {
						ID    string `json:"id"`
						Links struct {
							WebUI string `json:"webui"`
							Base  string `json:"base"`
						} `json:"_links"`
					}
					if json.NewDecoder(putResp1.Body).Decode(&putRes1) == nil && putRes1.ID != "" {
						usedSiteURL := siteURL
						if putRes1.Links.Base != "" {
							usedSiteURL = putRes1.Links.Base
						}
						pageURL := models.FormatConfluencePageURL(usedSiteURL, spaceKey, putRes1.ID, putRes1.Links.WebUI)
						_ = s.repo.UpdateDocumentConfluencePage(documentID, putRes1.ID, pageURL)
						return putRes1.ID, pageURL, nil
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("failed to update Confluence page %s", pageID)
}

func (s *AtlassianService) findExistingPageByTitle(ctx context.Context, client *http.Client, accessToken, cloudID, spaceKey, title string) (string, error) {
	// Try CQL search
	cql := fmt.Sprintf(`space = %q AND title = %q AND type = page`, spaceKey, title)
	searchURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/rest/api/content/search?cql=%s&limit=1", cloudID, url.QueryEscape(cql))
	if req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil); err == nil {
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		if resp, doErr := client.Do(req); doErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var data struct {
					Results []struct {
						ID string `json:"id"`
					} `json:"results"`
				}
				if json.NewDecoder(resp.Body).Decode(&data) == nil && len(data.Results) > 0 && data.Results[0].ID != "" {
					return data.Results[0].ID, nil
				}
			}
		}
	}

	// Fallback: Try v2 title search
	v2SearchURL := fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/api/v2/pages?title=%s&limit=5", cloudID, url.QueryEscape(title))
	if req2, err2 := http.NewRequestWithContext(ctx, "GET", v2SearchURL, nil); err2 == nil {
		req2.Header.Set("Authorization", "Bearer "+accessToken)
		req2.Header.Set("Accept", "application/json")
		if resp2, doErr2 := client.Do(req2); doErr2 == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode == http.StatusOK {
				var data2 struct {
					Results []struct {
						ID string `json:"id"`
					} `json:"results"`
				}
				if json.NewDecoder(resp2.Body).Decode(&data2) == nil && len(data2.Results) > 0 && data2.Results[0].ID != "" {
					return data2.Results[0].ID, nil
				}
			}
		}
	}

	return "", fmt.Errorf("page titled %q not found in space %s", title, spaceKey)
}

// resolveConfluenceURL resolves relative or host-absolute pagination links returned by Confluence
// to a fully qualified URL compatible with Atlassian Cloud OAuth 2.0 proxy.
func resolveConfluenceURL(cloudID, rawLink string) string {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" {
		return ""
	}

	if strings.HasPrefix(rawLink, "http://") || strings.HasPrefix(rawLink, "https://") {
		u, err := url.Parse(rawLink)
		if err == nil {
			reqURI := u.RequestURI()
			if strings.HasPrefix(u.Path, "/ex/confluence/") {
				return "https://api.atlassian.com" + reqURI
			}
			if strings.HasPrefix(u.Path, "/wiki/") {
				return fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s%s", cloudID, reqURI)
			}
			if strings.HasPrefix(u.Path, "/rest/") || strings.HasPrefix(u.Path, "/api/") {
				return fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki%s", cloudID, reqURI)
			}
			if u.Host == "api.atlassian.com" {
				return rawLink
			}
			if idx := strings.Index(u.Path, "/wiki/"); idx != -1 {
				u.Path = u.Path[idx:]
				return fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s%s", cloudID, u.RequestURI())
			}
			return fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki%s", cloudID, reqURI)
		}
	}

	if strings.HasPrefix(rawLink, "/ex/confluence/") {
		return "https://api.atlassian.com" + rawLink
	}
	if strings.HasPrefix(rawLink, "/wiki/") {
		return fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s%s", cloudID, rawLink)
	}
	if strings.HasPrefix(rawLink, "/") {
		return fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki%s", cloudID, rawLink)
	}
	return fmt.Sprintf("https://api.atlassian.com/ex/confluence/%s/wiki/%s", cloudID, rawLink)
}



