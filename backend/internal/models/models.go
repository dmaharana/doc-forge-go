package models

import (
	"time"
)

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         UserRole  `json:"role"`
	PasswordHash string    `json:"-"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LLMConfig struct {
	ID              string    `json:"id"`
	Provider        string    `json:"provider"` // openai, anthropic, azure, local
	APIKeyEncrypted string    `json:"-"`
	APIKeyMasked    string    `json:"api_key_masked,omitempty"`
	BaseURL         string    `json:"base_url,omitempty"`
	DefaultModel    string    `json:"default_model"`
	MaxTokens       int       `json:"max_tokens"`
	Temperature     float64   `json:"temperature"`
	IsActive        bool      `json:"is_active"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	OwnerName   string    `json:"owner_name,omitempty"`
	Status      string    `json:"status"` // active, archived
	UserRole    string    `json:"user_role,omitempty"` // populated for current user (owner, member)
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProjectMemberRole string

const (
	ProjectMemberRoleOwner  ProjectMemberRole = "owner"
	ProjectMemberRoleMember ProjectMemberRole = "member"
)

type ProjectMemberStatus string

const (
	ProjectMemberStatusActive  ProjectMemberStatus = "active"
	ProjectMemberStatusPending ProjectMemberStatus = "pending"
)

type ProjectMember struct {
	ID        string              `json:"id"`
	ProjectID string              `json:"project_id"`
	UserID    *string             `json:"user_id,omitempty"`
	UserName  string              `json:"user_name,omitempty"`
	UserEmail string              `json:"user_email"`
	Role      ProjectMemberRole   `json:"role"`
	Status    ProjectMemberStatus `json:"status"`
	CreatedAt time.Time           `json:"created_at"`
}

type SourceType string

const (
	SourceTypeUpload SourceType = "upload"
	SourceTypeRemote SourceType = "remote"
)

type Source struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	Name             string     `json:"name"`
	Type             SourceType `json:"type"` // upload, remote
	FilePath         string     `json:"file_path,omitempty"`
	OriginalFilename string     `json:"original_filename,omitempty"`
	MimeType         string     `json:"mime_type,omitempty"`
	SizeBytes        int64      `json:"size_bytes,omitempty"`
	RemoteURL        string     `json:"remote_url,omitempty"`
	RemoteType       string     `json:"remote_type,omitempty"` // git, http, s3, gcs, azure, local
	ExtractedText    string     `json:"extracted_text,omitempty"`
	MetadataJSON     string     `json:"metadata_json,omitempty"`
	Status           string     `json:"status"` // pending, ready, error
	CreatedAt        time.Time  `json:"created_at"`
}

type AtlassianConnection struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"user_id"`
	AccessTokenEncrypted  string     `json:"-"`
	RefreshTokenEncrypted string     `json:"-"`
	TokenExpiry           *time.Time `json:"token_expiry,omitempty"`
	CloudID               string     `json:"cloud_id,omitempty"`
	SiteURL               string     `json:"site_url,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ConfluenceTarget struct {
	ID              string     `json:"id"`
	ProjectID       string     `json:"project_id"`
	ConnectionID    string     `json:"connection_id,omitempty"`
	SpaceKey        string     `json:"space_key"`
	SpaceName       string     `json:"space_name"`
	ParentPageID    string     `json:"parent_page_id,omitempty"`
	ParentPageTitle string     `json:"parent_page_title,omitempty"`
	IsDefault       bool       `json:"is_default"`
	UpdatedBy       *string    `json:"updated_by,omitempty"`
	UpdaterName     string     `json:"updater_name,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type GenerationStatus string

const (
	GenerationQueued     GenerationStatus = "queued"
	GenerationProcessing GenerationStatus = "processing"
	GenerationCompleted  GenerationStatus = "completed"
	GenerationFailed     GenerationStatus = "failed"
)

type Generation struct {
	ID                 string           `json:"id"`
	ProjectID          string           `json:"project_id"`
	RequestedBy        string           `json:"requested_by"`
	RequesterName      string           `json:"requester_name,omitempty"`
	Status             GenerationStatus `json:"status"`
	TemplateSet        string           `json:"template_set"` // comma separated or json array
	LLMConfigID        *string          `json:"llm_config_id,omitempty"`
	ModelUsed          string           `json:"model_used,omitempty"`
	ErrorMessage       string           `json:"error_message,omitempty"`
	NotificationSentAt *time.Time       `json:"notification_sent_at,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	StartedAt          *time.Time       `json:"started_at,omitempty"`
	CompletedAt        *time.Time       `json:"completed_at,omitempty"`
	SourceIDs          []string         `json:"source_ids,omitempty"`
}

