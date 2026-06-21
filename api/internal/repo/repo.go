package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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

// ErrNotFound wraps auth.ErrNotFound so that packages which cannot import repo
// (to avoid an import cycle) can still detect not-found via errors.Is.
var ErrNotFound = fmt.Errorf("not found: %w", auth.ErrNotFound)
var ErrConflict = errors.New("conflict")

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

// Comment / transfer status values.
const (
	StatusOpen     = "open"
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusDeclined = "declined"
)

type Document struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Owner            auth.Principal `json:"owner"`
	CurrentVersionID string         `json:"current_version_id"`
	ParentFolderID   string         `json:"parent_folder_id,omitempty"`
}

type Folder struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Owner          auth.Principal `json:"owner"`
	ParentFolderID string         `json:"parent_folder_id,omitempty"`
}

type FolderEntry struct {
	Type     string    `json:"type"`
	Folder   *Folder   `json:"folder,omitempty"`
	Document *Document `json:"document,omitempty"`
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
	ID              string         `json:"id"`
	DocumentID      string         `json:"document_id"`
	VersionID       string         `json:"version_id"`
	Author          auth.Principal `json:"author"`
	Body            string         `json:"body"`
	SelectedText    string         `json:"selected_text"`
	Anchor          Anchor         `json:"anchor"`
	Status          string         `json:"status"`
	ParentCommentID string         `json:"parent_comment_id,omitempty"`
	RootCommentID   string         `json:"root_comment_id,omitempty"`
	Replies         []Comment      `json:"replies,omitempty"`
}

// Repository is the application persistence boundary. The real implementation
// will wrap sqlc-generated queries. API tests can provide fakes.
type Repository interface {
	ResolveBearerToken(ctx context.Context, tokenHash string) (auth.Principal, error)
	ResolveUser(ctx context.Context, id string) (auth.Principal, error)
	UpsertGoogleUser(ctx context.Context, id, email, name, googleSub, pictureURL string) (auth.Principal, error)
	CreateCLICredential(ctx context.Context, userID, name, tokenHash string) (string, error)
	ListCLICredentials(ctx context.Context, userID string) ([]CLICredential, error)
	RevokeCLICredential(ctx context.Context, userID, credentialID string) error
	RoleForDocument(ctx context.Context, principal auth.Principal, documentID string) (Role, error)
	RoleForFolder(ctx context.Context, principal auth.Principal, folderID string) (Role, error)
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
	CreateGrant(ctx context.Context, documentID string, principal auth.Principal, role Role, grantedBy auth.Principal) (Grant, error)
	ListGrants(ctx context.Context, documentID string) ([]Grant, error)
	UpdateGrant(ctx context.Context, documentID, grantID string, role Role) (Grant, error)
	DeleteGrant(ctx context.Context, documentID, grantID string) error
	CreateVersion(ctx context.Context, documentID, baseVersionID, changeSummary string, html []byte, createdBy auth.Principal) (Version, error)
	ListVersions(ctx context.Context, documentID string) ([]Version, error)
	GetVersion(ctx context.Context, id string) (Version, error)
	GetVersionHTML(ctx context.Context, id string) ([]byte, error)
	CreateComment(ctx context.Context, documentID, versionID, body string, anchor Anchor, author auth.Principal) (Comment, error)
	CreateCommentReply(ctx context.Context, documentID, parentCommentID, body string, author auth.Principal) (Comment, error)
	GetComment(ctx context.Context, id string) (Comment, error)
	ListComments(ctx context.Context, documentID, status, versionID string) ([]Comment, error)
	UpdateComment(ctx context.Context, id, body, status string) (Comment, error)
	DeleteComment(ctx context.Context, id string) error
	CreateFolder(ctx context.Context, owner auth.Principal, title, parentFolderID string) (Folder, error)
	ListFolder(ctx context.Context, principal auth.Principal, folderID, path string) ([]FolderEntry, bool, *Folder, error)
	GetFolder(ctx context.Context, id string) (Folder, error)
	UpdateFolder(ctx context.Context, id, title string, parentFolderID *string) (Folder, error)
	DeleteFolder(ctx context.Context, id string) error
	AddDocumentToFolder(ctx context.Context, principal auth.Principal, folderID, documentID string) (bool, error)
	RemoveDocumentFromFolder(ctx context.Context, folderID, documentID string) error
	ListFolderDocuments(ctx context.Context, principal auth.Principal, folderID string) ([]Document, error)
	ListEffectiveAccess(ctx context.Context, resourceType, resourceID string) ([]EffectiveAccessItem, error)
	ListPins(ctx context.Context, principal auth.Principal) ([]Pin, error)
	PutPin(ctx context.Context, principal auth.Principal, resourceType, resourceID string) error
	DeletePin(ctx context.Context, principal auth.Principal, resourceType, resourceID string) error
}

