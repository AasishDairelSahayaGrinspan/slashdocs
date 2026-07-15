package knowledge

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
)

var (
	ErrNotFound = errors.New("knowledge record not found")
	ErrConflict = errors.New("knowledge record conflict")
	ErrInvalid  = errors.New("invalid knowledge record")
)

const MaxPortalSlugLength = 80

const (
	ProviderGitHub      = "github"
	ProviderSlack       = "slack"
	ProviderDevRev      = "devrev"
	ProviderGoogleDrive = "google_drive"
	ProviderConfluence  = "confluence"
	ProviderNotion      = "notion"
	ProviderManual      = "manual"
)

var Providers = map[string]bool{
	ProviderGitHub: true, ProviderSlack: true, ProviderDevRev: true,
	ProviderGoogleDrive: true, ProviderConfluence: true, ProviderNotion: true,
	ProviderManual: true,
}

type PathRule struct {
	Pattern           string   `json:"pattern"`
	RequiredReviewers []string `json:"required_reviewers,omitempty"`
	RequiredTeams     []string `json:"required_teams,omitempty"`
}

type PortalSettings struct {
	Enabled    bool   `json:"enabled"`
	Slug       string `json:"slug"`
	Visibility string `json:"visibility"`
}

// NormalizePortalSlug is the canonical public-portal address normalizer. Keep
// this ASCII-only behavior aligned with the SQL expression in migration 00024
// and the browser preview so every storage backend reserves the same slug.
func NormalizePortalSlug(value string) string {
	var normalized strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			normalized.WriteRune(r)
			lastDash = false
			continue
		}
		if normalized.Len() > 0 && !lastDash {
			normalized.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(normalized.String(), "-")
	if len(slug) > MaxPortalSlugLength {
		slug = slug[:MaxPortalSlugLength]
	}
	return slug
}

type FreshnessPolicy struct {
	StaleAfterHours int    `json:"stale_after_hours"`
	OnStale         string `json:"on_stale"`
}

type TransferState struct {
	OwnerID          string `json:"owner_id,omitempty"`
	OwnerEmail       string `json:"owner_email,omitempty"`
	Status           string `json:"status,omitempty"`
	RequestedByID    string `json:"requested_by_id,omitempty"`
	TemporaryGrantID string `json:"temporary_grant_id,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

type Settings struct {
	RepositoryID      string          `json:"repository_id"`
	Archived          bool            `json:"archived"`
	RequiredReviewers []string        `json:"required_reviewers"`
	RequiredTeams     []string        `json:"required_teams"`
	PathRules         []PathRule      `json:"path_rules"`
	Portal            PortalSettings  `json:"portal"`
	FreshnessPolicy   FreshnessPolicy `json:"freshness_policy"`
	Transfer          TransferState   `json:"transfer,omitempty"`
	UpdatedAt         string          `json:"updated_at,omitempty"`
}

func DefaultSettings(repositoryID string) Settings {
	return Settings{
		RepositoryID:      repositoryID,
		RequiredReviewers: []string{}, RequiredTeams: []string{}, PathRules: []PathRule{},
		Portal:          PortalSettings{Visibility: "organization"},
		FreshnessPolicy: FreshnessPolicy{StaleAfterHours: 168, OnStale: "notify"},
	}
}

type Freshness struct {
	State           string `json:"state"`
	CheckedAt       string `json:"checked_at,omitempty"`
	StaleAfterHours int    `json:"stale_after_hours,omitempty"`
}

type Source struct {
	ID                 string         `json:"id"`
	RepositoryID       string         `json:"repository_id"`
	Provider           string         `json:"provider"`
	ExternalRepository string         `json:"external_repository,omitempty"`
	Branch             string         `json:"branch"`
	IncludePaths       []string       `json:"include_paths"`
	ExcludePaths       []string       `json:"exclude_paths"`
	TargetFolderID     string         `json:"target_folder_id,omitempty"`
	TargetDocumentID   string         `json:"target_document_id,omitempty"`
	TriggerMode        string         `json:"trigger_mode"`
	Direction          string         `json:"direction"`
	Status             string         `json:"status"`
	Freshness          Freshness      `json:"freshness"`
	LastSyncedAt       string         `json:"last_synced_at,omitempty"`
	CreatedBy          auth.Principal `json:"created_by"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
}