type DocumentStatus string

const (
	DocStatusDraft    DocumentStatus = "draft"
	DocStatusInReview DocumentStatus = "in_review"
	DocStatusApproved DocumentStatus = "approved"
)

type Document struct {
	ID               string         `json:"id"`
	GenerationID     *string        `json:"generation_id,omitempty"`
	ProjectID        string         `json:"project_id"`
	Title            string         `json:"title"`
	TemplateType     string         `json:"template_type"` // product_spec, adr, release_notes, runbook
	ContentMD        string         `json:"content_md"`
	Status           DocumentStatus `json:"status"`
	CurrentVersionID *string        `json:"current_version_id,omitempty"`
	ConfluencePageID *string        `json:"confluence_page_id,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`

	// Populated for detail views
	Versions []DocumentVersion  `json:"versions,omitempty"`
	Feedback []DocumentFeedback `json:"feedback,omitempty"`
}

type DocumentVersion struct {
	ID            string    `json:"id"`
	DocumentID    string    `json:"document_id"`
	VersionNumber int       `json:"version_number"`
	ContentMD     string    `json:"content_md"`
	ChangeSummary string    `json:"change_summary"`
	CreatedBy     string    `json:"created_by"`
	AuthorName    string    `json:"author_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type FeedbackCategory string

const (
	FeedbackCorrection     FeedbackCategory = "correction"
	FeedbackMissingInfo    FeedbackCategory = "missing_info"
	FeedbackAdditionalFact FeedbackCategory = "additional_fact"
	FeedbackClarification  FeedbackCategory = "clarification"
	FeedbackStyle          FeedbackCategory = "style"
	FeedbackOther          FeedbackCategory = "other"
)

type FeedbackStatus string

const (
	FeedbackOpen      FeedbackStatus = "open"
	FeedbackAddressed FeedbackStatus = "addressed"
	FeedbackDismissed FeedbackStatus = "dismissed"
)

type DocumentFeedback struct {
	ID            string           `json:"id"`
	DocumentID    string           `json:"document_id"`
	UserID        string           `json:"user_id"`
	AuthorName    string           `json:"author_name,omitempty"`
	AuthorEmail   string           `json:"author_email,omitempty"`
	SectionAnchor string           `json:"section_anchor,omitempty"`
	Category      FeedbackCategory `json:"category"`
	Content       string           `json:"content"`
	IsBlocking    bool             `json:"is_blocking"`
	Status        FeedbackStatus   `json:"status"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

type NotificationChannel string

const (
	NotificationChannelEmail NotificationChannel = "email"
	NotificationChannelTeams NotificationChannel = "teams"
)

type NotificationPreference struct {
	ID                       string              `json:"id"`
	UserID                   string              `json:"user_id"`
	ProjectID                *string             `json:"project_id,omitempty"`
	Channel                  NotificationChannel `json:"channel"`
	Enabled                  bool                `json:"enabled"`
	TeamsWebhookURLEncrypted string              `json:"-"`
	TeamsWebhookMasked       string              `json:"teams_webhook_masked,omitempty"`
	CreatedAt                time.Time           `json:"created_at"`
}
