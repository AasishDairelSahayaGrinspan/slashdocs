package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
)

type Role string

const (
	RoleNone      Role = ""
	RoleViewer    Role = "viewer"
	RoleCommenter Role = "commenter"
	RoleEditor    Role = "editor"
	RoleOwner     Role = "owner"
)

var roleRank = map[Role]int{RoleViewer: 1, RoleCommenter: 2, RoleEditor: 3, RoleOwner: 4}

// MaxRole returns whichever role grants more access. RoleNone ("") ranks below
// every real role, so MaxRole(RoleNone, x) == x.
func MaxRole(a, b Role) Role {
	if roleRank[b] > roleRank[a] {
		return b
	}
	return a
}

func sortedKVKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStrings(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}

// ErrNotFound wraps auth.ErrNotFound so that packages which cannot import repo
// (to avoid an import cycle) can still detect not-found via errors.Is.
var ErrNotFound = fmt.Errorf("not found: %w", auth.ErrNotFound)
var ErrConflict = errors.New("conflict")
var ErrForbidden = errors.New("forbidden")
var ErrInvalidCommentBody = errors.New("invalid comment body")
var ErrInvalidCommentMention = errors.New("invalid comment mention")

// Ownership-transfer state errors, shared by all repository implementations so
// handlers can map them to responses without matching on message text.
var (
	ErrNotTransferTarget  = errors.New("not transfer target")
	ErrTransferNotPending = errors.New("transfer not pending")
)

// ErrVersionConflict indicates an optimistic-concurrency failure when creating
// a new version: the supplied base_version_id is stale. The concrete error
// returned is a *VersionConflictError carrying the latest version id; callers
// can match it with errors.Is(err, ErrVersionConflict) or extract details with
// errors.As(&VersionConflictError{}).
var ErrVersionConflict = errors.New("version_conflict")

// VersionConflictError carries the current/latest version id alongside the
// sentinel ErrVersionConflict.
type VersionConflictError struct {
	LatestVersionID string
}

func (e *VersionConflictError) Error() string        { return "version_conflict" }
func (e *VersionConflictError) Is(target error) bool { return target == ErrVersionConflict }

var ErrAmbiguousPath = errors.New("ambiguous_path")

var (
	ErrPullRequestRequired      = errors.New("pull_request_required")
	ErrRepositoryNotEmpty       = errors.New("repository_not_empty")
	ErrSelfReview               = errors.New("self_review")
	ErrApprovalRequired         = errors.New("approval_required")
	ErrChangesRequested         = errors.New("changes_requested")
	ErrStalePullRequest         = errors.New("stale_pull_request")
	ErrPullRequestClosed        = errors.New("pull_request_closed")
	ErrRepositoryBoundary       = errors.New("repository_boundary")
	ErrInvalidPullRequestChange = errors.New("invalid_pull_request_change")
)

type PullRequestRequiredError struct {
	RepositoryID string
}

func (e *PullRequestRequiredError) Error() string        { return "pull_request_required" }
func (e *PullRequestRequiredError) Is(target error) bool { return target == ErrPullRequestRequired }

type AmbiguousPathError struct {
	Segment    string
	Candidates []Folder
}

func (e *AmbiguousPathError) Error() string        { return "ambiguous_path" }
func (e *AmbiguousPathError) Is(target error) bool { return target == ErrAmbiguousPath }

// Comment / transfer status values.
const (
	StatusOpen     = "open"
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusDeclined = "declined"
)

type Document struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Owner            auth.Principal    `json:"owner"`
	CurrentVersionID string            `json:"current_version_id"`
	ParentFolderID   string            `json:"parent_folder_id,omitempty"`
	ParentFolder     *FolderBreadcrumb `json:"parent_folder,omitempty"`
	UpdatedAt        string            `json:"updated_at,omitempty"`
}

type FolderBreadcrumb struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Folder struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Owner          auth.Principal     `json:"owner"`
	ParentFolderID string             `json:"parent_folder_id,omitempty"`
	Ancestors      []FolderBreadcrumb `json:"ancestors,omitempty"`
}

// FolderAccess is one element of a folder's ancestor chain annotated with a
// principal's effective role at that folder (RoleNone when they have no
// access at that level).
type FolderAccess struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Role  Role   `json:"role"`
}

type FolderEntry struct {
	Type       string    `json:"type"`
	Folder     *Folder   `json:"folder,omitempty"`
	Document   *Document `json:"document,omitempty"`
	LastActive string    `json:"last_active,omitempty"`
}

type Pin struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

type AccessSource struct {
	ResourceType  string `json:"resource_type"`
	ResourceID    string `json:"resource_id"`
	ResourceTitle string `json:"resource_title"`
	Role          Role   `json:"role"`
	GrantID       string `json:"grant_id,omitempty"`
}
type EffectiveAccessItem struct {
	Principal     auth.Principal `json:"principal"`
	EffectiveRole Role           `json:"effective_role"`
	Sources       []AccessSource `json:"sources"`
}

type ServiceAccount struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Owner    auth.Principal `json:"owner"`
	Disabled bool           `json:"disabled"`
}

type ServiceAccountKey struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type OwnershipTransfer struct {
	ID               string `json:"id"`
	ServiceAccountID string `json:"service_account_id"`
	FromUserID       string `json:"from_user_id"`
	ToUserID         string `json:"to_user_id"`
	Status           string `json:"status"`
}

