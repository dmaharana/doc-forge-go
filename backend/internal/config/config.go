package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          int
	DBPath        string
	JWTSecret     string
	EncryptionKey []byte
	UploadDir     string
	DevMode       bool
	MockAtlassian bool

	// Atlassian OAuth2
	AtlassianClientID     string
	AtlassianClientSecret string
	AtlassianRedirectURI  string
	AtlassianScopes       string

	// SMTP Notification
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// Admin Seed
	AdminEmail    string
	AdminPassword string
	AdminName     string
}

func Load() *Config {
	// Attempt to load .env if present
	_ = godotenv.Load()

	port, _ := strconv.Atoi(getEnv("PORT", "8080"))
	if port == 0 {
		port = 8080
	}

	smtpPort, _ := strconv.Atoi(getEnv("SMTP_PORT", "587"))

	devMode := getEnv("DEV_MODE", "true") == "true"
	mockAtlassian := getEnv("MOCK_ATLASSIAN", "true") == "true"

	encKeyHex := os.Getenv("ENCRYPTION_KEY")
	var encKey []byte
	if len(encKeyHex) >= 32 {
		if decoded, err := hex.DecodeString(encKeyHex); err == nil && len(decoded) == 32 {
			encKey = decoded
		} else {
			// fallback: raw 32 bytes
			encKey = []byte(encKeyHex[:32])
		}
	} else {
		// Generate or use deterministic fallback for dev
		log.Println("[CONFIG] WARNING: ENCRYPTION_KEY not set or < 32 bytes, using dev default 32-byte key")
		encKey = []byte("12345678901234567890123456789012")
	}

	jwtSecret := getEnv("JWT_SECRET", "docforge-dev-secret-key-32-bytes-minimum")

	return &Config{
		Port:          port,
		DBPath:        getEnv("DB_PATH", "./docforge.db"),
		JWTSecret:     jwtSecret,
		EncryptionKey: encKey,
		UploadDir:     getEnv("UPLOAD_DIR", "./uploads"),
		DevMode:       devMode,
		MockAtlassian: mockAtlassian,

		AtlassianClientID:     os.Getenv("ATLASSIAN_CLIENT_ID"),
		AtlassianClientSecret: os.Getenv("ATLASSIAN_CLIENT_SECRET"),
		AtlassianRedirectURI:  getEnv("ATLASSIAN_REDIRECT_URI", "http://localhost:8080/api/v1/atlassian/callback"),
		AtlassianScopes:       getEnv("ATLASSIAN_SCOPES", "read:me offline_access write:page:confluence read:page:confluence read:space:confluence write:confluence-content read:confluence-space.summary write:confluence-file read:confluence-content.all read:confluence-content.summary search:confluence readonly:content.attachment:confluence"),

		SMTPHost: os.Getenv("SMTP_HOST"),
		SMTPPort: smtpPort,
		SMTPUser: os.Getenv("SMTP_USER"),
		SMTPPass: os.Getenv("SMTP_PASS"),
		SMTPFrom: getEnv("SMTP_FROM", "docforge@example.com"),

		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@docforge.local"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "Admin123!"),
		AdminName:     getEnv("ADMIN_NAME", "System Administrator"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func GenerateRandomKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
