package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"docforge/internal/config"
	"docforge/internal/models"
	"docforge/internal/repository"
	"docforge/internal/service"
)

type contextKey string

const (
	UserContextKey contextKey = "currentUser"
)

type Claims struct {
	UserID string          `json:"user_id"`
	Email  string          `json:"email"`
	Name   string          `json:"name"`
	Role   models.UserRole `json:"role"`
	jwt.RegisteredClaims
}

func GenerateJWT(user *models.User, secret string) (string, error) {
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Name:   user.Name,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "docforge",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseJWT(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

// AuthMiddleware validates JWT Bearer token and attaches user to context
func AuthMiddleware(cfg *config.Config, repo *repository.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// Also check query param or cookie for convenience (e.g. file downloads)
				if tokenQ := r.URL.Query().Get("token"); tokenQ != "" {
					authHeader = "Bearer " + tokenQ
				}
			}

			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"unauthorized: missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := ParseJWT(tokenStr, cfg.JWTSecret)
			if err != nil {
				http.Error(w, `{"error":"unauthorized: invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			user, err := repo.GetUserByID(claims.UserID)
			if err != nil || user == nil || !user.IsActive {
				http.Error(w, `{"error":"unauthorized: user inactive or not found"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin middleware ensures the authenticated user has RoleAdmin
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserContextKey).(*models.User)
		if !ok || user.Role != models.RoleAdmin {
			http.Error(w, `{"error":"forbidden: administrator privilege required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetCurrentUser extracts the authenticated user from context
func GetCurrentUser(r *http.Request) *models.User {
	if u, ok := r.Context().Value(UserContextKey).(*models.User); ok {
		return u
	}
	return nil
}

// ----------------- AUTH HANDLERS -----------------

type AuthHandler struct {
	cfg       *config.Config
	repo      *repository.Repository
	atlassian *service.AtlassianService
}

func NewAuthHandler(cfg *config.Config, repo *repository.Repository, atlassian *service.AtlassianService) *AuthHandler {
	return &AuthHandler{cfg: cfg, repo: repo, atlassian: atlassian}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	user, err := h.repo.GetUserByEmail(req.Email)
	if err != nil || user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	if !user.IsActive {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "account is deactivated"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	token, err := GenerateJWT(user, h.cfg.JWTSecret)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	respondJSON(w, http.StatusOK, LoginResponse{
		Token: token,
		User:  user,
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	if user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	respondJSON(w, http.StatusOK, user)
}

// Atlassian SSO Endpoints
func (h *AuthHandler) AtlassianSSOStart(w http.ResponseWriter, r *http.Request) {
	state := uuid.New().String()
	authURL := h.atlassian.GetSSOAuthorizationURL(state)
	respondJSON(w, http.StatusOK, map[string]string{
		"authorization_url": authURL,
		"is_mock_mode":      fmt.Sprintf("%t", h.atlassian.IsMockMode()),
	})
}

func (h *AuthHandler) AtlassianSSOCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		code = "mock_atlassian_code"
	}

	user, _, err := h.atlassian.ExchangeSSOCode(r.Context(), code)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	token, err := GenerateJWT(user, h.cfg.JWTSecret)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Logging in...</title></head>
<body>
<script>
  localStorage.setItem('docforge_token', '%s');
  window.location.href = '/';
</script>
<p>Authentication successful. Redirecting...</p>
</body>
</html>`, token)
		_, _ = w.Write([]byte(html))
		return
	}

	respondJSON(w, http.StatusOK, LoginResponse{
		Token: token,
		User:  user,
	})
}

// Microsoft SSO Extension Points
func (h *AuthHandler) MicrosoftSSOStart(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Microsoft SSO extension point. For MVP, please use Atlassian SSO or standard email/password.",
		"sso_configured": false,
	})
}

func (h *AuthHandler) MicrosoftSSOCallback(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusNotImplemented, map[string]string{"error": "Microsoft SSO not active in MVP"})
}