type Grant struct {
	ID           string         `json:"id"`
	DocumentID   string         `json:"document_id,omitempty"`
	ResourceType string         `json:"resource_type,omitempty"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Principal    auth.Principal `json:"principal"`
	Role         Role           `json:"role"`
	GrantedBy    auth.Principal `json:"granted_by"`
}

type Version struct {
	ID            string         `json:"id"`
	Number        int            `json:"number"`
	DocumentID    string         `json:"document_id"`
	CreatedBy     auth.Principal `json:"created_by"`
	ChangeSummary string         `json:"change_summary"`
	SHA256        string         `json:"sha256"`
}

const (
	RepositoryScopePersonal     = "personal"
	RepositoryScopeOrganization = "organization"

	PullRequestStatusOpen   = "open"
	PullRequestStatusMerged = "merged"
	PullRequestStatusClosed = "closed"

	ReviewStateApproved         = "approved"
	ReviewStateChangesRequested = "changes_requested"

	PullRequestChangeUpdate = "update"
	PullRequestChangeCreate = "create"
	PullRequestChangeRename = "rename"
	PullRequestChangeMove   = "move"
	PullRequestChangeDelete = "delete"
)

type RepositoryRecord struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Slug              string         `json:"slug"`
	Description       string         `json:"description"`
	Scope             string         `json:"scope"`
	NamespaceID       string         `json:"namespace_id"`
	NamespaceName     string         `json:"namespace_name"`
	RootFolderID      string         `json:"root_folder_id"`
	CreatedBy         auth.Principal `json:"created_by"`
	RequiredApprovals int            `json:"required_approvals"`
	Protected         bool           `json:"protected"`
	CreatedAt         string         `json:"created_at,omitempty"`
	UpdatedAt         string         `json:"updated_at,omitempty"`
}

type CreateRepositoryParams struct {
	Name              string
	Slug              string
	Description       string
	Scope             string
	NamespaceID       string
	NamespaceName     string
	RootFolderID      string
	DocumentID        string
	RequiredApprovals int
	Protected         bool
	GrantOrgViewer    bool
}

type PullRequestReview struct {
	ID            string         `json:"id"`
	PullRequestID string         `json:"pull_request_id"`
	HeadVersionID string         `json:"head_version_id"`
	Patchset      int            `json:"patchset"`
	Reviewer      auth.Principal `json:"reviewer"`
	State         string         `json:"state"`
	Body          string         `json:"body"`
	SubmittedAt   string         `json:"submitted_at"`
}

// PullRequestChange is one document-tree mutation in a pull-request patchset.
// Base fields describe the published tree when the patchset was created; head
// fields describe the proposed result. HTML is stored separately and is never
// returned inline from repository APIs.
type PullRequestChange struct {
	ID            string `json:"id"`
	PullRequestID string `json:"pull_request_id"`
	Patchset      int    `json:"patchset"`
	Operation     string `json:"operation"`
	DocumentID    string `json:"document_id,omitempty"`
	FolderID      string `json:"folder_id,omitempty"`
	BaseVersionID string `json:"base_version_id,omitempty"`
	HeadVersionID string `json:"head_version_id,omitempty"`
	BaseTitle     string `json:"base_title,omitempty"`
	Title         string `json:"title,omitempty"`
	BasePath      string `json:"base_path,omitempty"`
	Path          string `json:"path,omitempty"`
}

// PullRequestChangeInput is accepted by change-set and patchset creation. HTML
// is required for create/update operations and ignored for tree-only changes.
type PullRequestChangeInput struct {
	Operation     string
	DocumentID    string
	FolderID      string
	BaseVersionID string
	Title         string
	Path          string
	HTML          []byte
}

type PullRequest struct {
	ID                 string              `json:"id"`
	RepositoryID       string              `json:"repository_id"`
	Number             int64               `json:"number"`
	DocumentID         string              `json:"document_id"`
	BaseVersionID      string              `json:"base_version_id"`
	HeadVersionID      string              `json:"head_version_id"`
	Patchset           int                 `json:"patchset"`
	Changes            []PullRequestChange `json:"changes"`
	Title              string              `json:"title"`
	Body               string              `json:"body"`
	ChangeSummary      string              `json:"change_summary"`
	Status             string              `json:"status"`
	Author             auth.Principal      `json:"author"`
	MergedBy           *auth.Principal     `json:"merged_by,omitempty"`
	CreatedAt          string              `json:"created_at,omitempty"`
	UpdatedAt          string              `json:"updated_at,omitempty"`
	MergedAt           string              `json:"merged_at,omitempty"`
	Reviews            []PullRequestReview `json:"reviews"`
	RequiredApprovals  int                 `json:"required_approvals"`
	Mergeable          bool                `json:"mergeable"`
	MergeBlockedReason string              `json:"merge_blocked_reason,omitempty"`
}

// UserProfile is an access-filtered activity view for a user. Callers build it
// from resources the requesting principal can already read; repository fields
// are omitted for loose documents that do not belong to a repository.
type UserProfile struct {
	User          auth.Principal           `json:"user"`
	Stats         UserProfileStats         `json:"stats"`
	Documents     []UserProfileDocument    `json:"documents"`
	PullRequests  []UserProfilePullRequest `json:"pull_requests"`
	Contributions []UserContribution       `json:"contributions"`
}

type UserProfileStats struct {
	DocumentsCreated          int `json:"documents_created"`
	PullRequestsAuthored      int `json:"pull_requests_authored"`
	Contributions             int `json:"contributions"`
	RepositoriesContributedTo int `json:"repositories_contributed_to"`
}

type UserProfileDocument struct {
	Document   Document          `json:"document"`
	Repository *RepositoryRecord `json:"repository,omitempty"`
}

type UserProfilePullRequest struct {
	PullRequest PullRequest      `json:"pull_request"`
	Repository  RepositoryRecord `json:"repository"`
}

// UserContribution represents one visible activity item on a user's profile.
// OccurredAt is empty for historical document versions because the current
// persistence model does not expose a version timestamp.
type UserContribution struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Title             string `json:"title"`
	DocumentID        string `json:"document_id,omitempty"`
	DocumentTitle     string `json:"document_title,omitempty"`
	RepositoryID      string `json:"repository_id,omitempty"`
	RepositoryName    string `json:"repository_name,omitempty"`
	PullRequestNumber int64  `json:"pull_request_number,omitempty"`
	State             string `json:"state,omitempty"`
	OccurredAt        string `json:"occurred_at,omitempty"`
}

type Anchor struct {
	Quote       string `json:"quote"`
	Prefix      string `json:"prefix"`
	Suffix      string `json:"suffix"`
	DOMPath     string `json:"dom_path"`
	StartOffset int    `json:"start_offset"`
	EndOffset   int    `json:"end_offset"`
}

type CLICredential struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Comment struct {
	ID              string           `json:"id"`
	DocumentID      string           `json:"document_id"`
	VersionID       string           `json:"version_id"`
	Author          auth.Principal   `json:"author"`
	Body            string           `json:"body"`
	SelectedText    string           `json:"selected_text"`
	Anchor          Anchor           `json:"anchor"`
	Status          string           `json:"status"`
	ParentCommentID string           `json:"parent_comment_id,omitempty"`
	RootCommentID   string           `json:"root_comment_id,omitempty"`
	Mentions        []CommentMention `json:"mentions"`
	Replies         []Comment        `json:"replies,omitempty"`
}

// CommentMention associates an exact UTF-16 range in a comment body with a
// concrete user or service-account principal. UTF-16 matches JavaScript string
// indexing, which is where mention ranges originate. The association grants no
// access; future notification delivery must re-check effective document access
// and apply recipient/rate limits before sending anything.
type CommentMention struct {
	Principal   auth.Principal `json:"principal"`
	OffsetUTF16 int64          `json:"offset"`
	LengthUTF16 int64          `json:"length"`
	// LabelText is the internal snapshot of the exact visible @label. It is not
	// exposed on the API; clients render the authoritative body slice.
	LabelText string `json:"-"`
}

// CommentUpdate distinguishes omitted mentions from an explicit empty list.
// See the PostgreSQL implementation for preservation/replacement semantics.
type CommentUpdate struct {
	Body     *string
	Status   string
	Mentions *[]CommentMention
}

type DocumentKVSnapshot struct {
	Revision int64                      `json:"revision"`
	Items    map[string]json.RawMessage `json:"items"`
}

type DocumentKVChange struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value,omitempty"`
	Deleted   bool            `json:"deleted,omitempty"`
	Revision  int64           `json:"revision"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	UpdatedBy *auth.Principal `json:"updated_by,omitempty"`
	Operation string          `json:"operation,omitempty"`
}

type DocumentKVPatchResult struct {
	Revision  int64              `json:"revision"`
	Changes   []DocumentKVChange `json:"changes"`
	UpdatedBy *auth.Principal    `json:"updated_by,omitempty"`
}

type DocumentKVEvent struct {
	DocumentID string             `json:"document_id"`
	Revision   int64              `json:"revision"`
	Changes    []DocumentKVChange `json:"changes"`
	UpdatedAt  string             `json:"updated_at,omitempty"`
	UpdatedBy  *auth.Principal    `json:"updated_by,omitempty"`
}

// Repository is the application persistence boundary, implemented by the
// PostgreSQL store in repo/postgres. Tests exercise that same implementation
// against a disposable database (see api/internal/testdb) rather than an
// in-memory fake, so test behaviour cannot drift from production.
type Repository interface {
	ResolveBearerToken(ctx context.Context, tokenHash string) (auth.Principal, error)
	ResolveUser(ctx context.Context, id string) (auth.Principal, error)
	UpsertGoogleUser(ctx context.Context, id, email, name, googleSub, pictureURL string) (auth.Principal, error)
	CreateCLICredential(ctx context.Context, userID, name, tokenHash string) (string, error)
	ListCLICredentials(ctx context.Context, userID string) ([]CLICredential, error)
	RevokeCLICredential(ctx context.Context, userID, credentialID string) error
	// RoleForDocument and RoleForFolder return the principal's effective role
	// on the resource. RoleNone (never an error) means the resource is missing
	// or unshared -- the same permission verdict either way; a non-nil error is
	// always a lookup fault, so handlers can map it to a 500 rather than a 403.
	RoleForDocument(ctx context.Context, principal auth.Principal, documentID string) (Role, error)
	RoleForFolder(ctx context.Context, principal auth.Principal, folderID string) (Role, error)
	// FolderAccessChain returns a folder's ancestor chain (root first, the
	// folder itself last) with the principal's effective role at each level,
	// resolved in one query. Returns ErrNotFound when the folder is missing.
	FolderAccessChain(ctx context.Context, principal auth.Principal, folderID string) ([]FolderAccess, error)
	CreateDocument(ctx context.Context, owner auth.Principal, title string, html []byte) (Document, Version, error)
	ListDocuments(ctx context.Context, principal auth.Principal) ([]Document, error)
	SearchDocumentsByTitle(ctx context.Context, principal auth.Principal, query string, includeOrgWide bool, limit int32) ([]Document, error)
	// RecordDocumentOpened notes that a principal opened a document, so a
	// broadly-shared ("anyone") document joins their workspace listing after
	// first open. Idempotent.
	RecordDocumentOpened(ctx context.Context, documentID string, principal auth.Principal) error
	GetDocument(ctx context.Context, id string) (Document, error)
	UpdateDocument(ctx context.Context, id, title string) (Document, error)
	DeleteDocument(ctx context.Context, id string) error
	CreateServiceAccount(ctx context.Context, owner auth.Principal, name string) (ServiceAccount, error)
	ListServiceAccounts(ctx context.Context, owner auth.Principal) ([]ServiceAccount, error)
	GetServiceAccount(ctx context.Context, id string) (ServiceAccount, error)
	GetServiceAccountByName(ctx context.Context, name string) (ServiceAccount, error)
	UpdateServiceAccount(ctx context.Context, id, name string, disabled *bool) (ServiceAccount, error)
	CreateServiceAccountKey(ctx context.Context, saID, name, tokenHash string) (string, error)
	ListServiceAccountKeys(ctx context.Context, saID string) ([]ServiceAccountKey, error)
	RevokeServiceAccountKey(ctx context.Context, saID, keyID string) error
	CreateOwnershipTransfer(ctx context.Context, saID string, from auth.Principal, toEmail string) (OwnershipTransfer, error)
	ListOwnershipTransfers(ctx context.Context, user auth.Principal) ([]OwnershipTransfer, error)
	AcceptOwnershipTransfer(ctx context.Context, id string, user auth.Principal) (OwnershipTransfer, error)
	DeclineOwnershipTransfer(ctx context.Context, id string, user auth.Principal) error
	PrincipalExists(ctx context.Context, principal auth.Principal) (bool, error)
	UserByEmail(ctx context.Context, email string) (auth.Principal, error)
	SearchUsersByEmail(ctx context.Context, query string) ([]auth.Principal, error)
	EnsureUserByEmail(ctx context.Context, email string) (auth.Principal, error)
	CreateGrant(ctx context.Context, resourceType, resourceID string, principal auth.Principal, role Role, grantedBy auth.Principal) (Grant, error)
	ListGrants(ctx context.Context, documentID string) ([]Grant, error)
	UpdateGrant(ctx context.Context, documentID, grantID string, role Role) (Grant, error)
	DeleteGrant(ctx context.Context, documentID, grantID string) error
	CreateVersion(ctx context.Context, documentID, baseVersionID, changeSummary string, html []byte, createdBy auth.Principal) (Version, error)
	ListVersions(ctx context.Context, documentID string) ([]Version, error)
	GetVersion(ctx context.Context, id string) (Version, error)
	GetVersionHTML(ctx context.Context, id string) ([]byte, error)
	CreateComment(ctx context.Context, documentID, versionID, body string, anchor Anchor, author auth.Principal, mentions []CommentMention) (Comment, error)
	CreateCommentReply(ctx context.Context, documentID, parentCommentID, body string, author auth.Principal, mentions []CommentMention) (Comment, error)
	GetComment(ctx context.Context, id string) (Comment, error)
	ListComments(ctx context.Context, documentID, status, versionID string) ([]Comment, error)
	UpdateComment(ctx context.Context, id string, update CommentUpdate) (Comment, error)
	DeleteComment(ctx context.Context, id string) error
	ListDocumentKV(ctx context.Context, documentID string) (DocumentKVSnapshot, error)
	ListDocumentKVEvents(ctx context.Context, documentID string, afterRevision int64, limit int32) ([]DocumentKVEvent, error)
	PatchDocumentKV(ctx context.Context, documentID string, set map[string]json.RawMessage, deleteKeys []string, actor auth.Principal) (DocumentKVPatchResult, error)
	CreateFolder(ctx context.Context, owner auth.Principal, title, parentFolderID string) (Folder, error)
	ListFolder(ctx context.Context, principal auth.Principal, folderID, path string) ([]FolderEntry, bool, *Folder, error)
	SearchFoldersByTitle(ctx context.Context, principal auth.Principal, query string, includeOrgWide bool, limit int32) ([]Folder, error)
	RecordFolderOpened(ctx context.Context, folderID string, principal auth.Principal) error
	GetFolder(ctx context.Context, id string) (Folder, error)
	UpdateFolder(ctx context.Context, id, title string, parentFolderID *string) (Folder, error)
	DeleteFolder(ctx context.Context, id string, principal auth.Principal) error
	FolderHasGrants(ctx context.Context, folderID string) (bool, error)
	MoveDocumentToFolder(ctx context.Context, principal auth.Principal, folderID, documentID string) (bool, error)
	MoveDocumentToRoot(ctx context.Context, folderID, documentID string) error
	ListFolderDocuments(ctx context.Context, principal auth.Principal, folderID string) ([]Document, error)
	ListEffectiveAccess(ctx context.Context, resourceType, resourceID string) ([]EffectiveAccessItem, error)
	ListPins(ctx context.Context, principal auth.Principal) ([]Pin, error)
	PutPin(ctx context.Context, principal auth.Principal, resourceType, resourceID string) error
	DeletePin(ctx context.Context, principal auth.Principal, resourceType, resourceID string) error
	CreateRepository(ctx context.Context, creator auth.Principal, params CreateRepositoryParams) (RepositoryRecord, error)
	ImportRepository(ctx context.Context, creator auth.Principal, params CreateRepositoryParams, kind string, files []RepositoryImportFile) (RepositoryImportResult, error)
	ListRepositories(ctx context.Context, principal auth.Principal) ([]RepositoryRecord, error)
	GetRepository(ctx context.Context, id string) (RepositoryRecord, error)
	DeleteRepository(ctx context.Context, id string, principal auth.Principal) error
	TransferRepositoryOwnership(ctx context.Context, id string, from, to auth.Principal) (RepositoryRecord, error)
	RoleForRepository(ctx context.Context, principal auth.Principal, repositoryID string) (Role, error)
	RepositoryForDocument(ctx context.Context, documentID string) (RepositoryRecord, error)
	RepositoryForFolder(ctx context.Context, folderID string) (RepositoryRecord, error)
	ListRepositoryDocuments(ctx context.Context, principal auth.Principal, repositoryID string) ([]Document, error)
	CreateRepositoryDocument(ctx context.Context, repositoryID string, creator auth.Principal, title string, html []byte) (Document, Version, error)
	CreatePullRequest(ctx context.Context, repositoryID, documentID, baseVersionID, title, body, changeSummary string, html []byte, author auth.Principal) (PullRequest, error)
	CreatePullRequestChangeSet(ctx context.Context, repositoryID, title, body, changeSummary string, changes []PullRequestChangeInput, author auth.Principal) (PullRequest, error)
	CreatePullRequestPatchset(ctx context.Context, repositoryID string, number int64, changeSummary string, changes []PullRequestChangeInput, author auth.Principal) (PullRequest, error)
	RebasePullRequest(ctx context.Context, repositoryID string, number int64, author auth.Principal) (PullRequest, error)
	ListPullRequests(ctx context.Context, repositoryID string) ([]PullRequest, error)
	GetPullRequest(ctx context.Context, repositoryID string, number int64) (PullRequest, error)
	ReviewPullRequest(ctx context.Context, repositoryID string, number int64, reviewer auth.Principal, state, body string) (PullRequest, error)
	MergePullRequest(ctx context.Context, repositoryID string, number int64, mergedBy auth.Principal) (PullRequest, error)
	ClosePullRequest(ctx context.Context, repositoryID string, number int64, closedBy auth.Principal) (PullRequest, error)
}

type Memory struct {
	mu          sync.Mutex
	docN        int
	verN        int
	saN         int
	grantN      int
	commentN    int
	credN       int
	sakN        int
	folderN     int
	repoN       int
	prN         int
	reviewN     int
	docs        map[string]Document
	folders     map[string]Folder
	pins        map[string]Pin
	versions    map[string]Version
	versionHTML map[string][]byte
	grants      []Grant
	users       map[string]auth.Principal
	creds       map[string]auth.Principal
	cliCreds    []cliCredRecord
	sas         map[string]ServiceAccount
	comments    map[string]Comment
	kv          map[string]map[string]DocumentKVChange
	kvRev       map[string]int64
	kvEvents    map[string][]DocumentKVEvent
	transfers   map[string]OwnershipTransfer
	repos       map[string]RepositoryRecord
	prs         map[string]PullRequest

	docWorkspace    map[string]map[string]bool // docID -> "type:id" -> true
	folderWorkspace map[string]map[string]bool // folderID -> "type:id" -> true
}

var _ Repository = (*Memory)(nil)

type cliCredRecord struct {
	ID        string
	UserID    string
	TokenHash string
	Name      string
}

// NewMemory returns an empty in-memory repository for the local development
// server. Production and repository tests use PostgreSQL.
func NewMemory() *Memory {
	return &Memory{docs: map[string]Document{}, folders: map[string]Folder{}, pins: map[string]Pin{}, versions: map[string]Version{}, versionHTML: map[string][]byte{}, users: map[string]auth.Principal{}, creds: map[string]auth.Principal{}, sas: map[string]ServiceAccount{}, comments: map[string]Comment{}, kv: map[string]map[string]DocumentKVChange{}, kvRev: map[string]int64{}, kvEvents: map[string][]DocumentKVEvent{}, transfers: map[string]OwnershipTransfer{}, repos: map[string]RepositoryRecord{}, prs: map[string]PullRequest{}}
}

func (m *Memory) ResolveBearerToken(ctx context.Context, tokenHash string) (auth.Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.creds[tokenHash]
	if !ok {
		return auth.Principal{}, ErrNotFound
	}
	return p, nil
}

func (m *Memory) ResolveUser(ctx context.Context, id string) (auth.Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.users[id]
	if !ok {
		return auth.Principal{}, ErrNotFound
	}
	return p, nil
}

func (m *Memory) UpsertGoogleUser(ctx context.Context, id, email, name, googleSub, pictureURL string) (auth.Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	for existingID, existing := range m.users {
		if strings.EqualFold(existing.Email, email) {
			existing.Email = email
			existing.Name = name
			existing.PictureURL = pictureURL
			m.users[existingID] = existing
			return existing, nil
		}
	}
	p := auth.Principal{Type: auth.PrincipalUser, ID: id, Email: email, Name: name, PictureURL: pictureURL}
	m.users[id] = p
	return p, nil
}

func (m *Memory) CreateCLICredential(ctx context.Context, userID, name, tokenHash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.users[userID]
	if !ok {
		p = auth.Principal{Type: auth.PrincipalUser, ID: userID}
	}
	m.credN++
	id := fmt.Sprintf("cred_%d", m.credN)
	m.creds[tokenHash] = p
	m.cliCreds = append(m.cliCreds, cliCredRecord{ID: id, UserID: userID, TokenHash: tokenHash, Name: name})
	return id, nil
}

func (m *Memory) ListCLICredentials(ctx context.Context, userID string) ([]CLICredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []CLICredential{}
	for _, c := range m.cliCreds {
		if c.UserID == userID {
			out = append(out, CLICredential{ID: c.ID, Name: c.Name})
		}
	}
	return out, nil
}
func (m *Memory) RevokeCLICredential(ctx context.Context, userID, credentialID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.cliCreds {
		if c.ID == credentialID && c.UserID == userID {
			delete(m.creds, c.TokenHash)
			m.cliCreds = append(m.cliCreds[:i], m.cliCreds[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *Memory) RoleForDocument(ctx context.Context, p auth.Principal, documentID string) (Role, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.docs[documentID]; !ok {
		return RoleNone, nil
	}
	best := RoleNone
	if d := m.docs[documentID]; p.Type == d.Owner.Type && p.ID == d.Owner.ID {
		best = RoleOwner
	}
	ids := map[string]bool{documentID: true}
	for fid := m.docs[documentID].ParentFolderID; fid != ""; fid = m.folders[fid].ParentFolderID {
		ids[fid] = true
	}
	for _, g := range m.grants {
		if !ids[g.ResourceID] && !ids[g.DocumentID] {
			continue
		}
		if (g.Principal.Type == p.Type && g.Principal.ID == p.ID) || g.Principal.Type == auth.PrincipalAnyone {
			best = MaxRole(best, g.Role)
		}
	}
	if repository, wrapped := m.repositoryForDocumentLocked(documentID); wrapped && repository.Scope == RepositoryScopeOrganization && p.Type != auth.PrincipalAnonymous && p.ID != "" {
		best = MaxRole(best, RoleViewer)
	}
	return best, nil
}

func (m *Memory) CreateDocument(ctx context.Context, owner auth.Principal, title string, html []byte) (Document, Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	documentID := m.nextDocumentIDLocked()
	versionID := m.nextVersionIDLocked()
	d := Document{ID: documentID, Title: title, Owner: owner, CurrentVersionID: versionID}
	v := Version{ID: d.CurrentVersionID, Number: 1, DocumentID: d.ID, CreatedBy: owner, SHA256: fmt.Sprintf("sha_%d", m.verN)}
	m.docs[d.ID] = d
	m.versions[v.ID] = v
	m.versionHTML[v.ID] = append([]byte(nil), html...)
	return d, v, nil
}

func (m *Memory) nextDocumentIDLocked() string {
	for {
		m.docN++
		id := fmt.Sprintf("doc_%d", m.docN)
		if _, exists := m.docs[id]; !exists {
			return id
		}
	}
}

func (m *Memory) GetDocument(ctx context.Context, id string) (Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[id]
	if !ok {
		return Document{}, ErrNotFound
	}
	return d, nil
}
func (m *Memory) CreateServiceAccount(ctx context.Context, owner auth.Principal, name string) (ServiceAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sa := range m.sas {
		if sa.Name == name {
			return ServiceAccount{}, ErrConflict
		}
	}
	m.saN++
	sa := ServiceAccount{ID: fmt.Sprintf("sa_%d", m.saN), Name: name, Owner: owner}
	m.sas[sa.ID] = sa
	return sa, nil
}
func (m *Memory) GetServiceAccount(ctx context.Context, id string) (ServiceAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sa, ok := m.sas[id]
	if !ok {
		return ServiceAccount{}, ErrNotFound
	}
	return sa, nil
}
func (m *Memory) GetServiceAccountByName(ctx context.Context, name string) (ServiceAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sa := range m.sas {
		if sa.Name == name {
			return sa, nil
		}
	}
	return ServiceAccount{}, ErrNotFound
}
func (m *Memory) PrincipalExists(ctx context.Context, p auth.Principal) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch p.Type {
	case auth.PrincipalUser:
		_, ok := m.users[p.ID]
		return ok, nil
	case auth.PrincipalServiceAccount:
		_, ok := m.sas[p.ID]
		return ok, nil
	}
	return false, nil
}
func (m *Memory) UserByEmail(ctx context.Context, email string) (auth.Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	for _, p := range m.users {
		if strings.EqualFold(p.Email, email) {
			return p, nil
		}
	}
	return auth.Principal{}, ErrNotFound
}

func (m *Memory) SearchUsersByEmail(ctx context.Context, query string) ([]auth.Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := []auth.Principal{}
	for _, p := range m.users {
		if q == "" || strings.Contains(strings.ToLower(p.Email), q) || strings.Contains(strings.ToLower(p.Name), q) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *Memory) EnsureUserByEmail(_ context.Context, email string) (auth.Principal, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, principal := range m.users {
		if strings.EqualFold(principal.Email, email) {
			principal.Email = email
			m.users[principal.ID] = principal
			return principal, nil
		}
	}
	id := fmt.Sprintf("usr_%d", len(m.users)+1)
	p := auth.Principal{Type: auth.PrincipalUser, ID: id, Email: email}
	m.users[id] = p
	return p, nil
}
func (m *Memory) CreateGrant(ctx context.Context, resourceType, resourceID string, principal auth.Principal, role Role, grantedBy auth.Principal) (Grant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch principal.Type {
	case auth.PrincipalUser:
		if canonical, ok := m.users[principal.ID]; ok {
			principal = canonical
		}
	case auth.PrincipalServiceAccount:
		if serviceAccount, ok := m.sas[principal.ID]; ok {
			principal = auth.Principal{Type: auth.PrincipalServiceAccount, ID: serviceAccount.ID, Name: serviceAccount.Name}
		}
	case auth.PrincipalAnyone:
		principal = auth.Principal{Type: auth.PrincipalAnyone}
	}
	m.grantN++
	g := Grant{ID: fmt.Sprintf("gr_%d", m.grantN), DocumentID: resourceID, ResourceType: resourceType, ResourceID: resourceID, Principal: principal, Role: role, GrantedBy: grantedBy}
	m.grants = append(m.grants, g)
	return g, nil
}
func (m *Memory) CreateVersion(ctx context.Context, documentID, baseVersionID, changeSummary string, html []byte, createdBy auth.Principal) (Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[documentID]
	if !ok {
		return Version{}, ErrNotFound
	}
	if repository, ok := m.repositoryForDocumentLocked(documentID); ok && repository.Protected {
		return Version{}, &PullRequestRequiredError{RepositoryID: repository.ID}
	}
	if baseVersionID != d.CurrentVersionID {
		return Version{}, &VersionConflictError{LatestVersionID: d.CurrentVersionID}
	}
	versionID := m.nextVersionIDLocked()
	v := Version{ID: versionID, Number: m.verN, DocumentID: documentID, CreatedBy: createdBy, ChangeSummary: changeSummary, SHA256: fmt.Sprintf("sha_%d", m.verN)}
	d.CurrentVersionID = v.ID
	m.docs[documentID] = d
	m.versions[v.ID] = v
	m.versionHTML[v.ID] = append([]byte(nil), html...)
	return v, nil
}
func (m *Memory) CreateComment(ctx context.Context, documentID, versionID, body string, anchor Anchor, author auth.Principal, mentions []CommentMention) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body = NormalizeCommentBody(body)
	if body == "" {
		return Comment{}, ErrInvalidCommentBody
	}
	v, ok := m.versions[versionID]
	if !ok || v.DocumentID != documentID {
		return Comment{}, ErrNotFound
	}
	mentions, err := m.prepareCommentMentionsLocked(body, mentions)
	if err != nil {
		return Comment{}, err
	}
	m.commentN++
	cm := Comment{ID: fmt.Sprintf("cmt_%d", m.commentN), DocumentID: documentID, VersionID: versionID, Author: author, Body: body, SelectedText: anchor.Quote, Anchor: anchor, Status: StatusOpen, Mentions: mentions}
	cm.RootCommentID = cm.ID
	m.comments[cm.ID] = cm
	return cm, nil
}
func (m *Memory) CreateCommentReply(ctx context.Context, documentID, parentCommentID, body string, author auth.Principal, mentions []CommentMention) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body = NormalizeCommentBody(body)
	if body == "" {
		return Comment{}, ErrInvalidCommentBody
	}
	parent, ok := m.comments[parentCommentID]
	if !ok || parent.DocumentID != documentID {
		return Comment{}, ErrNotFound
	}
	mentions, err := m.prepareCommentMentionsLocked(body, mentions)
	if err != nil {
		return Comment{}, err
	}
	rootID := parent.RootCommentID
	if rootID == "" {
		rootID = parent.ID
	}
	var id string
	for {
		m.commentN++
		id = fmt.Sprintf("cmt_%d", m.commentN)
		if _, exists := m.comments[id]; !exists {
			break
		}
	}
	cm := Comment{ID: id, DocumentID: documentID, VersionID: parent.VersionID, Author: author, Body: body, Status: StatusOpen, ParentCommentID: parent.ID, RootCommentID: rootID, Mentions: mentions}
	m.comments[cm.ID] = cm
	return cm, nil
}

func (m *Memory) prepareCommentMentionsLocked(body string, mentions []CommentMention) ([]CommentMention, error) {
	ordered, err := ValidateCommentMentionRanges(body, mentions)
	if err != nil {
		return nil, err
	}
	for i := range ordered {
		principal := ordered[i].Principal
		switch principal.Type {
		case auth.PrincipalUser:
			canonical, ok := m.users[principal.ID]
			if !ok {
				return nil, fmt.Errorf("%w: principal does not exist", ErrInvalidCommentMention)
			}
			ordered[i].Principal = canonical
		case auth.PrincipalServiceAccount:
			serviceAccount, ok := m.sas[principal.ID]
			if !ok {
				return nil, fmt.Errorf("%w: principal does not exist", ErrInvalidCommentMention)
			}
			ordered[i].Principal = auth.Principal{Type: auth.PrincipalServiceAccount, ID: serviceAccount.ID, Name: serviceAccount.Name}
		default:
			return nil, fmt.Errorf("%w: principal type must be user or service_account", ErrInvalidCommentMention)
		}
		if !CommentMentionMatchesCanonicalLabel(ordered[i]) {
			return nil, fmt.Errorf("%w: range does not exactly match the principal label", ErrInvalidCommentMention)
		}
	}
	return ordered, nil
}
func (m *Memory) GetComment(ctx context.Context, id string) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cm, ok := m.comments[id]
	if !ok {
		return Comment{}, ErrNotFound
	}
	return cm, nil
}

func (m *Memory) ListDocuments(ctx context.Context, principal auth.Principal) ([]Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Document{}
	for _, d := range m.docs {
		if d.ParentFolderID != "" {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}
func (m *Memory) SearchDocumentsByTitle(ctx context.Context, principal auth.Principal, query string, includeOrgWide bool, limit int32) ([]Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	query = strings.ToLower(query)
	out := []Document{}
	for _, d := range m.docs {
		if query == "" || strings.Contains(strings.ToLower(d.Title), query) {
			out = append(out, d)
		}
		if limit > 0 && len(out) >= int(limit) {
			break
		}
	}
	return out, nil
}
func (m *Memory) RecordDocumentOpened(ctx context.Context, documentID string, principal auth.Principal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.docWorkspace == nil {
		m.docWorkspace = map[string]map[string]bool{}
	}
	if m.docWorkspace[documentID] == nil {
		m.docWorkspace[documentID] = map[string]bool{}
	}
	m.docWorkspace[documentID][string(principal.Type)+":"+principal.ID] = true
	return nil
}
func (m *Memory) UpdateDocument(ctx context.Context, id, title string) (Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[id]
	if !ok {
		return Document{}, ErrNotFound
	}
	if repository, wrapped := m.repositoryForDocumentLocked(id); wrapped && repository.Protected {
		return Document{}, &PullRequestRequiredError{RepositoryID: repository.ID}
	}
	if title != "" {
		d.Title = title
	}
	m.docs[id] = d
	return d, nil
}
func (m *Memory) DeleteDocument(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.docs[id]; !ok {
		return ErrNotFound
	}
	if repository, wrapped := m.repositoryForDocumentLocked(id); wrapped && repository.Protected {
		return &PullRequestRequiredError{RepositoryID: repository.ID}
	}
	delete(m.docs, id)
	delete(m.kv, id)
	delete(m.kvRev, id)
	delete(m.kvEvents, id)
	return nil
}
func (m *Memory) ListServiceAccounts(ctx context.Context, owner auth.Principal) ([]ServiceAccount, error) {
	return []ServiceAccount{}, nil
}
func (m *Memory) UpdateServiceAccount(ctx context.Context, id, name string, disabled *bool) (ServiceAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sa, ok := m.sas[id]
	if !ok {
		return ServiceAccount{}, ErrNotFound
	}
	if name != "" {
		sa.Name = name
	}
	if disabled != nil {
		sa.Disabled = *disabled
	}
	m.sas[id] = sa
	return sa, nil
}
func (m *Memory) CreateServiceAccountKey(ctx context.Context, saID, name, tokenHash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sakN++
	id := fmt.Sprintf("sak_%d", m.sakN)
	// The principal represents the service account identity; its Name is the
	// service account's name (not the SA id, and not the key name).
	saName := ""
	if sa, ok := m.sas[saID]; ok {
		saName = sa.Name
	}
	m.creds[tokenHash] = auth.Principal{Type: auth.PrincipalServiceAccount, ID: saID, Name: saName}
	return id, nil
}
func (m *Memory) ListServiceAccountKeys(ctx context.Context, saID string) ([]ServiceAccountKey, error) {
	return []ServiceAccountKey{}, nil
}
func (m *Memory) RevokeServiceAccountKey(ctx context.Context, saID, keyID string) error { return nil }
func (m *Memory) ListGrants(ctx context.Context, resourceID string) ([]Grant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Grant{}
	for _, g := range m.grants {
		if g.ResourceID == resourceID || g.DocumentID == resourceID {
			out = append(out, g)
		}
	}
	return out, nil
}
func (m *Memory) UpdateGrant(ctx context.Context, resourceID, grantID string, role Role) (Grant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, g := range m.grants {
		if g.ID == grantID && (g.ResourceID == resourceID || g.DocumentID == resourceID) {
			m.grants[i].Role = role
			return m.grants[i], nil
		}
	}
	return Grant{}, ErrNotFound
}
func (m *Memory) DeleteGrant(ctx context.Context, resourceID, grantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, g := range m.grants {
		if g.ID == grantID && (g.ResourceID == resourceID || g.DocumentID == resourceID) {
			m.grants = append(m.grants[:i], m.grants[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}
func (m *Memory) ListVersions(ctx context.Context, documentID string) ([]Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Version{}
	for _, v := range m.versions {
		if v.DocumentID == documentID && v.Number > 0 {
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *Memory) GetVersion(ctx context.Context, id string) (Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.versions[id]
	if !ok {
		return Version{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) GetVersionHTML(ctx context.Context, id string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	html, ok := m.versionHTML[id]
	if ok {
		return append([]byte(nil), html...), nil
	}
	if _, ok := m.versions[id]; ok {
		// Test fixtures created before in-memory HTML persistence seeded version
		// metadata directly. Keep those records renderable without weakening the
		// not-found behavior for unknown version IDs.
		return []byte("<html></html>"), nil
	}
	return nil, ErrNotFound
}

func (m *Memory) nextVersionIDLocked() string {
	for {
		m.verN++
		id := fmt.Sprintf("ver_%d", m.verN)
		if _, exists := m.versions[id]; !exists {
			return id
		}
	}
}
func (m *Memory) ListComments(ctx context.Context, documentID, status, versionID string) ([]Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Comment{}
	for _, cm := range m.comments {
		if cm.DocumentID != documentID {
			continue
		}
		if status != "" && cm.Status != status {
			continue
		}
		if versionID != "" && cm.VersionID != versionID {
			continue
		}
		out = append(out, cm)
	}
	return out, nil
}
func (m *Memory) UpdateComment(ctx context.Context, id string, update CommentUpdate) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cm, ok := m.comments[id]
	if !ok {
		return Comment{}, ErrNotFound
	}
	bodyChanged := false
	if update.Body != nil {
		body := NormalizeCommentBody(*update.Body)
		if body == "" {
			return Comment{}, ErrInvalidCommentBody
		}
		bodyChanged = body != cm.Body
		cm.Body = body
	}
	if update.Status != "" {
		cm.Status = update.Status
	}
	if update.Mentions != nil {
		mentions, err := m.prepareCommentMentionsLocked(cm.Body, *update.Mentions)
		if err != nil {
			return Comment{}, err
		}
		cm.Mentions = mentions
	} else if bodyChanged {
		cm.Mentions = []CommentMention{}
	}
	m.comments[id] = cm
	return cm, nil
}
func (m *Memory) DeleteComment(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.comments, id)
	return nil
}

func (m *Memory) ListDocumentKV(ctx context.Context, documentID string) (DocumentKVSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.docs[documentID]; !ok {
		return DocumentKVSnapshot{}, ErrNotFound
	}
	items := map[string]json.RawMessage{}
	for key, item := range m.kv[documentID] {
		if item.Deleted {
			continue
		}
		items[key] = append(json.RawMessage(nil), item.Value...)
	}
	return DocumentKVSnapshot{Revision: m.kvRev[documentID], Items: items}, nil
}

func (m *Memory) ListDocumentKVEvents(ctx context.Context, documentID string, afterRevision int64, limit int32) ([]DocumentKVEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.docs[documentID]; !ok {
		return nil, ErrNotFound
	}
	if limit <= 0 {
		limit = 100
	}
	events := m.kvEvents[documentID]
	out := make([]DocumentKVEvent, 0, min(int(limit), len(events)))
	for _, ev := range events {
		if ev.Revision <= afterRevision {
			continue
		}
		out = append(out, cloneDocumentKVEvent(ev))
		if len(out) >= int(limit) {
			break
		}
	}
	return out, nil
}

func (m *Memory) PatchDocumentKV(ctx context.Context, documentID string, set map[string]json.RawMessage, deleteKeys []string, actor auth.Principal) (DocumentKVPatchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.docs[documentID]; !ok {
		return DocumentKVPatchResult{}, ErrNotFound
	}
	if len(set) == 0 && len(deleteKeys) == 0 {
		return DocumentKVPatchResult{Revision: m.kvRev[documentID], Changes: []DocumentKVChange{}, UpdatedBy: &actor}, nil
	}
	m.kvRev[documentID]++
	rev := m.kvRev[documentID]
	if m.kv[documentID] == nil {
		m.kv[documentID] = map[string]DocumentKVChange{}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	changes := make([]DocumentKVChange, 0, len(set)+len(deleteKeys))
	for _, key := range sortedKVKeys(set) {
		value := append(json.RawMessage(nil), set[key]...)
		ch := DocumentKVChange{Key: key, Value: value, Revision: rev, UpdatedAt: now, UpdatedBy: &actor, Operation: "set"}
		m.kv[documentID][key] = ch
		changes = append(changes, ch)
	}
	for _, key := range sortedStrings(deleteKeys) {
		ch := DocumentKVChange{Key: key, Deleted: true, Revision: rev, UpdatedAt: now, UpdatedBy: &actor, Operation: "delete"}
		m.kv[documentID][key] = ch
		changes = append(changes, ch)
	}
	m.kvEvents[documentID] = append(m.kvEvents[documentID], cloneDocumentKVEvent(DocumentKVEvent{
		DocumentID: documentID,
		Revision:   rev,
		Changes:    changes,
		UpdatedAt:  now,
		UpdatedBy:  &actor,
	}))
	return DocumentKVPatchResult{Revision: rev, Changes: changes, UpdatedBy: &actor}, nil
}

func cloneDocumentKVEvent(ev DocumentKVEvent) DocumentKVEvent {
	out := ev
	if ev.UpdatedBy != nil {
		actor := *ev.UpdatedBy
		out.UpdatedBy = &actor
	}
	out.Changes = make([]DocumentKVChange, len(ev.Changes))
	for i, ch := range ev.Changes {
		out.Changes[i] = cloneDocumentKVChange(ch)
	}
	return out
}

func cloneDocumentKVChange(ch DocumentKVChange) DocumentKVChange {
	out := ch
	if ch.Value != nil {
		out.Value = append(json.RawMessage(nil), ch.Value...)
	}
	if ch.UpdatedBy != nil {
		actor := *ch.UpdatedBy
		out.UpdatedBy = &actor
	}
	return out
}

func (m *Memory) CreateOwnershipTransfer(ctx context.Context, saID string, from auth.Principal, toEmail string) (OwnershipTransfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Resolve the target email to a stable user ID so ToUserID has the same
	// meaning as the Postgres implementation (which stores a resolved user id,
	// not an email). AcceptOwnershipTransfer compares ToUserID to user.ID.
	toID := m.resolveUserIDByEmailLocked(toEmail)
	id := fmt.Sprintf("xfer_%d", len(m.transfers)+1)
	x := OwnershipTransfer{ID: id, ServiceAccountID: saID, FromUserID: from.ID, ToUserID: toID, Status: StatusPending}
	m.transfers[id] = x
	return x, nil
}

// resolveUserIDByEmailLocked returns the id of an existing user with the given
// email, creating a placeholder user if none exists. Caller must hold m.mu.
func (m *Memory) resolveUserIDByEmailLocked(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, p := range m.users {
		if strings.EqualFold(p.Email, email) {
			return p.ID
		}
	}
	id := fmt.Sprintf("usr_%d", len(m.users)+1)
	m.users[id] = auth.Principal{Type: auth.PrincipalUser, ID: id, Email: email}
	return id
}
func (m *Memory) ListOwnershipTransfers(ctx context.Context, user auth.Principal) ([]OwnershipTransfer, error) {
	return []OwnershipTransfer{}, nil
}
func (m *Memory) AcceptOwnershipTransfer(ctx context.Context, id string, user auth.Principal) (OwnershipTransfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	x, ok := m.transfers[id]
	if !ok {
		return OwnershipTransfer{}, ErrNotFound
	}
	if x.ToUserID != user.ID {
		return x, ErrNotTransferTarget
	}
	if x.Status != StatusPending {
		return x, ErrTransferNotPending
	}
	x.Status = StatusAccepted
	m.transfers[id] = x
	return x, nil
}
func (m *Memory) DeclineOwnershipTransfer(ctx context.Context, id string, user auth.Principal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	x, ok := m.transfers[id]
	if !ok || (x.FromUserID != user.ID && x.ToUserID != user.ID) {
		return ErrNotFound
	}
	x.Status = StatusDeclined
	m.transfers[id] = x
	return nil
}

func (m *Memory) folderDepthLocked(parent string) int {
	d := 0
	for parent != "" {
		d++
		parent = m.folders[parent].ParentFolderID
	}
	return d
}
func (m *Memory) subtreeHeightLocked(id string) int {
	h := 1
	for _, f := range m.folders {
		if f.ParentFolderID == id {
			if ch := 1 + m.subtreeHeightLocked(f.ID); ch > h {
				h = ch
			}
		}
	}
	return h
}
func (m *Memory) folderConflictLocked(owner auth.Principal, parent, title, except string) bool {
	for _, f := range m.folders {
		if f.ID != except && f.Owner.Type == owner.Type && f.Owner.ID == owner.ID && f.ParentFolderID == parent && strings.EqualFold(f.Title, title) {
			return true
		}
	}
	return false
}

func (m *Memory) CreateFolder(ctx context.Context, owner auth.Principal, title, parentFolderID string) (Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if parentFolderID != "" {
		if _, ok := m.folders[parentFolderID]; !ok {
			return Folder{}, ErrNotFound
		}
		if repository, wrapped := m.repositoryForFolderLocked(parentFolderID); wrapped && repository.Protected {
			return Folder{}, ErrRepositoryBoundary
		}
	}
	if m.folderDepthLocked(parentFolderID)+1 > 5 {
		return Folder{}, ErrConflict
	}
	if m.folderConflictLocked(owner, parentFolderID, title, "") {
		return Folder{}, ErrConflict
	}
	m.folderN++
	f := Folder{ID: fmt.Sprintf("fld_%d", m.folderN), Title: title, Owner: owner, ParentFolderID: parentFolderID}
	m.folders[f.ID] = f
	return f, nil
}
func (m *Memory) GetFolder(ctx context.Context, id string) (Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.folders[id]
	if !ok {
		return Folder{}, ErrNotFound
	}
	f.Ancestors = m.folderAncestors(f.ParentFolderID)
	return f, nil
}

func (m *Memory) folderAncestors(parentID string) []FolderBreadcrumb {
	var out []FolderBreadcrumb
	cur := parentID
	for i := 0; i < 10 && cur != ""; i++ {
		p, ok := m.folders[cur]
		if !ok {
			break
		}
		out = append([]FolderBreadcrumb{{ID: p.ID, Title: p.Title}}, out...)
		cur = p.ParentFolderID
	}
	return out
}
func (m *Memory) RoleForFolder(ctx context.Context, p auth.Principal, folderID string) (Role, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	role, err := m.roleForFolderLocked(p, folderID)
	if errors.Is(err, ErrNotFound) {
		return RoleNone, nil
	}
	return role, err
}

func (m *Memory) FolderAccessChain(ctx context.Context, p auth.Principal, folderID string) ([]FolderAccess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.folders[folderID]; !ok {
		return nil, ErrNotFound
	}
	ids := []string{}
	for id := folderID; id != ""; id = m.folders[id].ParentFolderID {
		ids = append(ids, id)
	}
	out := make([]FolderAccess, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		folder := m.folders[ids[i]]
		role, err := m.roleForFolderLocked(p, folder.ID)
		if errors.Is(err, ErrNotFound) {
			role = RoleNone
		} else if err != nil {
			return nil, err
		}
		out = append(out, FolderAccess{ID: folder.ID, Title: folder.Title, Role: role})
	}
	return out, nil
}

func (m *Memory) roleForFolderLocked(p auth.Principal, folderID string) (Role, error) {
	f, ok := m.folders[folderID]
	if !ok {
		return "", ErrNotFound
	}
	best := RoleNone
	if f.Owner.Type == p.Type && f.Owner.ID == p.ID {
		best = RoleOwner
	}
	ids := map[string]bool{}
	for fid := folderID; fid != ""; fid = m.folders[fid].ParentFolderID {
		ids[fid] = true
	}
	for _, g := range m.grants {
		if ids[g.ResourceID] && ((g.Principal.Type == p.Type && g.Principal.ID == p.ID) || g.Principal.Type == auth.PrincipalAnyone) {
			best = MaxRole(best, g.Role)
		}
	}
	if repository, wrapped := m.repositoryForFolderLocked(folderID); wrapped && repository.Scope == RepositoryScopeOrganization && p.Type != auth.PrincipalAnonymous && p.ID != "" {
		best = MaxRole(best, RoleViewer)
	}
	if best == RoleNone {
		return "", ErrNotFound
	}
	return best, nil
}

//nolint:gocyclo // Path resolution, ambiguity reporting, and listing stay together to preserve the public read contract.
func (m *Memory) ListFolder(ctx context.Context, p auth.Principal, folderID, path string) ([]FolderEntry, bool, *Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parent := folderID
	if path != "" && path != "/" {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		for i, part := range parts {
			found := ""
			if i == 0 && parent == "" {
				var candidates []Folder
				for _, f := range m.folders {
					if f.ParentFolderID == "" && strings.EqualFold(f.Title, part) {
						if repository, wrapped := m.repositoryForFolderLocked(f.ID); wrapped && repository.RootFolderID == f.ID {
							continue
						}
						visible := f.Owner.Type == p.Type && f.Owner.ID == p.ID
						if !visible {
							for _, g := range m.grants {
								if g.ResourceType == "folder" && g.ResourceID == f.ID {
									visible = true
									break
								}
							}
						}
						if visible {
							candidates = append(candidates, f)
						}
					}
				}
				if len(candidates) == 0 {
					return nil, false, nil, ErrNotFound
				}
				if len(candidates) > 1 {
					return nil, false, nil, &AmbiguousPathError{Segment: part, Candidates: candidates}
				}
				found = candidates[0].ID
			} else {
				for _, f := range m.folders {
					if f.ParentFolderID == parent && strings.EqualFold(f.Title, part) {
						found = f.ID
						break
					}
				}
			}
			if found == "" {
				return nil, false, nil, ErrNotFound
			}
			parent = found
		}
	}
	var cur *Folder
	if parent != "" {
		f := m.folders[parent]
		cur = &f
	}
	out := []FolderEntry{}
	has := false
	for _, f := range m.folders {
		if f.ParentFolderID == parent {
			if parent == "" {
				if repository, wrapped := m.repositoryForFolderLocked(f.ID); wrapped && repository.RootFolderID == f.ID {
					continue
				}
			}
			if parent == "" && len(out) >= 50 {
				has = true
				continue
			}
			out = append(out, FolderEntry{Type: "folder", Folder: &f})
		}
	}
	if parent != "" {
		for _, d := range m.docs {
			if d.ParentFolderID == parent {
				dd := d
				out = append(out, FolderEntry{Type: "document", Document: &dd})
			}
		}
	}
	return out, has, cur, nil
}

//nolint:gocyclo // Fixed-point descendant discovery is intentionally linear and side-effect free.
func (m *Memory) SearchFoldersByTitle(ctx context.Context, p auth.Principal, query string, includeOrgWide bool, limit int32) ([]Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	accessible := map[string]bool{}
	for id, f := range m.folders {
		if f.Owner.Type == p.Type && f.Owner.ID == p.ID {
			accessible[id] = true
		}
	}
	for _, g := range m.grants {
		if g.ResourceType == "folder" {
			if (g.Principal.Type == p.Type && g.Principal.ID == p.ID) || (includeOrgWide && g.Principal.Type == "anyone") {
				accessible[g.ResourceID] = true
			}
		}
	}
	changed := true
	for changed {
		changed = false
		for id, f := range m.folders {
			if !accessible[id] && accessible[f.ParentFolderID] {
				accessible[id] = true
				changed = true
			}
		}
	}
	q := strings.ToLower(query)
	var out []Folder
	for id, f := range m.folders {
		if accessible[id] && strings.Contains(strings.ToLower(f.Title), q) {
			out = append(out, f)
			if limit > 0 && len(out) >= int(limit) {
				break
			}
		}
	}
	return out, nil
}
func (m *Memory) RecordFolderOpened(_ context.Context, folderID string, p auth.Principal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.folderWorkspace == nil {
		m.folderWorkspace = map[string]map[string]bool{}
	}
	if m.folderWorkspace[folderID] == nil {
		m.folderWorkspace[folderID] = map[string]bool{}
	}
	m.folderWorkspace[folderID][string(p.Type)+":"+p.ID] = true
	return nil
}

//nolint:gocyclo // The in-memory implementation mirrors the production atomic folder state transition.
func (m *Memory) UpdateFolder(ctx context.Context, id, title string, parent *string) (Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.folders[id]
	if !ok {
		return Folder{}, ErrNotFound
	}
	if repository, ok := m.repositoryForFolderLocked(id); ok && (repository.RootFolderID == id || repository.Protected) {
		return Folder{}, ErrRepositoryBoundary
	}
	np := f.ParentFolderID
	if parent != nil {
		np = *parent
		if np == id {
			return Folder{}, ErrConflict
		}
		for x := np; x != ""; x = m.folders[x].ParentFolderID {
			if x == id {
				return Folder{}, ErrConflict
			}
		}
		if np != "" {
			if _, ok := m.folders[np]; !ok {
				return Folder{}, ErrNotFound
			}
		}
		sourceRepository, sourceWrapped := m.repositoryForFolderLocked(id)
		destinationRepository, destinationWrapped := m.repositoryForFolderLocked(np)
		if (sourceWrapped && sourceRepository.Protected) || (destinationWrapped && destinationRepository.Protected) {
			return Folder{}, ErrRepositoryBoundary
		}
		if sourceWrapped != destinationWrapped || (sourceWrapped && sourceRepository.ID != destinationRepository.ID) {
			return Folder{}, ErrRepositoryBoundary
		}
		if m.folderDepthLocked(np)+m.subtreeHeightLocked(id) > 5 {
			return Folder{}, ErrConflict
		}
	}
	nt := f.Title
	if title != "" {
		nt = title
	}
	if m.folderConflictLocked(f.Owner, np, nt, id) {
		return Folder{}, ErrConflict
	}
	f.Title = nt
	f.ParentFolderID = np
	m.folders[id] = f
	return f, nil
}
func (m *Memory) DeleteFolder(ctx context.Context, id string, p auth.Principal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	root, ok := m.folders[id]
	if !ok {
		return ErrNotFound
	}
	if _, ok := m.repositoryForFolderLocked(id); ok {
		return ErrRepositoryBoundary
	}
	if root.Owner.Type != p.Type || root.Owner.ID != p.ID {
		return ErrForbidden
	}
	var del func(string)
	del = func(fid string) {
		for _, d := range m.docs {
			if d.ParentFolderID == fid {
				d.ParentFolderID = ""
				m.docs[d.ID] = d
			}
		}
		for _, f := range m.folders {
			if f.ParentFolderID == fid {
				del(f.ID)
			}
		}
		delete(m.folders, fid)
	}
	del(id)
	return nil
}
func (m *Memory) FolderHasGrants(ctx context.Context, folderID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := folderID
	for depth := 0; depth < 20 && cur != ""; depth++ {
		for _, g := range m.grants {
			if g.ResourceID == cur && g.ResourceType == "folder" {
				return true, nil
			}
		}
		f, ok := m.folders[cur]
		if !ok {
			break
		}
		cur = f.ParentFolderID
	}
	return false, nil
}
func (m *Memory) MoveDocumentToFolder(ctx context.Context, p auth.Principal, folderID, docID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[docID]
	if !ok {
		return false, ErrNotFound
	}
	if _, ok := m.folders[folderID]; !ok {
		return false, ErrNotFound
	}
	sourceRepository, sourceWrapped := m.repositoryForDocumentLocked(docID)
	destinationRepository, destinationWrapped := m.repositoryForFolderLocked(folderID)
	if (sourceWrapped && sourceRepository.Protected) || (destinationWrapped && destinationRepository.Protected) {
		return false, ErrRepositoryBoundary
	}
	if sourceWrapped != destinationWrapped || (sourceWrapped && sourceRepository.ID != destinationRepository.ID) {
		return false, ErrRepositoryBoundary
	}
	shared := false
	for _, g := range m.grants {
		if g.ResourceID == folderID {
			shared = true
		}
	}
	d.ParentFolderID = folderID
	m.docs[docID] = d
	return shared, nil
}
func (m *Memory) MoveDocumentToRoot(ctx context.Context, folderID, docID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[docID]
	if !ok || d.ParentFolderID != folderID {
		return ErrNotFound
	}
	if _, wrapped := m.repositoryForDocumentLocked(docID); wrapped {
		return ErrRepositoryBoundary
	}
	d.ParentFolderID = ""
	m.docs[docID] = d
	return nil
}
func (m *Memory) ListFolderDocuments(ctx context.Context, p auth.Principal, folderID string) ([]Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Document{}
	for _, d := range m.docs {
		if d.ParentFolderID == folderID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (m *Memory) ListEffectiveAccess(ctx context.Context, rt, rid string) ([]EffectiveAccessItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	titles := map[string]string{}
	ids := []string{rid}
	if rt == "document" {
		d := m.docs[rid]
		titles[rid] = d.Title
		for fid := d.ParentFolderID; fid != ""; fid = m.folders[fid].ParentFolderID {
			ids = append(ids, fid)
			titles[fid] = m.folders[fid].Title
		}
	} else {
		for fid := rid; fid != ""; fid = m.folders[fid].ParentFolderID {
			if fid != rid {
				ids = append(ids, fid)
			}
			titles[fid] = m.folders[fid].Title
		}
	}
	by := map[string]*EffectiveAccessItem{}
	add := func(p auth.Principal, src AccessSource) {
		k := string(p.Type) + ":" + p.ID
		if by[k] == nil {
			by[k] = &EffectiveAccessItem{Principal: p}
		}
		by[k].EffectiveRole = MaxRole(by[k].EffectiveRole, src.Role)
		by[k].Sources = append(by[k].Sources, src)
	}
	if rt == "document" {
		d := m.docs[rid]
		add(d.Owner, AccessSource{ResourceType: "document", ResourceID: rid, ResourceTitle: d.Title, Role: RoleOwner})
	} else {
		f := m.folders[rid]
		add(f.Owner, AccessSource{ResourceType: "folder", ResourceID: rid, ResourceTitle: f.Title, Role: RoleOwner})
	}
	okid := map[string]bool{}
	for _, id := range ids {
		okid[id] = true
	}
	for _, g := range m.grants {
		if okid[g.ResourceID] {
			add(g.Principal, AccessSource{ResourceType: g.ResourceType, ResourceID: g.ResourceID, ResourceTitle: titles[g.ResourceID], Role: g.Role, GrantID: g.ID})
		}
	}
	out := []EffectiveAccessItem{}
	for _, v := range by {
		out = append(out, *v)
	}
	return out, nil
}
func (m *Memory) ListPins(ctx context.Context, p auth.Principal) ([]Pin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Pin{}
	pref := string(p.Type) + ":" + p.ID + ":"
	for k, pin := range m.pins {
		if strings.HasPrefix(k, pref) {
			out = append(out, pin)
		}
	}
	return out, nil
}
func (m *Memory) PutPin(ctx context.Context, p auth.Principal, rt, rid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pins[string(p.Type)+":"+p.ID+":"+rt+":"+rid] = Pin{ResourceType: rt, ResourceID: rid}
	return nil
}
func (m *Memory) DeletePin(ctx context.Context, p auth.Principal, rt, rid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pins, string(p.Type)+":"+p.ID+":"+rt+":"+rid)
	return nil
}

func (m *Memory) repositoryForFolderLocked(folderID string) (RepositoryRecord, bool) {
	for current, depth := folderID, 0; current != "" && depth < 64; depth++ {
		for _, repository := range m.repos {
			if repository.RootFolderID == current {
				return repository, true
			}
		}
		folder, ok := m.folders[current]
		if !ok {
			break
		}
		current = folder.ParentFolderID
	}
	return RepositoryRecord{}, false
}

func (m *Memory) repositoryForDocumentLocked(documentID string) (RepositoryRecord, bool) {
	document, ok := m.docs[documentID]
	if !ok || document.ParentFolderID == "" {
		return RepositoryRecord{}, false
	}
	return m.repositoryForFolderLocked(document.ParentFolderID)
}

//nolint:gocyclo // The in-memory implementation validates and publishes the repository as one atomic state transition.
func (m *Memory) CreateRepository(_ context.Context, creator auth.Principal, params CreateRepositoryParams) (RepositoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if params.Name == "" || params.Slug == "" || (params.Scope != RepositoryScopePersonal && params.Scope != RepositoryScopeOrganization) || (params.RootFolderID != "" && params.DocumentID != "") {
		return RepositoryRecord{}, ErrConflict
	}
	for _, existing := range m.repos {
		if existing.Scope == params.Scope && existing.NamespaceID == params.NamespaceID && strings.EqualFold(existing.Slug, params.Slug) {
			return RepositoryRecord{}, ErrConflict
		}
	}

	rootID := params.RootFolderID
	if rootID != "" {
		root, ok := m.folders[rootID]
		if !ok {
			return RepositoryRecord{}, ErrNotFound
		}
		if root.ParentFolderID != "" || root.Owner.Type != creator.Type || root.Owner.ID != creator.ID {
			return RepositoryRecord{}, ErrForbidden
		}
		if _, wrapped := m.repositoryForFolderLocked(rootID); wrapped {
			return RepositoryRecord{}, ErrConflict
		}
		descendants := map[string]bool{rootID: true}
		changed := true
		for changed {
			changed = false
			for id, folder := range m.folders {
				if !descendants[id] && descendants[folder.ParentFolderID] {
					descendants[id] = true
					changed = true
				}
			}
		}
		for id := range descendants {
			folder := m.folders[id]
			if folder.Owner.Type != creator.Type || folder.Owner.ID != creator.ID {
				return RepositoryRecord{}, ErrForbidden
			}
		}
		for _, document := range m.docs {
			if descendants[document.ParentFolderID] && (document.Owner.Type != creator.Type || document.Owner.ID != creator.ID) {
				return RepositoryRecord{}, ErrForbidden
			}
		}
	} else {
		if params.DocumentID != "" {
			document, ok := m.docs[params.DocumentID]
			if !ok {
				return RepositoryRecord{}, ErrNotFound
			}
			if document.ParentFolderID != "" || document.Owner.Type != creator.Type || document.Owner.ID != creator.ID {
				return RepositoryRecord{}, ErrForbidden
			}
		}
		if m.folderConflictLocked(creator, "", params.Name, "") {
			return RepositoryRecord{}, ErrConflict
		}
		m.folderN++
		rootID = fmt.Sprintf("fld_%d", m.folderN)
		m.folders[rootID] = Folder{ID: rootID, Title: params.Name, Owner: creator}
		if params.DocumentID != "" {
			document := m.docs[params.DocumentID]
			document.ParentFolderID = rootID
			m.docs[document.ID] = document
		}
	}

	for {
		m.repoN++
		if _, exists := m.repos[fmt.Sprintf("repo_%d", m.repoN)]; !exists {
			break
		}
	}
	id := fmt.Sprintf("repo_%d", m.repoN)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	repository := RepositoryRecord{
		ID:                id,
		Name:              params.Name,
		Slug:              params.Slug,
		Description:       params.Description,
		Scope:             params.Scope,
		NamespaceID:       params.NamespaceID,
		NamespaceName:     params.NamespaceName,
		RootFolderID:      rootID,
		CreatedBy:         creator,
		RequiredApprovals: params.RequiredApprovals,
		Protected:         params.Protected,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	m.repos[id] = repository
	if params.GrantOrgViewer {
		m.grantN++
		m.grants = append(m.grants, Grant{ID: fmt.Sprintf("gr_%d", m.grantN), DocumentID: rootID, ResourceType: "folder", ResourceID: rootID, Principal: auth.Principal{Type: auth.PrincipalAnyone}, Role: RoleViewer, GrantedBy: creator})
	}
	return repository, nil
}

func (m *Memory) ListRepositories(_ context.Context, principal auth.Principal) ([]RepositoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []RepositoryRecord{}
	for _, repository := range m.repos {
		if role, _ := m.roleForRepositoryLocked(principal, repository); role != RoleNone {
			out = append(out, repository)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NamespaceName == out[j].NamespaceName {
			return out[i].Name < out[j].Name
		}
		return out[i].NamespaceName < out[j].NamespaceName
	})
	return out, nil
}

func (m *Memory) GetRepository(_ context.Context, id string) (RepositoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	repository, ok := m.repos[id]
	if !ok {
		return RepositoryRecord{}, ErrNotFound
	}
	return repository, nil
}

//nolint:gocyclo // Repository cleanup is kept as one locked transition to avoid partially deleted in-memory state.
func (m *Memory) DeleteRepository(_ context.Context, id string, principal auth.Principal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	repository, ok := m.repos[id]
	if !ok {
		return ErrNotFound
	}
	if repository.CreatedBy.Type != principal.Type || repository.CreatedBy.ID != principal.ID {
		return ErrForbidden
	}

	folderIDs := map[string]bool{repository.RootFolderID: true}
	for changed := true; changed; {
		changed = false
		for folderID, folder := range m.folders {
			if !folderIDs[folderID] && folderIDs[folder.ParentFolderID] {
				folderIDs[folderID] = true
				changed = true
			}
		}
	}
	documentIDs := map[string]bool{}
	for documentID, document := range m.docs {
		if folderIDs[document.ParentFolderID] {
			documentIDs[documentID] = true
		}
	}
	versionIDs := map[string]bool{}
	for versionID, version := range m.versions {
		if documentIDs[version.DocumentID] {
			versionIDs[versionID] = true
		}
	}

	delete(m.repos, id)
	for pullRequestID, pullRequest := range m.prs {
		if pullRequest.RepositoryID == id {
			for _, change := range pullRequest.Changes {
				if change.HeadVersionID != "" {
					versionIDs[change.HeadVersionID] = true
				}
			}
			delete(m.prs, pullRequestID)
		}
	}
	for commentID, comment := range m.comments {
		if documentIDs[comment.DocumentID] {
			delete(m.comments, commentID)
		}
	}
	for versionID := range versionIDs {
		delete(m.versions, versionID)
		delete(m.versionHTML, versionID)
	}
	for documentID := range documentIDs {
		delete(m.docs, documentID)
		delete(m.kv, documentID)
		delete(m.kvRev, documentID)
		delete(m.kvEvents, documentID)
		delete(m.docWorkspace, documentID)
	}
	for folderID := range folderIDs {
		delete(m.folders, folderID)
		delete(m.folderWorkspace, folderID)
	}
	for key, pin := range m.pins {
		if (pin.ResourceType == "document" && documentIDs[pin.ResourceID]) ||
			(pin.ResourceType == "folder" && folderIDs[pin.ResourceID]) {
			delete(m.pins, key)
		}
	}
	keptGrants := m.grants[:0]
	for _, grant := range m.grants {
		if (grant.ResourceType == "document" && documentIDs[grant.ResourceID]) ||
			(grant.ResourceType == "folder" && folderIDs[grant.ResourceID]) {
			continue
		}
		keptGrants = append(keptGrants, grant)
	}
	m.grants = keptGrants

	return nil
}

func repositoryNamespaceName(principal auth.Principal) string {
	if strings.TrimSpace(principal.Name) != "" {
		return strings.TrimSpace(principal.Name)
	}
	if strings.TrimSpace(principal.Email) != "" {
		return strings.TrimSpace(principal.Email)
	}
	return principal.ID
}

//nolint:gocyclo // Ownership validation and mutation remain one locked transition to mirror production semantics.
func (m *Memory) TransferRepositoryOwnership(_ context.Context, id string, from, to auth.Principal) (RepositoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	repository, ok := m.repos[id]
	if !ok {
		return RepositoryRecord{}, ErrNotFound
	}
	if repository.CreatedBy.Type != from.Type || repository.CreatedBy.ID != from.ID {
		return RepositoryRecord{}, ErrForbidden
	}
	if to.ID == "" || to.Type == auth.PrincipalAnonymous || to.Type == auth.PrincipalAnyone {
		return RepositoryRecord{}, ErrConflict
	}
	if repository.Scope == RepositoryScopePersonal {
		for _, existing := range m.repos {
			if existing.ID != repository.ID && existing.Scope == RepositoryScopePersonal && existing.NamespaceID == to.ID && strings.EqualFold(existing.Slug, repository.Slug) {
				return RepositoryRecord{}, ErrConflict
			}
		}
	}
	root, ok := m.folders[repository.RootFolderID]
	if !ok {
		return RepositoryRecord{}, ErrNotFound
	}
	if m.folderConflictLocked(to, "", root.Title, root.ID) {
		return RepositoryRecord{}, ErrConflict
	}

	folderIDs := map[string]bool{repository.RootFolderID: true}
	for changed := true; changed; {
		changed = false
		for folderID, folder := range m.folders {
			if !folderIDs[folderID] && folderIDs[folder.ParentFolderID] {
				folderIDs[folderID] = true
				changed = true
			}
		}
	}
	for folderID := range folderIDs {
		folder := m.folders[folderID]
		folder.Owner = to
		m.folders[folderID] = folder
	}
	for documentID, document := range m.docs {
		if folderIDs[document.ParentFolderID] {
			document.Owner = to
			m.docs[documentID] = document
		}
	}
	repository.CreatedBy = to
	if repository.Scope == RepositoryScopePersonal {
		repository.NamespaceID = to.ID
		repository.NamespaceName = repositoryNamespaceName(to)
	}
	repository.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	m.repos[id] = repository
	return repository, nil
}

func (m *Memory) RoleForRepository(_ context.Context, principal auth.Principal, repositoryID string) (Role, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	repository, ok := m.repos[repositoryID]
	if !ok {
		return RoleNone, ErrNotFound
	}
	return m.roleForRepositoryLocked(principal, repository)
}

func (m *Memory) roleForRepositoryLocked(principal auth.Principal, repository RepositoryRecord) (Role, error) {
	role, err := m.roleForFolderLocked(principal, repository.RootFolderID)
	if repository.Scope == RepositoryScopeOrganization && principal.Type != auth.PrincipalAnonymous && principal.ID != "" {
		return MaxRole(role, RoleViewer), nil
	}
	return role, err
}

func (m *Memory) RepositoryForDocument(_ context.Context, documentID string) (RepositoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.docs[documentID]; !exists {
		return RepositoryRecord{}, ErrNotFound
	}
	repository, ok := m.repositoryForDocumentLocked(documentID)
	if !ok {
		return RepositoryRecord{}, ErrNotFound
	}
	return repository, nil
}

func (m *Memory) RepositoryForFolder(_ context.Context, folderID string) (RepositoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.folders[folderID]; !exists {
		return RepositoryRecord{}, ErrNotFound
	}
	repository, ok := m.repositoryForFolderLocked(folderID)
	if !ok {
		return RepositoryRecord{}, ErrNotFound
	}
	return repository, nil
}

func (m *Memory) ListRepositoryDocuments(_ context.Context, principal auth.Principal, repositoryID string) ([]Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	repository, ok := m.repos[repositoryID]
	if !ok {
		return nil, ErrNotFound
	}
	if role, _ := m.roleForRepositoryLocked(principal, repository); role == RoleNone {
		return nil, ErrNotFound
	}
	out := []Document{}
	for _, document := range m.docs {
		if wrapped, ok := m.repositoryForDocumentLocked(document.ID); ok && wrapped.ID == repositoryID {
			out = append(out, document)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

func (m *Memory) CreateRepositoryDocument(_ context.Context, repositoryID string, creator auth.Principal, title string, html []byte) (Document, Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	repository, ok := m.repos[repositoryID]
	if !ok {
		return Document{}, Version{}, ErrNotFound
	}
	role, _ := m.roleForRepositoryLocked(creator, repository)
	if role != RoleEditor && role != RoleOwner {
		return Document{}, Version{}, ErrForbidden
	}
	for _, document := range m.docs {
		if wrapped, wrappedOK := m.repositoryForDocumentLocked(document.ID); wrappedOK && wrapped.ID == repositoryID {
			return Document{}, Version{}, ErrRepositoryNotEmpty
		}
	}
	documentID := m.nextDocumentIDLocked()
	versionID := m.nextVersionIDLocked()
	document := Document{ID: documentID, Title: title, Owner: creator, CurrentVersionID: versionID, ParentFolderID: repository.RootFolderID, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	version := Version{ID: versionID, Number: 1, DocumentID: documentID, CreatedBy: creator, SHA256: fmt.Sprintf("sha_%d", m.verN)}
	m.docs[document.ID] = document
	m.versions[version.ID] = version
	m.versionHTML[version.ID] = append([]byte(nil), html...)
	return document, version, nil
}

func (m *Memory) pullRequestLocked(repositoryID string, number int64) (string, PullRequest, bool) {
	for id, pullRequest := range m.prs {
		if pullRequest.RepositoryID == repositoryID && pullRequest.Number == number {
			return id, pullRequest, true
		}
	}
	return "", PullRequest{}, false
}

//nolint:gocyclo // Review-policy branches stay together so mergeability has one source of truth.
func (m *Memory) computePullRequestLocked(pullRequest PullRequest) PullRequest {
	repository := m.repos[pullRequest.RepositoryID]
	if pullRequest.Patchset < 1 {
		pullRequest.Patchset = 1
	}
	if len(pullRequest.Changes) == 0 && pullRequest.DocumentID != "" {
		pullRequest.Changes = []PullRequestChange{{
			PullRequestID: pullRequest.ID, Patchset: pullRequest.Patchset,
			Operation: PullRequestChangeUpdate, DocumentID: pullRequest.DocumentID,
			BaseVersionID: pullRequest.BaseVersionID, HeadVersionID: pullRequest.HeadVersionID,
		}}
	}
	pullRequest.RequiredApprovals = repository.RequiredApprovals
	if pullRequest.Reviews == nil {
		pullRequest.Reviews = []PullRequestReview{}
	}
	pullRequest.Mergeable = false
	pullRequest.MergeBlockedReason = ""
	if pullRequest.Status != PullRequestStatusOpen {
		pullRequest.MergeBlockedReason = stringError(ErrPullRequestClosed)
		return pullRequest
	}
	if m.pullRequestStaleLocked(pullRequest) {
		pullRequest.MergeBlockedReason = stringError(ErrStalePullRequest)
		return pullRequest
	}
	approvals := 0
	for _, review := range pullRequest.Reviews {
		if review.Patchset != pullRequest.Patchset {
			continue
		}
		reviewerRole, _ := m.roleForFolderLocked(review.Reviewer, repository.RootFolderID)
		if reviewerRole != RoleEditor && reviewerRole != RoleOwner {
			continue
		}
		if review.State == ReviewStateChangesRequested {
			pullRequest.MergeBlockedReason = stringError(ErrChangesRequested)
			return pullRequest
		}
		if review.State == ReviewStateApproved && (review.Reviewer.Type != pullRequest.Author.Type || review.Reviewer.ID != pullRequest.Author.ID) {
			approvals++
		}
	}
	if approvals < repository.RequiredApprovals {
		pullRequest.MergeBlockedReason = stringError(ErrApprovalRequired)
		return pullRequest
	}
	pullRequest.Mergeable = true
	return pullRequest
}

func stringError(err error) string { return err.Error() }

func validPullRequestChangeOperation(operation string) bool {
	switch operation {
	case PullRequestChangeUpdate, PullRequestChangeCreate, PullRequestChangeRename, PullRequestChangeMove, PullRequestChangeDelete:
		return true
	default:
		return false
	}
}

func (m *Memory) pullRequestStaleLocked(pullRequest PullRequest) bool {
	repository, exists := m.repos[pullRequest.RepositoryID]
	if !exists {
		return true
	}
	for _, change := range pullRequest.Changes {
		if change.Operation == PullRequestChangeCreate {
			if _, exists := m.docs[change.DocumentID]; exists {
				return true
			}
			folder, exists := m.folders[change.FolderID]
			if !exists {
				return true
			}
			wrapped, ok := m.repositoryForFolderLocked(folder.ID)
			if !ok || wrapped.ID != repository.ID {
				return true
			}
			continue
		}
		document, exists := m.docs[change.DocumentID]
		if !exists || document.CurrentVersionID != change.BaseVersionID {
			return true
		}
		wrapped, ok := m.repositoryForDocumentLocked(document.ID)
		if !ok || wrapped.ID != repository.ID {
			return true
		}
		if change.Operation == PullRequestChangeMove {
			target, ok := m.repositoryForFolderLocked(change.FolderID)
			if !ok || target.ID != repository.ID {
				return true
			}
		}
	}
	return false
}

//nolint:gocyclo // The operation validation matrix is kept together for deterministic boundary-rule precedence.
func (m *Memory) validatePullRequestChangeInputsLocked(repository RepositoryRecord, changes []PullRequestChangeInput) error {
	if len(changes) == 0 {
		return ErrInvalidPullRequestChange
	}
	seen := map[string]bool{}
	for _, input := range changes {
		if !validPullRequestChangeOperation(input.Operation) {
			return ErrInvalidPullRequestChange
		}
		if input.Operation == PullRequestChangeCreate {
			if strings.TrimSpace(input.Title) == "" || input.HTML == nil {
				return ErrInvalidPullRequestChange
			}
			if input.DocumentID != "" {
				if _, exists := m.docs[input.DocumentID]; exists || seen[input.DocumentID] {
					return ErrConflict
				}
				seen[input.DocumentID] = true
			}
			folderID := input.FolderID
			if folderID == "" {
				folderID = repository.RootFolderID
			}
			wrapped, ok := m.repositoryForFolderLocked(folderID)
			if !ok || wrapped.ID != repository.ID {
				return ErrRepositoryBoundary
			}
			continue
		}
		if input.DocumentID == "" || seen[input.DocumentID] {
			return ErrInvalidPullRequestChange
		}
		seen[input.DocumentID] = true
		document, ok := m.docs[input.DocumentID]
		if !ok {
			return ErrNotFound
		}
		wrapped, ok := m.repositoryForDocumentLocked(input.DocumentID)
		if !ok || wrapped.ID != repository.ID {
			return ErrRepositoryBoundary
		}
		if input.BaseVersionID == "" || document.CurrentVersionID != input.BaseVersionID {
			return &VersionConflictError{LatestVersionID: document.CurrentVersionID}
		}
		switch input.Operation {
		case PullRequestChangeUpdate:
			if input.HTML == nil {
				return ErrInvalidPullRequestChange
			}
		case PullRequestChangeRename:
			if strings.TrimSpace(input.Title) == "" {
				return ErrInvalidPullRequestChange
			}
		case PullRequestChangeMove:
			if input.FolderID == "" {
				return ErrInvalidPullRequestChange
			}
			target, ok := m.repositoryForFolderLocked(input.FolderID)
			if !ok || target.ID != repository.ID {
				return ErrRepositoryBoundary
			}
		}
	}
	return nil
}

func (m *Memory) buildPullRequestChangesLocked(pullRequestID string, patchset int, repository RepositoryRecord, inputs []PullRequestChangeInput, changeSummary string, author auth.Principal) ([]PullRequestChange, error) {
	if err := m.validatePullRequestChangeInputsLocked(repository, inputs); err != nil {
		return nil, err
	}
	changes := make([]PullRequestChange, 0, len(inputs))
	for index, input := range inputs {
		documentID := input.DocumentID
		folderID := input.FolderID
		if input.Operation == PullRequestChangeCreate {
			if documentID == "" {
				documentID = m.nextDocumentIDLocked()
			}
			if folderID == "" {
				folderID = repository.RootFolderID
			}
		}
		change := PullRequestChange{
			ID:            fmt.Sprintf("%s_change_%d_%d", pullRequestID, patchset, index+1),
			PullRequestID: pullRequestID, Patchset: patchset, Operation: input.Operation,
			DocumentID: documentID, FolderID: folderID, BaseVersionID: input.BaseVersionID,
			Title: strings.TrimSpace(input.Title), Path: strings.TrimSpace(input.Path),
		}
		if document, ok := m.docs[documentID]; ok {
			change.BaseTitle = document.Title
			change.BasePath = document.ParentFolderID
			if change.Title == "" {
				change.Title = document.Title
			}
			if change.FolderID == "" {
				change.FolderID = document.ParentFolderID
			}
		}
		if input.Operation == PullRequestChangeUpdate || input.Operation == PullRequestChangeCreate {
			versionID := m.nextVersionIDLocked()
			change.HeadVersionID = versionID
			m.versions[versionID] = Version{ID: versionID, DocumentID: documentID, CreatedBy: author, ChangeSummary: changeSummary, SHA256: fmt.Sprintf("sha_%d", m.verN)}
			m.versionHTML[versionID] = append([]byte(nil), input.HTML...)
		} else if input.Operation != PullRequestChangeDelete {
			change.HeadVersionID = input.BaseVersionID
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func setPullRequestCompatibilityFields(pullRequest *PullRequest) {
	pullRequest.DocumentID, pullRequest.BaseVersionID, pullRequest.HeadVersionID = "", "", ""
	if len(pullRequest.Changes) > 0 {
		first := pullRequest.Changes[0]
		if first.Operation == PullRequestChangeCreate {
			return
		}
		pullRequest.DocumentID = first.DocumentID
		pullRequest.BaseVersionID = first.BaseVersionID
		pullRequest.HeadVersionID = first.HeadVersionID
	}
}

func (m *Memory) CreatePullRequest(ctx context.Context, repositoryID, documentID, baseVersionID, title, body, changeSummary string, html []byte, author auth.Principal) (PullRequest, error) {
	pullRequest, err := m.CreatePullRequestChangeSet(ctx, repositoryID, title, body, changeSummary, []PullRequestChangeInput{{
		Operation: PullRequestChangeUpdate, DocumentID: documentID, BaseVersionID: baseVersionID, HTML: html,
	}}, author)
	if errors.Is(err, ErrRepositoryBoundary) {
		return PullRequest{}, ErrNotFound
	}
	return pullRequest, err
}

func (m *Memory) CreatePullRequestChangeSet(_ context.Context, repositoryID, title, body, changeSummary string, inputs []PullRequestChangeInput, author auth.Principal) (PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	repository, ok := m.repos[repositoryID]
	if !ok {
		return PullRequest{}, ErrNotFound
	}
	var pullRequestID string
	for {
		m.prN++
		pullRequestID = fmt.Sprintf("pr_%d", m.prN)
		if _, exists := m.prs[pullRequestID]; !exists {
			break
		}
	}
	number := int64(1)
	for _, existing := range m.prs {
		if existing.RepositoryID == repositoryID && existing.Number >= number {
			number = existing.Number + 1
		}
	}
	changes, err := m.buildPullRequestChangesLocked(pullRequestID, 1, repository, inputs, changeSummary, author)
	if err != nil {
		return PullRequest{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pullRequest := PullRequest{ID: pullRequestID, RepositoryID: repositoryID, Number: number, Patchset: 1, Changes: changes, Title: title, Body: body, ChangeSummary: changeSummary, Status: PullRequestStatusOpen, Author: author, CreatedAt: now, UpdatedAt: now, Reviews: []PullRequestReview{}, RequiredApprovals: repository.RequiredApprovals}
	setPullRequestCompatibilityFields(&pullRequest)
	pullRequest = m.computePullRequestLocked(pullRequest)
	m.prs[pullRequest.ID] = pullRequest
	return pullRequest, nil
}

func (m *Memory) CreatePullRequestPatchset(_ context.Context, repositoryID string, number int64, changeSummary string, inputs []PullRequestChangeInput, author auth.Principal) (PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, pullRequest, ok := m.pullRequestLocked(repositoryID, number)
	if !ok {
		return PullRequest{}, ErrNotFound
	}
	if pullRequest.Status != PullRequestStatusOpen {
		return PullRequest{}, ErrPullRequestClosed
	}
	repository := m.repos[repositoryID]
	nextPatchset := pullRequest.Patchset + 1
	changes, err := m.buildPullRequestChangesLocked(pullRequest.ID, nextPatchset, repository, inputs, changeSummary, author)
	if err != nil {
		return PullRequest{}, err
	}
	pullRequest.Patchset = nextPatchset
	pullRequest.Changes = changes
	if changeSummary != "" {
		pullRequest.ChangeSummary = changeSummary
	}
	setPullRequestCompatibilityFields(&pullRequest)
	pullRequest.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	pullRequest = m.computePullRequestLocked(pullRequest)
	m.prs[id] = pullRequest
	return pullRequest, nil
}

func (m *Memory) RebasePullRequest(_ context.Context, repositoryID string, number int64, author auth.Principal) (PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, pullRequest, ok := m.pullRequestLocked(repositoryID, number)
	if !ok {
		return PullRequest{}, ErrNotFound
	}
	if pullRequest.Status != PullRequestStatusOpen {
		return PullRequest{}, ErrPullRequestClosed
	}
	pullRequest = m.computePullRequestLocked(pullRequest)
	inputs := make([]PullRequestChangeInput, 0, len(pullRequest.Changes))
	for _, change := range pullRequest.Changes {
		input := PullRequestChangeInput{Operation: change.Operation, DocumentID: change.DocumentID, FolderID: change.FolderID, Title: change.Title, Path: change.Path}
		if change.Operation != PullRequestChangeCreate {
			document, exists := m.docs[change.DocumentID]
			if !exists {
				return PullRequest{}, ErrStalePullRequest
			}
			input.BaseVersionID = document.CurrentVersionID
		}
		if change.Operation == PullRequestChangeUpdate || change.Operation == PullRequestChangeCreate {
			html, exists := m.versionHTML[change.HeadVersionID]
			if !exists {
				return PullRequest{}, ErrStalePullRequest
			}
			input.HTML = append([]byte(nil), html...)
		}
		inputs = append(inputs, input)
	}
	nextPatchset := pullRequest.Patchset + 1
	changes, err := m.buildPullRequestChangesLocked(pullRequest.ID, nextPatchset, m.repos[repositoryID], inputs, pullRequest.ChangeSummary, author)
	if err != nil {
		if errors.Is(err, ErrVersionConflict) {
			return PullRequest{}, ErrStalePullRequest
		}
		return PullRequest{}, err
	}
	pullRequest.Patchset = nextPatchset
	pullRequest.Changes = changes
	setPullRequestCompatibilityFields(&pullRequest)
	pullRequest.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	pullRequest = m.computePullRequestLocked(pullRequest)
	m.prs[id] = pullRequest
	return pullRequest, nil
}

func (m *Memory) ListPullRequests(_ context.Context, repositoryID string) ([]PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.repos[repositoryID]; !ok {
		return nil, ErrNotFound
	}
	out := []PullRequest{}
	for _, pullRequest := range m.prs {
		if pullRequest.RepositoryID == repositoryID {
			out = append(out, m.computePullRequestLocked(pullRequest))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	return out, nil
}

func (m *Memory) GetPullRequest(_ context.Context, repositoryID string, number int64) (PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, pullRequest, ok := m.pullRequestLocked(repositoryID, number)
	if !ok {
		return PullRequest{}, ErrNotFound
	}
	return m.computePullRequestLocked(pullRequest), nil
}

func (m *Memory) ReviewPullRequest(_ context.Context, repositoryID string, number int64, reviewer auth.Principal, state, body string) (PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, pullRequest, ok := m.pullRequestLocked(repositoryID, number)
	if !ok {
		return PullRequest{}, ErrNotFound
	}
	if pullRequest.Status != PullRequestStatusOpen {
		return PullRequest{}, ErrPullRequestClosed
	}
	if pullRequest.Author.Type == reviewer.Type && pullRequest.Author.ID == reviewer.ID {
		return PullRequest{}, ErrSelfReview
	}
	if state != ReviewStateApproved && state != ReviewStateChangesRequested {
		return PullRequest{}, ErrConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	review := PullRequestReview{PullRequestID: pullRequest.ID, HeadVersionID: pullRequest.HeadVersionID, Patchset: pullRequest.Patchset, Reviewer: reviewer, State: state, Body: body, SubmittedAt: now}
	replaced := false
	for index, existing := range pullRequest.Reviews {
		if existing.Reviewer.Type == reviewer.Type && existing.Reviewer.ID == reviewer.ID {
			review.ID = existing.ID
			pullRequest.Reviews[index] = review
			replaced = true
			break
		}
	}
	if !replaced {
		m.reviewN++
		review.ID = fmt.Sprintf("review_%d", m.reviewN)
		pullRequest.Reviews = append(pullRequest.Reviews, review)
	}
	pullRequest.UpdatedAt = now
	pullRequest = m.computePullRequestLocked(pullRequest)
	m.prs[id] = pullRequest
	return pullRequest, nil
}

//nolint:gocyclo // Merge validation and publication remain one locked transition to avoid partial in-memory state.
func (m *Memory) MergePullRequest(_ context.Context, repositoryID string, number int64, mergedBy auth.Principal) (PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, pullRequest, ok := m.pullRequestLocked(repositoryID, number)
	if !ok {
		return PullRequest{}, ErrNotFound
	}
	checked := m.computePullRequestLocked(pullRequest)
	if !checked.Mergeable {
		switch checked.MergeBlockedReason {
		case ErrPullRequestClosed.Error():
			return PullRequest{}, ErrPullRequestClosed
		case ErrStalePullRequest.Error():
			return PullRequest{}, ErrStalePullRequest
		case ErrChangesRequested.Error():
			return PullRequest{}, ErrChangesRequested
		default:
			return PullRequest{}, ErrApprovalRequired
		}
	}
	// Validate every head before mutating the tree. The memory lock then makes
	// the following application atomic to concurrent callers.
	for _, change := range checked.Changes {
		if change.Operation == PullRequestChangeUpdate || change.Operation == PullRequestChangeCreate {
			version, ok := m.versions[change.HeadVersionID]
			if !ok || version.DocumentID != change.DocumentID {
				return PullRequest{}, ErrStalePullRequest
			}
		}
	}
	for _, change := range checked.Changes {
		switch change.Operation {
		case PullRequestChangeUpdate:
			version := m.versions[change.HeadVersionID]
			version.Number = m.nextPublishedVersionNumberLocked(change.DocumentID)
			m.versions[version.ID] = version
			document := m.docs[change.DocumentID]
			document.CurrentVersionID = version.ID
			document.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			m.docs[document.ID] = document
		case PullRequestChangeCreate:
			version := m.versions[change.HeadVersionID]
			version.Number = 1
			m.versions[version.ID] = version
			m.docs[change.DocumentID] = Document{ID: change.DocumentID, Title: change.Title, Owner: pullRequest.Author, CurrentVersionID: version.ID, ParentFolderID: change.FolderID, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		case PullRequestChangeRename:
			document := m.docs[change.DocumentID]
			document.Title = change.Title
			document.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			m.docs[document.ID] = document
		case PullRequestChangeMove:
			document := m.docs[change.DocumentID]
			document.ParentFolderID = change.FolderID
			document.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			m.docs[document.ID] = document
		case PullRequestChangeDelete:
			m.deleteDocumentForPullRequestLocked(change.DocumentID)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pullRequest.Status = PullRequestStatusMerged
	pullRequest.MergedBy = &mergedBy
	pullRequest.MergedAt = now
	pullRequest.UpdatedAt = now
	pullRequest = m.computePullRequestLocked(pullRequest)
	m.prs[id] = pullRequest
	return pullRequest, nil
}

func (m *Memory) nextPublishedVersionNumberLocked(documentID string) int {
	next := 1
	for _, version := range m.versions {
		if version.DocumentID == documentID && version.Number >= next {
			next = version.Number + 1
		}
	}
	return next
}

func (m *Memory) deleteDocumentForPullRequestLocked(documentID string) {
	delete(m.docs, documentID)
	delete(m.kv, documentID)
	delete(m.kvRev, documentID)
	delete(m.kvEvents, documentID)
	delete(m.docWorkspace, documentID)
	for id, version := range m.versions {
		if version.DocumentID == documentID {
			delete(m.versions, id)
			delete(m.versionHTML, id)
		}
	}
	for id, comment := range m.comments {
		if comment.DocumentID == documentID {
			delete(m.comments, id)
		}
	}
	for key, pin := range m.pins {
		if pin.ResourceType == "document" && pin.ResourceID == documentID {
			delete(m.pins, key)
		}
	}
	kept := m.grants[:0]
	for _, grant := range m.grants {
		if grant.ResourceType != "document" || grant.ResourceID != documentID {
			kept = append(kept, grant)
		}
	}
	m.grants = kept
}

func (m *Memory) ClosePullRequest(_ context.Context, repositoryID string, number int64, _ auth.Principal) (PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, pullRequest, ok := m.pullRequestLocked(repositoryID, number)
	if !ok {
		return PullRequest{}, ErrNotFound
	}
	if pullRequest.Status != PullRequestStatusOpen {
		return PullRequest{}, ErrPullRequestClosed
	}
	pullRequest.Status = PullRequestStatusClosed
	pullRequest.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	pullRequest = m.computePullRequestLocked(pullRequest)
	m.prs[id] = pullRequest
	return pullRequest, nil
}