type SyncRun struct {
	ID                string        `json:"id"`
	RepositoryID      string        `json:"repository_id"`
	SourceID          string        `json:"source_id"`
	DeliveryID        string        `json:"delivery_id,omitempty"`
	BeforeSHA         string        `json:"before_sha,omitempty"`
	AfterSHA          string        `json:"after_sha,omitempty"`
	ChangedFiles      []string      `json:"changed_files"`
	Status            string        `json:"status"`
	Outcome           string        `json:"outcome,omitempty"`
	Message           string        `json:"message,omitempty"`
	PullRequestNumber int64         `json:"pull_request_number,omitempty"`
	ContextItems      []ContextItem `json:"context_items,omitempty"`
	Provenance        []Citation    `json:"provenance"`
	StartedAt         string        `json:"started_at"`
	FinishedAt        string        `json:"finished_at,omitempty"`
}

// ContextItem is a provider-neutral, untrusted excerpt carried from an event
// into the reviewable document proposal. Renderers must escape every field.
type ContextItem struct {
	Kind       string            `json:"kind"`
	ExternalID string            `json:"external_id,omitempty"`
	Title      string            `json:"title,omitempty"`
	Body       string            `json:"body,omitempty"`
	URL        string            `json:"url,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type AuditEvent struct {
	ID           string         `json:"id"`
	RepositoryID string         `json:"repository_id"`
	Action       string         `json:"action"`
	Actor        auth.Principal `json:"actor"`
	SubjectType  string         `json:"subject_type"`
	SubjectID    string         `json:"subject_id,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
	CreatedAt    string         `json:"created_at"`
}

type Notification struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	RepositoryID string `json:"repository_id,omitempty"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	Body         string `json:"body,omitempty"`
	URL          string `json:"url,omitempty"`
	Read         bool   `json:"read"`
	CreatedAt    string `json:"created_at"`
}

type Team struct {
	ID           string   `json:"id"`
	RepositoryID string   `json:"repository_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	MemberIDs    []string `json:"member_ids"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Title       string `json:"title"`
	HTML        string `json:"html,omitempty"`
}

type Citation struct {
	DocumentID string `json:"document_id,omitempty"`
	Title      string `json:"title,omitempty"`
	VersionID  string `json:"version_id,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
	Provider   string `json:"provider,omitempty"`
	URL        string `json:"url,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
}

type ChangeProposal struct {
	ID           string   `json:"id"`
	RepositoryID string   `json:"repository_id"`
	SourceID     string   `json:"source_id"`
	Provider     string   `json:"provider"`
	Direction    string   `json:"direction"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	ChangedFiles []string `json:"changed_files"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"created_at"`
}

//nolint:interfacebloat // Store is the durable connected-knowledge boundary shared by memory and Postgres backends.
type Store interface {
	GetSettings(context.Context, string) (Settings, error)
	PutSettings(context.Context, Settings) (Settings, error)
	FindPortal(context.Context, string) (Settings, error)
	ListSources(context.Context, string) ([]Source, error)
	ListAllSources(context.Context) ([]Source, error)
	GetSource(context.Context, string, string) (Source, error)
	PutSource(context.Context, Source) (Source, error)
	DeleteSource(context.Context, string, string) error
	AcceptDelivery(context.Context, string, string) (bool, error)
	QueueDelivery(context.Context, string, string, []SyncRun) (bool, error)
	AddSyncRun(context.Context, SyncRun) (SyncRun, error)
	ListSyncRuns(context.Context, string) ([]SyncRun, error)
	ClaimQueuedSyncRuns(context.Context, int) ([]SyncRun, error)
	RecoverSyncRuns(context.Context) error
	AddAudit(context.Context, AuditEvent) error
	ListAudit(context.Context, string) ([]AuditEvent, error)
	AddNotification(context.Context, Notification) error
	ListNotifications(context.Context, string) ([]Notification, error)
	SetNotificationRead(context.Context, string, string, bool) (Notification, error)
	ListTeams(context.Context, string) ([]Team, error)
	GetTeam(context.Context, string, string) (Team, error)
	PutTeam(context.Context, Team) (Team, error)
	DeleteTeam(context.Context, string, string) error
}

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