type Memory struct {
	mu        sync.Mutex
	docN      int
	verN      int
	saN       int
	grantN    int
	commentN  int
	credN     int
	sakN      int
	folderN   int
	docs      map[string]Document
	folders   map[string]Folder
	pins      map[string]Pin
	versions  map[string]Version
	grants    []Grant
	users     map[string]auth.Principal
	creds     map[string]auth.Principal
	cliCreds  []cliCredRecord
	sas       map[string]ServiceAccount
	comments  map[string]Comment
	transfers map[string]OwnershipTransfer

	// Test-only fixtures, populated by NewMemorySeeded and consulted by
	// RoleForDocument / PrincipalExists. Kept out of the users/grants stores so
	// they do not perturb id sequencing in handler tests. Always nil/empty in
	// production (NewMemory), so the lookups below are inert there.
	fixtureRoles      map[string]Role
	fixturePrincipals map[string]bool
}

type cliCredRecord struct {
	ID        string
	UserID    string
	TokenHash string
	Name      string
}

// NewMemory returns an empty in-memory repository. It carries no fixtures so it
// behaves like a freshly migrated database; tests that need the canonical
// fixture graph call NewMemorySeeded instead.
func NewMemory() *Memory {
	return &Memory{docs: map[string]Document{}, folders: map[string]Folder{}, pins: map[string]Pin{}, versions: map[string]Version{}, users: map[string]auth.Principal{}, creds: map[string]auth.Principal{}, sas: map[string]ServiceAccount{}, comments: map[string]Comment{}, transfers: map[string]OwnershipTransfer{}}
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
	for existingID, existing := range m.users {
		if strings.EqualFold(existing.Email, email) {
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
		// Fall through to fixture handling below for API tests that seed roles
		// without a backing document.
		if role, ok := m.fixtureRoles[p.ID]; ok {
			return role, nil
		}
		return "", ErrNotFound
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
	if role, ok := m.fixtureRoles[p.ID]; ok {
		best = MaxRole(best, role)
	}
	if best == RoleNone {
		return "", ErrNotFound
	}
	return best, nil
}

func (m *Memory) CreateDocument(ctx context.Context, owner auth.Principal, title string, html []byte) (Document, Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docN++
	m.verN++
	d := Document{ID: fmt.Sprintf("doc_%d", m.docN), Title: title, Owner: owner, CurrentVersionID: fmt.Sprintf("ver_%d", m.verN)}
	v := Version{ID: d.CurrentVersionID, Number: 1, DocumentID: d.ID, CreatedBy: owner, SHA256: fmt.Sprintf("sha_%d", m.verN)}
	m.docs[d.ID] = d
	m.versions[v.ID] = v
	return d, v, nil
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
		if _, ok := m.users[p.ID]; ok {
			return true, nil
		}
		if m.fixturePrincipals[p.ID] {
			return true, nil
		}
	case auth.PrincipalServiceAccount:
		_, ok := m.sas[p.ID]
		return ok, nil
	}
	return false, nil
}
func (m *Memory) UserByEmail(ctx context.Context, email string) (auth.Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *Memory) EnsureUserByEmail(ctx context.Context, email string) (auth.Principal, error) {
	if p, err := m.UserByEmail(ctx, email); err == nil {
		return p, nil
	} else if !errors.Is(err, ErrNotFound) {
		return auth.Principal{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("usr_%d", len(m.users)+1)
	p := auth.Principal{Type: auth.PrincipalUser, ID: id, Email: email}
	m.users[id] = p
	return p, nil
}
func (m *Memory) CreateGrant(ctx context.Context, documentID string, principal auth.Principal, role Role, grantedBy auth.Principal) (Grant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grantN++
	rt := "document"
	if strings.HasPrefix(documentID, "fld_") {
		rt = "folder"
	}
	g := Grant{ID: fmt.Sprintf("gr_%d", m.grantN), DocumentID: documentID, ResourceType: rt, ResourceID: documentID, Principal: principal, Role: role, GrantedBy: grantedBy}
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
	if baseVersionID != d.CurrentVersionID {
		return Version{}, &VersionConflictError{LatestVersionID: d.CurrentVersionID}
	}
	m.verN++
	v := Version{ID: fmt.Sprintf("ver_%d", m.verN), Number: m.verN, DocumentID: documentID, CreatedBy: createdBy, ChangeSummary: changeSummary, SHA256: fmt.Sprintf("sha_%d", m.verN)}
	d.CurrentVersionID = v.ID
	m.docs[documentID] = d
	m.versions[v.ID] = v
	return v, nil
}
func (m *Memory) CreateComment(ctx context.Context, documentID, versionID, body string, anchor Anchor, author auth.Principal) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.versions[versionID]
	if !ok || v.DocumentID != documentID {
		return Comment{}, ErrNotFound
	}
	m.commentN++
	cm := Comment{ID: fmt.Sprintf("cmt_%d", m.commentN), DocumentID: documentID, VersionID: versionID, Author: author, Body: body, SelectedText: anchor.Quote, Anchor: anchor, Status: StatusOpen}
	cm.RootCommentID = cm.ID
	m.comments[cm.ID] = cm
	return cm, nil
}
func (m *Memory) CreateCommentReply(ctx context.Context, documentID, parentCommentID, body string, author auth.Principal) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parent, ok := m.comments[parentCommentID]
	if !ok || parent.DocumentID != documentID {
		return Comment{}, ErrNotFound
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
	cm := Comment{ID: id, DocumentID: documentID, VersionID: parent.VersionID, Author: author, Body: body, Status: StatusOpen, ParentCommentID: parent.ID, RootCommentID: rootID}
	m.comments[cm.ID] = cm
	return cm, nil
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
		if limit > 0 && int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}
func (m *Memory) RecordDocumentOpened(ctx context.Context, documentID string, principal auth.Principal) error {
	return nil
}
func (m *Memory) UpdateDocument(ctx context.Context, id, title string) (Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[id]
	if !ok {
		return Document{}, ErrNotFound
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
	delete(m.docs, id)
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
		if v.DocumentID == documentID {
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
	return []byte("<html></html>"), nil
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
func (m *Memory) UpdateComment(ctx context.Context, id, body, status string) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cm, ok := m.comments[id]
	if !ok {
		return Comment{}, ErrNotFound
	}
	if body != "" {
		cm.Body = body
	}
	if status != "" {
		cm.Status = status
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
	return f, nil
}
func (m *Memory) RoleForFolder(ctx context.Context, p auth.Principal, folderID string) (Role, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	if best == RoleNone {
		return "", ErrNotFound
	}
	return best, nil
}
func (m *Memory) ListFolder(ctx context.Context, p auth.Principal, folderID, path string) ([]FolderEntry, bool, *Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parent := folderID
	if path != "" && path != "/" {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		for _, part := range parts {
			found := ""
			for _, f := range m.folders {
				if f.ParentFolderID == parent && f.Title == part {
					found = f.ID
					break
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
func (m *Memory) UpdateFolder(ctx context.Context, id, title string, parent *string) (Folder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.folders[id]
	if !ok {
		return Folder{}, ErrNotFound
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
func (m *Memory) DeleteFolder(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	if _, ok := m.folders[id]; !ok {
		return ErrNotFound
	}
	del(id)
	return nil
}
func (m *Memory) AddDocumentToFolder(ctx context.Context, p auth.Principal, folderID, docID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[docID]
	if !ok {
		return false, ErrNotFound
	}
	if _, ok := m.folders[folderID]; !ok {
		return false, ErrNotFound
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
func (m *Memory) RemoveDocumentFromFolder(ctx context.Context, folderID, docID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[docID]
	if !ok || d.ParentFolderID != folderID {
		return ErrNotFound
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
			add(g.Principal, AccessSource{ResourceType: g.ResourceType, ResourceID: g.ResourceID, ResourceTitle: titles[g.ResourceID], Role: g.Role})
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
