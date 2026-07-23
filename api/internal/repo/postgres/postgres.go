package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	dbsqlc "github.com/razorpay/aidocs/api/internal/db/sqlc"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// htmlContentType is the content type stored for all version HTML blobs.
const htmlContentType = "text/html; charset=utf-8"

type Store struct {
	db    *pgxpool.Pool
	q     *dbsqlc.Queries
	blobs blob.Store
}

func New(db *pgxpool.Pool) *Store { return NewWithBlob(db, nil) }
func NewWithBlob(db *pgxpool.Pool, blobs blob.Store) *Store {
	if blobs == nil {
		// Tests/local callers that do not wire S3 still use a blob store;
		// version HTML is never duplicated in Postgres.
		blobs = blob.NewMemory()
	}
	return &Store{db: db, q: dbsqlc.New(db), blobs: blobs}
}
func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	return ConnectWithBlob(ctx, databaseURL, nil)
}
func ConnectWithBlob(ctx context.Context, databaseURL string, blobs blob.Store) (*Store, error) {
	p, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return NewWithBlob(p, blobs), nil
}
func (s *Store) Close()                         { s.db.Close() }
func (s *Store) Pool() *pgxpool.Pool            { return s.db }
func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

func (s *Store) ResolveBearerToken(ctx context.Context, tokenHash string) (auth.Principal, error) {
	u, err := s.q.ResolveCLIToken(ctx, tokenHash)
	if err == nil {
		_ = s.q.TouchCLIToken(ctx, tokenHash)
		return auth.Principal{Type: auth.PrincipalType(u.PrincipalType), ID: u.ID, Email: u.Email, Name: u.Name, PictureURL: u.PictureUrl}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, err
	}
	sa, err := s.q.ResolveServiceAccountToken(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, repo.ErrNotFound
	}
	if err == nil {
		_ = s.q.TouchServiceAccountToken(ctx, tokenHash)
		return auth.Principal{Type: auth.PrincipalType(sa.PrincipalType), ID: sa.ID, Email: sa.Email, Name: sa.Name, PictureURL: sa.PictureUrl}, nil
	}
	return auth.Principal{}, err
}
func (s *Store) ResolveUser(ctx context.Context, id string) (auth.Principal, error) {
	u, err := s.q.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, repo.ErrNotFound
	}
	if err != nil {
		return auth.Principal{}, err
	}
	return auth.Principal{Type: auth.PrincipalUser, ID: u.ID, Email: u.Email, Name: u.Name, PictureURL: u.PictureUrl}, nil
}
func (s *Store) UpsertGoogleUser(ctx context.Context, id, email, name, googleSub, pictureURL string) (auth.Principal, error) {
	err := s.q.UpsertGoogleUser(ctx, dbsqlc.UpsertGoogleUserParams{ID: id, Email: email, Name: name, GoogleSub: pgtype.Text{String: googleSub, Valid: true}, PictureUrl: pictureURL})
	if err != nil {
		return auth.Principal{}, err
	}
	u, err := s.q.GetUserByGoogleSub(ctx, pgtype.Text{String: googleSub, Valid: true})
	if err != nil {
		return auth.Principal{}, err
	}
	return auth.Principal{Type: auth.PrincipalUser, ID: u.ID, Email: u.Email, Name: u.Name, PictureURL: u.PictureUrl}, nil
}
func (s *Store) UserByEmail(ctx context.Context, email string) (auth.Principal, error) {
	u, err := s.q.GetSignedInUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, repo.ErrNotFound
	}
	if err != nil {
		return auth.Principal{}, err
	}
	return auth.Principal{Type: auth.PrincipalUser, ID: u.ID, Email: u.Email, Name: u.Name, PictureURL: u.PictureUrl}, nil
}

func (s *Store) SearchUsersByEmail(ctx context.Context, query string) ([]auth.Principal, error) {
	rows, err := s.q.SearchUsersByEmail(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]auth.Principal, 0, len(rows))
	for _, u := range rows {
		out = append(out, auth.Principal{Type: auth.PrincipalUser, ID: u.ID, Email: u.Email, Name: u.Name, PictureURL: u.PictureUrl})
	}
	return out, nil
}

func (s *Store) EnsureUserByEmail(ctx context.Context, email string) (auth.Principal, error) {
	u, err := s.UserByEmail(ctx, email)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, repo.ErrNotFound) {
		return auth.Principal{}, err
	}
	id := newID("usr")
	if err := s.q.InsertPlaceholderUser(ctx, dbsqlc.InsertPlaceholderUserParams{ID: id, Email: email}); err != nil {
		return auth.Principal{}, err
	}
	return auth.Principal{Type: auth.PrincipalUser, ID: id, Email: email}, nil
}
func (s *Store) CreateCLICredential(ctx context.Context, userID, name, tokenHash string) (string, error) {
	id := newID("cred")
	err := s.q.CreateCLICredential(ctx, dbsqlc.CreateCLICredentialParams{ID: id, UserID: userID, Name: name, TokenHash: tokenHash})
	return id, err
}
func (s *Store) ListCLICredentials(ctx context.Context, userID string) ([]repo.CLICredential, error) {
	rows, err := s.q.ListCLICredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.CLICredential, 0, len(rows))
	for _, r := range rows {
		out = append(out, repo.CLICredential{ID: r.ID, Name: r.Name})
	}
	return out, nil
}
func (s *Store) RevokeCLICredential(ctx context.Context, userID, credentialID string) error {
	return s.q.RevokeCLICredential(ctx, dbsqlc.RevokeCLICredentialParams{ID: credentialID, UserID: userID})
}

func (s *Store) RoleForDocument(ctx context.Context, p auth.Principal, documentID string) (repo.Role, error) {
	owner, err := s.q.GetDocumentOwner(ctx, documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", repo.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	best := repo.RoleNone
	if string(p.Type) == owner.OwnerType && p.ID == owner.OwnerID {
		best = repo.RoleOwner
	}
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE ancestors(rid, rtype, depth) AS (
			SELECT $1::text, 'document'::text, 0
			UNION ALL
			SELECT f.id, 'folder'::text, a.depth+1
			FROM ancestors a
			JOIN LATERAL (
				SELECT CASE WHEN a.rtype='document' THEN (SELECT parent_folder_id FROM documents WHERE id=a.rid)
				            ELSE (SELECT parent_folder_id FROM folders WHERE id=a.rid) END AS pid
			) sub ON true
			JOIN folders f ON f.id = sub.pid
			WHERE sub.pid IS NOT NULL AND a.depth < 64
		)
		SELECT g.resource_id, g.principal_type, g.principal_id, g.role
		FROM resource_grants g
		WHERE g.revoked_at IS NULL
		  AND g.resource_id IN (SELECT rid FROM ancestors)
	`, documentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var rid, pt, pid, role string
		if err := rows.Scan(&rid, &pt, &pid, &role); err != nil {
			return "", err
		}
		if (pt == string(p.Type) && pid == p.ID) || pt == string(auth.PrincipalAnyone) {
			best = repo.MaxRole(best, repo.Role(role))
		}
	}
	if best == repo.RoleNone {
		return "", repo.ErrNotFound
	}
	return best, nil
}

func (s *Store) CreateDocument(ctx context.Context, owner auth.Principal, title string, html []byte) (repo.Document, repo.Version, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if owner.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, owner); err != nil {
			return repo.Document{}, repo.Version{}, err
		}
	}
	docID, verID := newID("doc"), newID("ver")
	blobKey := fmt.Sprintf("docs/%s/%s.html", docID, verID)
	sum := sha(html)
	if err := s.blobs.Put(ctx, blobKey, htmlContentType, html); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), blobKey)
		}
	}()
	if err := q.InsertDocument(ctx, dbsqlc.InsertDocumentParams{ID: docID, Title: title, OwnerType: string(owner.Type), OwnerID: owner.ID}); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	if err := q.InsertInitialVersion(ctx, dbsqlc.InsertInitialVersionParams{ID: verID, DocumentID: docID, HtmlBlobKey: blobKey, Sha256: sum, CreatedByType: string(owner.Type), CreatedByID: owner.ID}); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	if err := q.UpdateDocumentCurrentVersion(ctx, dbsqlc.UpdateDocumentCurrentVersionParams{CurrentVersionID: pgtype.Text{String: verID, Valid: true}, ID: docID}); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	committed = true
	return repo.Document{ID: docID, Title: title, Owner: owner, CurrentVersionID: verID}, repo.Version{ID: verID, Number: 1, DocumentID: docID, CreatedBy: owner, SHA256: sum, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}
func (s *Store) GetDocument(ctx context.Context, id string) (repo.Document, error) {
	r, err := s.q.GetDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.Document{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.Document{}, err
	}
	return docFromGet(r), nil
}

func (s *Store) CreateServiceAccount(ctx context.Context, owner auth.Principal, name string) (repo.ServiceAccount, error) {
	if owner.Type != auth.PrincipalUser {
		return repo.ServiceAccount{}, errors.New("owner must be user")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.ServiceAccount{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if err := upsertUser(ctx, q, owner); err != nil {
		return repo.ServiceAccount{}, err
	}
	id := newID("sa")
	if err := q.InsertServiceAccount(ctx, dbsqlc.InsertServiceAccountParams{ID: id, Name: name, OwnerUserID: owner.ID}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return repo.ServiceAccount{}, repo.ErrConflict
		}
		return repo.ServiceAccount{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.ServiceAccount{}, err
	}
	return repo.ServiceAccount{ID: id, Name: name, Owner: owner}, nil
}
func (s *Store) GetServiceAccount(ctx context.Context, id string) (repo.ServiceAccount, error) {
	r, err := s.q.GetServiceAccount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.ServiceAccount{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.ServiceAccount{}, err
	}
	return repo.ServiceAccount{ID: r.ID, Name: r.Name, Disabled: r.Disabled, Owner: auth.Principal{Type: auth.PrincipalUser, ID: r.OwnerID, Email: r.OwnerEmail, Name: r.OwnerName}}, nil
}
func (s *Store) GetServiceAccountByName(ctx context.Context, name string) (repo.ServiceAccount, error) {
	r, err := s.q.GetServiceAccountByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.ServiceAccount{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.ServiceAccount{}, err
	}
	return repo.ServiceAccount{ID: r.ID, Name: r.Name, Disabled: r.Disabled, Owner: auth.Principal{Type: auth.PrincipalUser, ID: r.OwnerID, Email: r.OwnerEmail, Name: r.OwnerName}}, nil
}
func (s *Store) PrincipalExists(ctx context.Context, p auth.Principal) (bool, error) {
	switch p.Type {
	case auth.PrincipalUser:
		return s.q.UserExists(ctx, p.ID)
	case auth.PrincipalServiceAccount:
		return s.q.ServiceAccountExists(ctx, p.ID)
	default:
		return false, nil
	}
}
func (s *Store) CreateGrant(ctx context.Context, resourceType, resourceID string, principal auth.Principal, role repo.Role, grantedBy auth.Principal) (repo.Grant, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.Grant{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if grantedBy.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, grantedBy); err != nil {
			return repo.Grant{}, err
		}
	}
	if principal.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, principal); err != nil {
			return repo.Grant{}, err
		}
	}
	id := newID("gr")
	_, err = tx.Exec(ctx, `
		INSERT INTO resource_grants(id,resource_type,resource_id,principal_type,principal_id,role,granted_by_type,granted_by_user_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT(resource_type,resource_id,principal_type,principal_id)
		DO UPDATE SET role=EXCLUDED.role, revoked_at=NULL, granted_by_type=EXCLUDED.granted_by_type, granted_by_user_id=EXCLUDED.granted_by_user_id`,
		id, resourceType, resourceID, string(principal.Type), principal.ID, string(role), string(grantedBy.Type), grantedBy.ID)
	if err != nil {
		return repo.Grant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.Grant{}, err
	}
	return repo.Grant{ID: id, DocumentID: resourceID, ResourceType: resourceType, ResourceID: resourceID, Principal: principal, Role: role, GrantedBy: grantedBy}, nil
}

func (s *Store) CreateVersion(ctx context.Context, documentID, baseVersionID, changeSummary string, html []byte, createdBy auth.Principal) (repo.Version, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.Version{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	cur, err := q.GetDocumentVersionForUpdate(ctx, documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.Version{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.Version{}, err
	}
	if cur.CurrentVersionID.String != baseVersionID {
		return repo.Version{ID: cur.CurrentVersionID.String}, &repo.VersionConflictError{LatestVersionID: cur.CurrentVersionID.String}
	}
	id := newID("ver")
	blobKey := fmt.Sprintf("docs/%s/%s.html", documentID, id)
	sum := sha(html)
	if err := s.blobs.Put(ctx, blobKey, htmlContentType, html); err != nil {
		return repo.Version{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), blobKey)
		}
	}()
	if err := q.InsertVersion(ctx, dbsqlc.InsertVersionParams{ID: id, DocumentID: documentID, Number: cur.NextNumber, HtmlBlobKey: blobKey, Sha256: sum, ParentVersionID: pgtype.Text{String: cur.CurrentVersionID.String, Valid: cur.CurrentVersionID.Valid}, CreatedByType: string(createdBy.Type), CreatedByID: createdBy.ID, ChangeSummary: changeSummary}); err != nil {
		return repo.Version{}, err
	}
	if err := q.UpdateDocumentCurrentVersion(ctx, dbsqlc.UpdateDocumentCurrentVersionParams{CurrentVersionID: pgtype.Text{String: id, Valid: true}, ID: documentID}); err != nil {
		return repo.Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.Version{}, err
	}
	committed = true
	return repo.Version{ID: id, Number: int(cur.NextNumber), DocumentID: documentID, CreatedBy: createdBy, ChangeSummary: changeSummary, SHA256: sum, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}
func (s *Store) CreateComment(ctx context.Context, documentID, versionID, body string, anchor repo.Anchor, author auth.Principal) (repo.Comment, error) {
	v, err := s.GetVersion(ctx, versionID)
	if err != nil {
		return repo.Comment{}, err
	}
	if v.DocumentID != documentID {
		return repo.Comment{}, repo.ErrNotFound
	}
	b, err := json.Marshal(anchor)
	if err != nil {
		return repo.Comment{}, err
	}
	id := newID("cmt")
	err = s.q.InsertComment(ctx, dbsqlc.InsertCommentParams{ID: id, DocumentID: documentID, CreatedOnVersionID: versionID, AuthorType: string(author.Type), AuthorID: author.ID, AuthorEmail: author.Email, AuthorName: author.Name, Body: body, SelectedText: anchor.Quote, AnchorJson: b})
	if err != nil {
		return repo.Comment{}, err
	}
	return repo.Comment{ID: id, DocumentID: documentID, VersionID: versionID, Author: author, Body: body, SelectedText: anchor.Quote, Anchor: anchor, Status: repo.StatusOpen, RootCommentID: id, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}
func (s *Store) CreateCommentReply(ctx context.Context, documentID, parentCommentID, body string, author auth.Principal) (repo.Comment, error) {
	parent, err := s.GetComment(ctx, parentCommentID)
	if err != nil {
		return repo.Comment{}, err
	}
	if parent.DocumentID != documentID {
		return repo.Comment{}, repo.ErrNotFound
	}
	rootID := parent.RootCommentID
	if rootID == "" {
		rootID = parent.ID
	}
	id := newID("cmt")
	err = s.q.InsertCommentReply(ctx, dbsqlc.InsertCommentReplyParams{ID: id, DocumentID: documentID, CreatedOnVersionID: parent.VersionID, AuthorType: string(author.Type), AuthorID: author.ID, AuthorEmail: author.Email, AuthorName: author.Name, Body: body, ParentCommentID: pgtype.Text{String: parent.ID, Valid: true}, RootCommentID: rootID})
	if err != nil {
		return repo.Comment{}, err
	}
	return repo.Comment{ID: id, DocumentID: documentID, VersionID: parent.VersionID, Author: author, Body: body, Status: repo.StatusOpen, ParentCommentID: parent.ID, RootCommentID: rootID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}
func upsertUser(ctx context.Context, q *dbsqlc.Queries, p auth.Principal) error {
	return q.UpsertUser(ctx, dbsqlc.UpsertUserParams{ID: p.ID, Email: p.Email, Name: p.Name})
}
func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

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

func pgTimeRFC3339(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339Nano)
}

func (s *Store) ListDocuments(ctx context.Context, p auth.Principal) ([]repo.Document, error) {
	rows, err := s.q.ListDocuments(ctx, dbsqlc.ListDocumentsParams{OwnerID: p.ID, OwnerType: string(p.Type)})
	if err != nil {
		return nil, err
	}
	out := make([]repo.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, docFromList(r))
	}
	return out, nil
}
func (s *Store) SearchDocumentsByTitle(ctx context.Context, p auth.Principal, query string, includeOrgWide bool, limit int32) ([]repo.Document, error) {
	rows, err := s.q.SearchDocumentsByTitle(ctx, dbsqlc.SearchDocumentsByTitleParams{Column1: pgtype.Text{String: query, Valid: true}, OwnerID: p.ID, OwnerType: string(p.Type), Column4: includeOrgWide, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]repo.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, docFromSearch(r))
	}
	return out, nil
}
func (s *Store) RecordDocumentOpened(ctx context.Context, documentID string, p auth.Principal) error {
	return s.q.RecordWorkspaceEntry(ctx, dbsqlc.RecordWorkspaceEntryParams{
		DocumentID:    documentID,
		PrincipalType: string(p.Type),
		PrincipalID:   p.ID,
	})
}
func (s *Store) RecordFolderOpened(ctx context.Context, folderID string, p auth.Principal) error {
	_, err := s.db.Exec(ctx, `INSERT INTO folder_workspace_entries(folder_id,principal_type,principal_id,last_opened_at) VALUES($1,$2,$3,now()) ON CONFLICT (folder_id,principal_type,principal_id) DO UPDATE SET last_opened_at=now()`, folderID, string(p.Type), p.ID)
	return err
}
func (s *Store) UpdateDocument(ctx context.Context, id, title string) (repo.Document, error) {
	if title != "" {
		if err := s.q.UpdateDocumentTitle(ctx, dbsqlc.UpdateDocumentTitleParams{Title: title, ID: id}); err != nil {
			return repo.Document{}, err
		}
	}
	return s.GetDocument(ctx, id)
}
func (s *Store) DeleteDocument(ctx context.Context, id string) error {
	return s.q.DeleteDocument(ctx, id)
}
func (s *Store) ListServiceAccounts(ctx context.Context, owner auth.Principal) ([]repo.ServiceAccount, error) {
	rows, err := s.q.ListServiceAccounts(ctx, owner.ID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.ServiceAccount, 0, len(rows))
	for _, r := range rows {
		out = append(out, repo.ServiceAccount{ID: r.ID, Name: r.Name, Disabled: r.Disabled, Owner: auth.Principal{Type: auth.PrincipalUser, ID: r.OwnerID, Email: r.OwnerEmail, Name: r.OwnerName}})
	}
	return out, nil
}
func (s *Store) UpdateServiceAccount(ctx context.Context, id, name string, disabled *bool) (repo.ServiceAccount, error) {
	if name != "" {
		if err := s.q.UpdateServiceAccountName(ctx, dbsqlc.UpdateServiceAccountNameParams{Name: name, ID: id}); err != nil {
			return repo.ServiceAccount{}, err
		}
	}
	if disabled != nil {
		if *disabled {
			if err := s.q.DisableServiceAccount(ctx, id); err != nil {
				return repo.ServiceAccount{}, err
			}
		} else {
			if err := s.q.EnableServiceAccount(ctx, id); err != nil {
				return repo.ServiceAccount{}, err
			}
		}
	}
	return s.GetServiceAccount(ctx, id)
}
func (s *Store) CreateServiceAccountKey(ctx context.Context, saID, name, tokenHash string) (string, error) {
	id := newID("sak")
	err := s.q.InsertServiceAccountKey(ctx, dbsqlc.InsertServiceAccountKeyParams{ID: id, ServiceAccountID: saID, Name: name, TokenHash: tokenHash})
	return id, err
}
func (s *Store) ListServiceAccountKeys(ctx context.Context, saID string) ([]repo.ServiceAccountKey, error) {
	rows, err := s.q.ListServiceAccountKeys(ctx, saID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.ServiceAccountKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, repo.ServiceAccountKey{ID: r.ID, Name: r.Name})
	}
	return out, nil
}
func (s *Store) RevokeServiceAccountKey(ctx context.Context, saID, keyID string) error {
	return s.q.RevokeServiceAccountKey(ctx, dbsqlc.RevokeServiceAccountKeyParams{ServiceAccountID: saID, ID: keyID})
}
func (s *Store) ListGrants(ctx context.Context, resourceID string) ([]repo.Grant, error) {
	rows, err := s.db.Query(ctx, `
		SELECT g.id, g.resource_type, g.resource_id, g.principal_type, g.principal_id,
		       COALESCE(u.email,'') AS principal_email,
		       COALESCE(u.name,sa.name,'') AS principal_name,
		       COALESCE(u.picture_url,'') AS principal_picture_url,
		       g.role, g.granted_by_type, g.granted_by_user_id,
		       COALESCE(gu.email,'') AS granted_by_email,
		       COALESCE(gu.name,gsa.name,'') AS granted_by_name
		FROM resource_grants g
		LEFT JOIN users u ON g.principal_type='user' AND g.principal_id=u.id
		LEFT JOIN service_accounts sa ON g.principal_type='service_account' AND g.principal_id=sa.id
		LEFT JOIN users gu ON g.granted_by_type='user' AND g.granted_by_user_id=gu.id
		LEFT JOIN service_accounts gsa ON g.granted_by_type='service_account' AND g.granted_by_user_id=gsa.id
		WHERE g.resource_id=$1 AND g.revoked_at IS NULL`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repo.Grant{}
	for rows.Next() {
		var g repo.Grant
		var pt, gbType string
		if err := rows.Scan(&g.ID, &g.ResourceType, &g.ResourceID, &pt, &g.Principal.ID,
			&g.Principal.Email, &g.Principal.Name, &g.Principal.PictureURL,
			&g.Role, &gbType, &g.GrantedBy.ID, &g.GrantedBy.Email, &g.GrantedBy.Name); err != nil {
			return nil, err
		}
		g.DocumentID = resourceID
		g.Principal.Type = auth.PrincipalType(pt)
		g.GrantedBy.Type = auth.PrincipalType(gbType)
		out = append(out, g)
	}
	return out, nil
}
func (s *Store) UpdateGrant(ctx context.Context, resourceID, grantID string, role repo.Role) (repo.Grant, error) {
	row := s.db.QueryRow(ctx, `
		UPDATE resource_grants SET role=$1
		WHERE id=$2 AND resource_id=$3 AND revoked_at IS NULL
		RETURNING id, resource_type, resource_id, principal_type, principal_id, role, granted_by_type, granted_by_user_id`,
		string(role), grantID, resourceID)
	var g repo.Grant
	var pt, gbType string
	err := row.Scan(&g.ID, &g.ResourceType, &g.ResourceID, &pt, &g.Principal.ID, &g.Role, &gbType, &g.GrantedBy.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.Grant{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.Grant{}, err
	}
	g.DocumentID = resourceID
	g.Principal.Type = auth.PrincipalType(pt)
	g.GrantedBy.Type = auth.PrincipalType(gbType)
	return g, nil
}
func (s *Store) DeleteGrant(ctx context.Context, resourceID, grantID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE resource_grants SET revoked_at=now() WHERE id=$1 AND resource_id=$2 AND revoked_at IS NULL`, grantID, resourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}
func (s *Store) ListVersions(ctx context.Context, documentID string) ([]repo.Version, error) {
	rows, err := s.q.ListVersions(ctx, documentID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.Version, 0, len(rows))
	for _, r := range rows {
		out = append(out, repo.Version{ID: r.ID, Number: int(r.Number), DocumentID: r.DocumentID, CreatedBy: auth.Principal{Type: auth.PrincipalType(r.CreatedByType), ID: r.CreatedByID, Email: r.CreatedByEmail, Name: r.CreatedByName, PictureURL: r.CreatedByPictureUrl}, ChangeSummary: r.ChangeSummary, SHA256: r.Sha256, CreatedAt: pgTimeRFC3339(r.CreatedAt)})
	}
	return out, nil
}
func (s *Store) GetVersion(ctx context.Context, id string) (repo.Version, error) {
	r, err := s.q.GetVersion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.Version{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.Version{}, err
	}
	return repo.Version{ID: r.ID, Number: int(r.Number), DocumentID: r.DocumentID, CreatedBy: auth.Principal{Type: auth.PrincipalType(r.CreatedByType), ID: r.CreatedByID, Email: r.CreatedByEmail, Name: r.CreatedByName, PictureURL: r.CreatedByPictureUrl}, ChangeSummary: r.ChangeSummary, SHA256: r.Sha256, CreatedAt: pgTimeRFC3339(r.CreatedAt)}, nil
}
func (s *Store) GetVersionHTML(ctx context.Context, id string) ([]byte, error) {
	key, err := s.q.GetVersionBlobKey(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repo.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b, _, err := s.blobs.Get(ctx, key)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, repo.ErrNotFound
	}
	return b, err
}
func (s *Store) GetComment(ctx context.Context, id string) (repo.Comment, error) {
	r, err := s.q.GetComment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.Comment{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.Comment{}, err
	}
	return commentFromGet(r), nil
}
func (s *Store) ListComments(ctx context.Context, documentID, status, versionID string) ([]repo.Comment, error) {
	rows, err := s.q.ListComments(ctx, dbsqlc.ListCommentsParams{DocumentID: documentID, Column2: status, Column3: versionID})
	if err != nil {
		return nil, err
	}
	out := make([]repo.Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, commentFromList(r))
	}
	return out, nil
}
func (s *Store) UpdateComment(ctx context.Context, id, body, status string) (repo.Comment, error) {
	if body != "" {
		if err := s.q.UpdateCommentBody(ctx, dbsqlc.UpdateCommentBodyParams{Body: body, ID: id}); err != nil {
			return repo.Comment{}, err
		}
	}
	if status != "" {
		if err := s.q.UpdateCommentStatus(ctx, dbsqlc.UpdateCommentStatusParams{Status: status, ID: id}); err != nil {
			return repo.Comment{}, err
		}
	}
	return s.GetComment(ctx, id)
}
func (s *Store) DeleteComment(ctx context.Context, id string) error {
	return s.q.DeleteComment(ctx, id)
}

func (s *Store) ListDocumentKV(ctx context.Context, documentID string) (repo.DocumentKVSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return repo.DocumentKVSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if _, err := q.GetDocumentOwner(ctx, documentID); errors.Is(err, pgx.ErrNoRows) {
		return repo.DocumentKVSnapshot{}, repo.ErrNotFound
	} else if err != nil {
		return repo.DocumentKVSnapshot{}, err
	}
	rows, err := q.ListDocumentKV(ctx, documentID)
	if err != nil {
		return repo.DocumentKVSnapshot{}, err
	}
	rev, err := q.GetDocumentKVRevision(ctx, documentID)
	if err != nil {
		return repo.DocumentKVSnapshot{}, err
	}
	items := make(map[string]json.RawMessage, len(rows))
	for _, row := range rows {
		items[row.Key] = append(json.RawMessage(nil), row.Value...)
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.DocumentKVSnapshot{}, err
	}
	return repo.DocumentKVSnapshot{Revision: rev, Items: items}, nil
}

func (s *Store) ListDocumentKVEvents(ctx context.Context, documentID string, afterRevision int64, limit int32) ([]repo.DocumentKVEvent, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if _, err := q.GetDocumentOwner(ctx, documentID); errors.Is(err, pgx.ErrNoRows) {
		return nil, repo.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.ListDocumentKVEventsAfter(ctx, dbsqlc.ListDocumentKVEventsAfterParams{
		DocumentID: documentID,
		Revision:   afterRevision,
		Limit:      limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]repo.DocumentKVEvent, 0, len(rows))
	for _, row := range rows {
		var changes []repo.DocumentKVChange
		if err := json.Unmarshal(row.Changes, &changes); err != nil {
			return nil, err
		}
		actor := auth.Principal{
			Type:  auth.PrincipalType(row.UpdatedByType),
			ID:    row.UpdatedByID,
			Email: row.UpdatedByEmail,
			Name:  row.UpdatedByName,
		}
		out = append(out, repo.DocumentKVEvent{
			DocumentID: row.DocumentID,
			Revision:   row.Revision,
			Changes:    changes,
			UpdatedAt:  pgTimeRFC3339(row.UpdatedAt),
			UpdatedBy:  &actor,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) PatchDocumentKV(ctx context.Context, documentID string, set map[string]json.RawMessage, deleteKeys []string, actor auth.Principal) (repo.DocumentKVPatchResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.DocumentKVPatchResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if _, err := q.LockDocumentForKV(ctx, documentID); errors.Is(err, pgx.ErrNoRows) {
		return repo.DocumentKVPatchResult{}, repo.ErrNotFound
	} else if err != nil {
		return repo.DocumentKVPatchResult{}, err
	}
	currentRev, err := q.GetDocumentKVRevision(ctx, documentID)
	if err != nil {
		return repo.DocumentKVPatchResult{}, err
	}
	if len(set) == 0 && len(deleteKeys) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return repo.DocumentKVPatchResult{}, err
		}
		return repo.DocumentKVPatchResult{Revision: currentRev, Changes: []repo.DocumentKVChange{}, UpdatedBy: &actor}, nil
	}
	rev := currentRev + 1
	now := time.Now().UTC().Format(time.RFC3339Nano)
	changes := make([]repo.DocumentKVChange, 0, len(set)+len(deleteKeys))
	actorType := string(actor.Type)
	for _, key := range sortedKVKeys(set) {
		value := append(json.RawMessage(nil), set[key]...)
		if err := q.UpsertDocumentKV(ctx, dbsqlc.UpsertDocumentKVParams{DocumentID: documentID, Key: key, Value: value, Revision: rev, UpdatedByType: actorType, UpdatedByID: actor.ID, UpdatedByEmail: actor.Email, UpdatedByName: actor.Name}); err != nil {
			return repo.DocumentKVPatchResult{}, err
		}
		changes = append(changes, repo.DocumentKVChange{Key: key, Value: value, Revision: rev, UpdatedAt: now, UpdatedBy: &actor, Operation: "set"})
	}
	for _, key := range sortedStrings(deleteKeys) {
		if err := q.DeleteDocumentKV(ctx, dbsqlc.DeleteDocumentKVParams{DocumentID: documentID, Key: key, Revision: rev, UpdatedByType: actorType, UpdatedByID: actor.ID, UpdatedByEmail: actor.Email, UpdatedByName: actor.Name}); err != nil {
			return repo.DocumentKVPatchResult{}, err
		}
		changes = append(changes, repo.DocumentKVChange{Key: key, Deleted: true, Revision: rev, UpdatedAt: now, UpdatedBy: &actor, Operation: "delete"})
	}
	rawChanges, err := json.Marshal(changes)
	if err != nil {
		return repo.DocumentKVPatchResult{}, err
	}
	if err := q.InsertDocumentKVEvent(ctx, dbsqlc.InsertDocumentKVEventParams{DocumentID: documentID, Revision: rev, Changes: rawChanges, UpdatedByType: actorType, UpdatedByID: actor.ID, UpdatedByEmail: actor.Email, UpdatedByName: actor.Name}); err != nil {
		return repo.DocumentKVPatchResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.DocumentKVPatchResult{}, err
	}
	return repo.DocumentKVPatchResult{Revision: rev, Changes: changes, UpdatedBy: &actor}, nil
}

func (s *Store) CreateOwnershipTransfer(ctx context.Context, saID string, from auth.Principal, toEmail string) (repo.OwnershipTransfer, error) {
	toID, err := s.q.GetUserIDByEmail(ctx, toEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		toID = newID("usr")
		err = s.q.InsertPlaceholderUser(ctx, dbsqlc.InsertPlaceholderUserParams{ID: toID, Email: toEmail})
	}
	if err != nil {
		return repo.OwnershipTransfer{}, err
	}
	id := newID("xfer")
	err = s.q.InsertOwnershipTransfer(ctx, dbsqlc.InsertOwnershipTransferParams{ID: id, ServiceAccountID: saID, FromUserID: from.ID, ToUserID: toID})
	return repo.OwnershipTransfer{ID: id, ServiceAccountID: saID, FromUserID: from.ID, ToUserID: toID, Status: repo.StatusPending}, err
}
func (s *Store) ListOwnershipTransfers(ctx context.Context, user auth.Principal) ([]repo.OwnershipTransfer, error) {
	rows, err := s.q.ListOwnershipTransfers(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.OwnershipTransfer, 0, len(rows))
	for _, r := range rows {
		out = append(out, repo.OwnershipTransfer{ID: r.ID, ServiceAccountID: r.ServiceAccountID, FromUserID: r.FromUserID, ToUserID: r.ToUserID, Status: r.Status})
	}
	return out, nil
}
func (s *Store) AcceptOwnershipTransfer(ctx context.Context, id string, user auth.Principal) (repo.OwnershipTransfer, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.OwnershipTransfer{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	r, err := q.GetOwnershipTransferForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.OwnershipTransfer{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.OwnershipTransfer{}, err
	}
	x := repo.OwnershipTransfer{ID: r.ID, ServiceAccountID: r.ServiceAccountID, FromUserID: r.FromUserID, ToUserID: r.ToUserID, Status: r.Status}
	if x.ToUserID != user.ID {
		return x, repo.ErrNotTransferTarget
	}
	if x.Status != repo.StatusPending {
		return x, repo.ErrTransferNotPending
	}
	if err := q.UpdateServiceAccountOwner(ctx, dbsqlc.UpdateServiceAccountOwnerParams{OwnerUserID: user.ID, ID: x.ServiceAccountID}); err != nil {
		return x, err
	}
	if _, err := q.AcceptOwnershipTransfer(ctx, id); err != nil {
		return x, err
	}
	x.Status = repo.StatusAccepted
	return x, tx.Commit(ctx)
}
func (s *Store) DeclineOwnershipTransfer(ctx context.Context, id string, user auth.Principal) error {
	_, err := s.q.DeclineOwnershipTransfer(ctx, dbsqlc.DeclineOwnershipTransferParams{ID: id, FromUserID: user.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.ErrNotFound
	}
	return err
}

func docFromGet(r dbsqlc.GetDocumentRow) repo.Document {
	return repo.Document{ID: r.ID, Title: r.Title, CurrentVersionID: r.CurrentVersionID.String, Owner: auth.Principal{Type: auth.PrincipalType(r.OwnerType), ID: r.OwnerID, Email: r.OwnerEmail, Name: r.OwnerName}, ParentFolderID: r.ParentFolderID}
}
func docFromList(r dbsqlc.ListDocumentsRow) repo.Document {
	d := repo.Document{ID: r.ID, Title: r.Title, CurrentVersionID: r.CurrentVersionID.String, Owner: auth.Principal{Type: auth.PrincipalType(r.OwnerType), ID: r.OwnerID, Email: r.OwnerEmail, Name: r.OwnerName}}
	if r.UpdatedAt.Valid {
		d.UpdatedAt = r.UpdatedAt.Time.Format(time.RFC3339)
	}
	return d
}
func docFromSearch(r dbsqlc.SearchDocumentsByTitleRow) repo.Document {
	return repo.Document{ID: r.ID, Title: r.Title, CurrentVersionID: r.CurrentVersionID.String, Owner: auth.Principal{Type: auth.PrincipalType(r.OwnerType), ID: r.OwnerID, Email: r.OwnerEmail, Name: r.OwnerName}}
}
func commentFromGet(r dbsqlc.Comment) repo.Comment {
	c := repo.Comment{ID: r.ID, DocumentID: r.DocumentID, VersionID: r.CreatedOnVersionID, Author: auth.Principal{Type: auth.PrincipalType(r.AuthorType), ID: r.AuthorID, Email: r.AuthorEmail, Name: r.AuthorName}, Body: r.Body, SelectedText: r.SelectedText, Status: r.Status, RootCommentID: r.RootCommentID, CreatedAt: pgTimeRFC3339(r.CreatedAt), ResolvedAt: pgTimeRFC3339(r.ResolvedAt)}
	if r.ParentCommentID.Valid {
		c.ParentCommentID = r.ParentCommentID.String
	}
	_ = json.Unmarshal(r.AnchorJson, &c.Anchor)
	return c
}
func commentFromList(r dbsqlc.Comment) repo.Comment {
	c := repo.Comment{ID: r.ID, DocumentID: r.DocumentID, VersionID: r.CreatedOnVersionID, Author: auth.Principal{Type: auth.PrincipalType(r.AuthorType), ID: r.AuthorID, Email: r.AuthorEmail, Name: r.AuthorName}, Body: r.Body, SelectedText: r.SelectedText, Status: r.Status, RootCommentID: r.RootCommentID, CreatedAt: pgTimeRFC3339(r.CreatedAt), ResolvedAt: pgTimeRFC3339(r.ResolvedAt)}
	if r.ParentCommentID.Valid {
		c.ParentCommentID = r.ParentCommentID.String
	}
	_ = json.Unmarshal(r.AnchorJson, &c.Anchor)
	return c
}

func (s *Store) RoleForFolder(ctx context.Context, p auth.Principal, folderID string) (repo.Role, error) {
	rows, err := s.db.Query(ctx, `WITH RECURSIVE a(id,parent_folder_id,depth) AS (SELECT id,parent_folder_id,1 FROM folders WHERE id=$1 UNION ALL SELECT f.id,f.parent_folder_id,a.depth+1 FROM folders f JOIN a ON f.id=a.parent_folder_id WHERE a.depth<64) SELECT resource_id,principal_type,principal_id,role FROM resource_grants WHERE revoked_at IS NULL AND resource_type='folder' AND resource_id IN (SELECT id FROM a)`, folderID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	f, err := s.GetFolder(ctx, folderID)
	if err != nil {
		return "", err
	}
	best := repo.RoleNone
	if f.Owner.Type == p.Type && f.Owner.ID == p.ID {
		best = repo.RoleOwner
	}
	for rows.Next() {
		var rid, pt, pid, role string
		if err := rows.Scan(&rid, &pt, &pid, &role); err != nil {
			return "", err
		}
		if (pt == string(p.Type) && pid == p.ID) || pt == string(auth.PrincipalAnyone) {
			best = repo.MaxRole(best, repo.Role(role))
		}
	}
	if best == repo.RoleNone {
		return "", repo.ErrNotFound
	}
	return best, nil
}
func (s *Store) CreateFolder(ctx context.Context, owner auth.Principal, title, parent string) (repo.Folder, error) {
	if parent != "" {
		depth, err := s.folderDepth(ctx, parent)
		if err != nil {
			return repo.Folder{}, err
		}
		if depth+1 > 5 {
			return repo.Folder{}, repo.ErrConflict
		}
	}
	id := newID("fld")
	_, err := s.db.Exec(ctx, `INSERT INTO folders(id,title,owner_type,owner_id,parent_folder_id) VALUES($1,$2,$3,$4,NULLIF($5,''))`, id, title, string(owner.Type), owner.ID, parent)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return repo.Folder{}, repo.ErrConflict
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return repo.Folder{}, repo.ErrNotFound
		}
		return repo.Folder{}, err
	}
	return repo.Folder{ID: id, Title: title, Owner: owner, ParentFolderID: parent}, nil
}

func (s *Store) folderDepth(ctx context.Context, id string) (int, error) {
	var depth int
	err := s.db.QueryRow(ctx, `
		WITH RECURSIVE a(id, depth) AS (
			SELECT id, 1 FROM folders WHERE id=$1
			UNION ALL
			SELECT f.id, a.depth+1 FROM folders f JOIN a ON f.id=(SELECT parent_folder_id FROM folders WHERE id=a.id) WHERE a.depth < 64 AND (SELECT parent_folder_id FROM folders WHERE id=a.id) IS NOT NULL
		)
		SELECT COALESCE(MAX(depth),0) FROM a`, id).Scan(&depth)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, repo.ErrNotFound
	}
	return depth, err
}

func (s *Store) subtreeHeight(ctx context.Context, id string) (int, error) {
	var height int
	err := s.db.QueryRow(ctx, `
		WITH RECURSIVE d(id, depth) AS (
			SELECT $1::text, 1
			UNION ALL
			SELECT f.id, d.depth+1 FROM folders f JOIN d ON f.parent_folder_id=d.id WHERE d.depth < 64
		)
		SELECT COALESCE(MAX(depth),0) FROM d`, id).Scan(&height)
	return height, err
}
func (s *Store) GetFolder(ctx context.Context, id string) (repo.Folder, error) {
	var f repo.Folder
	var pt string
	err := s.db.QueryRow(ctx, `
		SELECT f.id, f.title, f.owner_type, f.owner_id, COALESCE(f.parent_folder_id,''),
		       COALESCE(u.name,sa.name,''), COALESCE(u.email,'')
		FROM folders f
		LEFT JOIN users u ON f.owner_type='user' AND f.owner_id=u.id
		LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND f.owner_id=sa.id
		WHERE f.id=$1`, id).Scan(&f.ID, &f.Title, &pt, &f.Owner.ID, &f.ParentFolderID, &f.Owner.Name, &f.Owner.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.Folder{}, repo.ErrNotFound
	}
	f.Owner.Type = auth.PrincipalType(pt)
	if err == nil && f.ParentFolderID != "" {
		f.Ancestors, _ = s.folderAncestors(ctx, f.ParentFolderID)
	}
	return f, err
}

func (s *Store) folderAncestors(ctx context.Context, parentID string) ([]repo.FolderBreadcrumb, error) {
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE anc(id, title, parent, depth) AS (
			SELECT f.id, f.title, COALESCE(f.parent_folder_id,''), 1
			FROM folders f WHERE f.id=$1
			UNION ALL
			SELECT f.id, f.title, COALESCE(f.parent_folder_id,''), a.depth+1
			FROM folders f JOIN anc a ON f.id=a.parent
			WHERE a.parent != '' AND a.depth < 10
		)
		SELECT id, title FROM anc ORDER BY depth DESC`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []repo.FolderBreadcrumb
	for rows.Next() {
		var b repo.FolderBreadcrumb
		if err := rows.Scan(&b.ID, &b.Title); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Store) UpdateFolder(ctx context.Context, id, title string, parent *string) (repo.Folder, error) {
	f, err := s.GetFolder(ctx, id)
	if err != nil {
		return f, err
	}
	if title == "" {
		title = f.Title
	}
	p := f.ParentFolderID
	if parent != nil {
		p = *parent
		if p == id {
			return repo.Folder{}, repo.ErrConflict
		}
		if p != "" {
			if _, err := s.GetFolder(ctx, p); err != nil {
				return repo.Folder{}, err
			}
			var isCycle bool
			err := s.db.QueryRow(ctx, `
				WITH RECURSIVE a(id, depth) AS (
					SELECT $1::text, 1
					UNION ALL
					SELECT (SELECT parent_folder_id FROM folders WHERE id=a.id), a.depth+1
					FROM a WHERE a.depth < 64 AND (SELECT parent_folder_id FROM folders WHERE id=a.id) IS NOT NULL
				)
				SELECT EXISTS(SELECT 1 FROM a WHERE id=$2)`, p, id).Scan(&isCycle)
			if err != nil {
				return repo.Folder{}, err
			}
			if isCycle {
				return repo.Folder{}, repo.ErrConflict
			}
			parentDepth, err := s.folderDepth(ctx, p)
			if err != nil {
				return repo.Folder{}, err
			}
			subHeight, err := s.subtreeHeight(ctx, id)
			if err != nil {
				return repo.Folder{}, err
			}
			if parentDepth+subHeight > 5 {
				return repo.Folder{}, repo.ErrConflict
			}
		}
	}
	_, err = s.db.Exec(ctx, `UPDATE folders SET title=$2,parent_folder_id=NULLIF($3,''),updated_at=now() WHERE id=$1`, id, title, p)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return repo.Folder{}, repo.ErrConflict
		}
		return repo.Folder{}, err
	}
	return s.GetFolder(ctx, id)
}
func (s *Store) DeleteFolder(ctx context.Context, id string, p auth.Principal) error {
	var exists, topOwner bool
	if err := s.db.QueryRow(ctx, `
SELECT
  EXISTS(SELECT 1 FROM folders WHERE id=$1),
  EXISTS(SELECT 1 FROM folders WHERE id=$1 AND owner_type=$2 AND owner_id=$3)`, id, string(p.Type), p.ID).Scan(&exists, &topOwner); err != nil {
		return err
	}
	if !exists {
		return repo.ErrNotFound
	}
	if !topOwner {
		return repo.ErrForbidden
	}
	tag, err := s.db.Exec(ctx, `WITH RECURSIVE d(id,depth) AS (SELECT $1::text,1 UNION ALL SELECT f.id,d.depth+1 FROM folders f JOIN d ON f.parent_folder_id=d.id WHERE d.depth<64) DELETE FROM folders WHERE id IN (SELECT id FROM d)`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}
func (s *Store) ListFolder(ctx context.Context, p auth.Principal, folderID, path string) ([]repo.FolderEntry, bool, *repo.Folder, error) {
	parent := folderID
	if path != "" && path != "/" {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		for i, part := range parts {
			var found string
			var err error
			if i == 0 && parent == "" {
				rows, err2 := s.db.Query(ctx, `
					SELECT f.id, f.title, f.owner_type, f.owner_id,
					       COALESCE(u.name,sa.name,'') AS owner_name, COALESCE(u.email,'') AS owner_email
					FROM folders f
					LEFT JOIN users u ON f.owner_type='user' AND f.owner_id=u.id
					LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND f.owner_id=sa.id
					WHERE COALESCE(f.parent_folder_id,'')=''
					  AND lower(f.title)=lower($1)
					  AND (
					    (f.owner_type=$2 AND f.owner_id=$3)
					    OR EXISTS (
					      SELECT 1 FROM resource_grants g
					      WHERE g.resource_type='folder' AND g.resource_id=f.id
					        AND g.revoked_at IS NULL
					        AND ((g.principal_type=$2 AND g.principal_id=$3) OR g.principal_type='anyone')
					    )
					  )
					ORDER BY CASE WHEN f.owner_type=$2 AND f.owner_id=$3 THEN 0 ELSE 1 END`, part, string(p.Type), p.ID)
				if err2 != nil {
					return nil, false, nil, err2
				}
				var candidates []repo.Folder
				for rows.Next() {
					var f repo.Folder
					var pt, ownName, ownEmail string
					rows.Scan(&f.ID, &f.Title, &pt, &f.Owner.ID, &ownName, &ownEmail)
					f.Owner.Type = auth.PrincipalType(pt)
					f.Owner.Name = ownName
					f.Owner.Email = ownEmail
					candidates = append(candidates, f)
				}
				rows.Close()
				if len(candidates) == 0 {
					return nil, false, nil, repo.ErrNotFound
				}
				if len(candidates) > 1 {
					return nil, false, nil, &repo.AmbiguousPathError{Segment: part, Candidates: candidates}
				}
				found = candidates[0].ID
				err = nil
			} else {
				err = s.db.QueryRow(ctx, `SELECT id FROM folders WHERE parent_folder_id=$1 AND lower(title)=lower($2) LIMIT 1`, parent, part).Scan(&found)
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, false, nil, repo.ErrNotFound
			}
			if err != nil {
				return nil, false, nil, err
			}
			parent = found
		}
	}

	var rows pgx.Rows
	var err2 error
	isRoot := parent == ""
	if isRoot {
		rows, err2 = s.db.Query(ctx, `
			SELECT f.id, f.title, f.owner_type, f.owner_id, COALESCE(f.parent_folder_id,''),
			       COALESCE(u.name,sa.name,'') AS owner_name, COALESCE(u.email,'') AS owner_email,
			       COALESCE(we.last_opened_at, f.created_at) AS last_active
			FROM folders f
			LEFT JOIN users u ON f.owner_type='user' AND f.owner_id=u.id
			LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND f.owner_id=sa.id
			LEFT JOIN folder_workspace_entries we ON we.folder_id=f.id AND we.principal_type=$1 AND we.principal_id=$2
			WHERE (
			  COALESCE(f.parent_folder_id,'')=''
			  AND (
			    (f.owner_type=$1 AND f.owner_id=$2)
			    OR EXISTS (
			      SELECT 1 FROM resource_grants g
			      WHERE g.resource_type='folder' AND g.resource_id=f.id
			        AND g.revoked_at IS NULL
			        AND g.principal_type=$1 AND g.principal_id=$2
			    )
			    OR (
			      EXISTS (
			        SELECT 1 FROM folder_workspace_entries w
			        WHERE w.folder_id=f.id
			          AND w.principal_type=$1 AND w.principal_id=$2
			      )
			      AND EXISTS (
			        SELECT 1 FROM resource_grants g
			        WHERE g.resource_type='folder' AND g.resource_id=f.id
			          AND g.revoked_at IS NULL
			          AND g.principal_type='anyone'
			      )
			    )
			  )
			)
			OR (
			  f.parent_folder_id IS NOT NULL
			  AND EXISTS (
			    SELECT 1 FROM folder_workspace_entries w
			    WHERE w.folder_id=f.id
			      AND w.principal_type=$1 AND w.principal_id=$2
			  )
			  AND NOT EXISTS (
			    WITH RECURSIVE anc(fid, depth) AS (
			      SELECT f.parent_folder_id, 1
			      UNION ALL
			      SELECT p.parent_folder_id, a.depth+1 FROM folders p JOIN anc a ON p.id=a.fid WHERE p.parent_folder_id IS NOT NULL AND a.depth < 20
			    )
			    SELECT 1 FROM folder_workspace_entries fw
			    JOIN anc ON fw.folder_id=anc.fid
			    WHERE fw.principal_type=$1 AND fw.principal_id=$2
			  )
			)
			ORDER BY COALESCE(we.last_opened_at, f.created_at) DESC
			LIMIT 51`, string(p.Type), p.ID)
	} else {
		rows, err2 = s.db.Query(ctx, `
			SELECT f.id, f.title, f.owner_type, f.owner_id, COALESCE(f.parent_folder_id,''),
			       COALESCE(u.name,sa.name,'') AS owner_name, COALESCE(u.email,'') AS owner_email
			FROM folders f
			LEFT JOIN users u ON f.owner_type='user' AND f.owner_id=u.id
			LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND f.owner_id=sa.id
			WHERE COALESCE(f.parent_folder_id,'')=$1
			ORDER BY f.title
			LIMIT 51`, parent)
	}
	if err2 != nil {
		return nil, false, nil, err2
	}
	defer rows.Close()
	out := []repo.FolderEntry{}
	for rows.Next() {
		var f repo.Folder
		var pt string
		var lastActive time.Time
		if isRoot {
			if err := rows.Scan(&f.ID, &f.Title, &pt, &f.Owner.ID, &f.ParentFolderID, &f.Owner.Name, &f.Owner.Email, &lastActive); err != nil {
				return nil, false, nil, err
			}
		} else {
			if err := rows.Scan(&f.ID, &f.Title, &pt, &f.Owner.ID, &f.ParentFolderID, &f.Owner.Name, &f.Owner.Email); err != nil {
				return nil, false, nil, err
			}
		}
		f.Owner.Type = auth.PrincipalType(pt)
		if len(out) < 50 {
			ff := f
			entry := repo.FolderEntry{Type: "folder", Folder: &ff}
			if isRoot {
				entry.LastActive = lastActive.Format(time.RFC3339)
			}
			out = append(out, entry)
		}
	}
	has := len(out) == 50

	if parent != "" {
		docRows, err := s.db.Query(ctx, `
			SELECT d.id, d.title, d.owner_type, d.owner_id,
			       COALESCE(d.current_version_id,''), COALESCE(d.parent_folder_id,''),
			       COALESCE(u.name,sa.name,''), COALESCE(u.email,'')
			FROM documents d
			LEFT JOIN users u ON d.owner_type='user' AND d.owner_id=u.id
			LEFT JOIN service_accounts sa ON d.owner_type='service_account' AND d.owner_id=sa.id
			WHERE d.parent_folder_id=$1`, parent)
		if err != nil {
			return nil, false, nil, err
		}
		defer docRows.Close()
		for docRows.Next() {
			var d repo.Document
			var pt string
			if err := docRows.Scan(&d.ID, &d.Title, &pt, &d.Owner.ID, &d.CurrentVersionID, &d.ParentFolderID, &d.Owner.Name, &d.Owner.Email); err != nil {
				return nil, false, nil, err
			}
			d.Owner.Type = auth.PrincipalType(pt)
			out = append(out, repo.FolderEntry{Type: "document", Document: &d})
		}
	}

	var cur *repo.Folder
	if parent != "" {
		f, _ := s.GetFolder(ctx, parent)
		cur = &f
	}
	return out, has, cur, nil
}
func (s *Store) SearchFoldersByTitle(ctx context.Context, p auth.Principal, query string, includeOrgWide bool, limit int32) ([]repo.Folder, error) {
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE accessible(id, depth) AS (
			SELECT f.id, 1 FROM folders f WHERE f.owner_type=$2 AND f.owner_id=$1
			UNION
			SELECT g.resource_id, 1 FROM resource_grants g
			WHERE g.resource_type='folder' AND g.revoked_at IS NULL
			  AND ((g.principal_type=$2 AND g.principal_id=$1) OR ($3::boolean AND g.principal_type='anyone'))
			UNION ALL
			SELECT c.id, a.depth+1 FROM folders c JOIN accessible a ON c.parent_folder_id = a.id WHERE a.depth < 20
		)
		SELECT DISTINCT f.id, f.title, f.owner_type, f.owner_id, COALESCE(f.parent_folder_id,''),
		       COALESCE(u.name,sa.name,''), COALESCE(u.email,'')
		FROM folders f
		JOIN accessible a ON f.id = a.id
		LEFT JOIN users u ON f.owner_type='user' AND f.owner_id=u.id
		LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND f.owner_id=sa.id
		WHERE f.title ILIKE '%' || $4 || '%'
		ORDER BY f.title
		LIMIT $5`, p.ID, string(p.Type), includeOrgWide, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []repo.Folder
	for rows.Next() {
		var f repo.Folder
		var pt string
		if err := rows.Scan(&f.ID, &f.Title, &pt, &f.Owner.ID, &f.ParentFolderID, &f.Owner.Name, &f.Owner.Email); err != nil {
			return nil, err
		}
		f.Owner.Type = auth.PrincipalType(pt)
		out = append(out, f)
	}
	return out, nil
}
func (s *Store) FolderHasGrants(ctx context.Context, folderID string) (bool, error) {
	var shared bool
	err := s.db.QueryRow(ctx, `
		WITH RECURSIVE anc(fid, depth) AS (
			SELECT $1::text, 1
			UNION ALL
			SELECT f.parent_folder_id, a.depth+1 FROM folders f JOIN anc a ON f.id=a.fid WHERE f.parent_folder_id IS NOT NULL AND a.depth < 20
		)
		SELECT EXISTS(
			SELECT 1 FROM resource_grants g
			JOIN anc ON g.resource_id=anc.fid
			WHERE g.resource_type='folder' AND g.revoked_at IS NULL
		)`, folderID).Scan(&shared)
	return shared, err
}
func (s *Store) MoveDocumentToFolder(ctx context.Context, _ auth.Principal, folderID, docID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE documents SET parent_folder_id=$1, updated_at=now() WHERE id=$2`, folderID, docID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, repo.ErrNotFound
	}
	hasGrants, err := s.FolderHasGrants(ctx, folderID)
	return hasGrants, err
}
func (s *Store) MoveDocumentToRoot(ctx context.Context, folderID, docID string) error {
	_, err := s.db.Exec(ctx, `UPDATE documents SET parent_folder_id=NULL, updated_at=now() WHERE id=$1 AND parent_folder_id=$2`, docID, folderID)
	return err
}
func (s *Store) ListFolderDocuments(ctx context.Context, p auth.Principal, folderID string) ([]repo.Document, error) {
	rows, err := s.db.Query(ctx, `
		SELECT d.id,d.title,d.owner_type,d.owner_id,COALESCE(d.current_version_id,''),COALESCE(d.parent_folder_id,''),
		       COALESCE(u.name,sa.name,''), COALESCE(u.email,'')
		FROM documents d
		LEFT JOIN users u ON d.owner_type='user' AND d.owner_id=u.id
		LEFT JOIN service_accounts sa ON d.owner_type='service_account' AND d.owner_id=sa.id
		WHERE d.parent_folder_id=$1`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repo.Document{}
	for rows.Next() {
		var d repo.Document
		var pt string
		if err := rows.Scan(&d.ID, &d.Title, &pt, &d.Owner.ID, &d.CurrentVersionID, &d.ParentFolderID, &d.Owner.Name, &d.Owner.Email); err != nil {
			return nil, err
		}
		d.Owner.Type = auth.PrincipalType(pt)
		out = append(out, d)
	}
	return out, nil
}
func (s *Store) ListEffectiveAccess(ctx context.Context, rt, rid string) ([]repo.EffectiveAccessItem, error) {
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE ancestors(rid, rtype, depth) AS (
			SELECT $2::text, $1::text, 0
			UNION ALL
			SELECT CASE WHEN a.rtype='document' THEN (SELECT parent_folder_id FROM documents WHERE id=a.rid)
			            ELSE (SELECT parent_folder_id FROM folders WHERE id=a.rid) END,
			       'folder', a.depth+1
			FROM ancestors a WHERE a.depth < 64
		)
		SELECT a.rid, a.rtype,
		       COALESCE(f.title, d.title, '') AS title,
		       COALESCE(f.owner_type, d.owner_type, '') AS owner_type,
		       COALESCE(f.owner_id, d.owner_id, '') AS owner_id
		FROM ancestors a
		LEFT JOIN folders f ON a.rtype='folder' AND f.id=a.rid
		LEFT JOIN documents d ON a.rtype='document' AND d.id=a.rid
		WHERE a.rid IS NOT NULL`, rt, rid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type ancestor struct {
		id, rtype, title, ownerType, ownerID string
	}
	var chain []ancestor
	for rows.Next() {
		var a ancestor
		if err := rows.Scan(&a.id, &a.rtype, &a.title, &a.ownerType, &a.ownerID); err != nil {
			return nil, err
		}
		chain = append(chain, a)
	}
	if len(chain) == 0 {
		return []repo.EffectiveAccessItem{}, nil
	}

	by := map[string]*repo.EffectiveAccessItem{}
	add := func(p auth.Principal, src repo.AccessSource) {
		k := string(p.Type) + ":" + p.ID
		if by[k] == nil {
			by[k] = &repo.EffectiveAccessItem{Principal: p}
		}
		by[k].EffectiveRole = repo.MaxRole(by[k].EffectiveRole, src.Role)
		by[k].Sources = append(by[k].Sources, src)
	}

	root := chain[0]
	ownerP := auth.Principal{Type: auth.PrincipalType(root.ownerType), ID: root.ownerID}
	add(ownerP, repo.AccessSource{ResourceType: root.rtype, ResourceID: root.id, ResourceTitle: root.title, Role: repo.RoleOwner})

	ids := make([]string, 0, len(chain))
	titles := map[string]string{}
	rtypes := map[string]string{}
	for _, a := range chain {
		ids = append(ids, a.id)
		titles[a.id] = a.title
		rtypes[a.id] = a.rtype
	}

	grantRows, err := s.db.Query(ctx, `
		SELECT g.id, g.resource_id, g.resource_type, g.principal_type, g.principal_id,
		       COALESCE(u.email,''), COALESCE(u.name,sa.name,''), g.role
		FROM resource_grants g
		LEFT JOIN users u ON g.principal_type='user' AND g.principal_id=u.id
		LEFT JOIN service_accounts sa ON g.principal_type='service_account' AND g.principal_id=sa.id
		WHERE g.revoked_at IS NULL AND g.resource_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer grantRows.Close()
	for grantRows.Next() {
		var grantID, resID, resType, pt, pid, email, name, role string
		if err := grantRows.Scan(&grantID, &resID, &resType, &pt, &pid, &email, &name, &role); err != nil {
			return nil, err
		}
		p := auth.Principal{Type: auth.PrincipalType(pt), ID: pid, Email: email, Name: name}
		add(p, repo.AccessSource{ResourceType: rtypes[resID], ResourceID: resID, ResourceTitle: titles[resID], Role: repo.Role(role), GrantID: grantID})
	}

	out := make([]repo.EffectiveAccessItem, 0, len(by))
	for _, v := range by {
		out = append(out, *v)
	}
	return out, nil
}
func (s *Store) ListPins(ctx context.Context, p auth.Principal) ([]repo.Pin, error) {
	rows, err := s.db.Query(ctx, `SELECT resource_type,resource_id FROM pins WHERE principal_type=$1 AND principal_id=$2`, string(p.Type), p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repo.Pin{}
	for rows.Next() {
		var pin repo.Pin
		rows.Scan(&pin.ResourceType, &pin.ResourceID)
		out = append(out, pin)
	}
	return out, nil
}
func (s *Store) PutPin(ctx context.Context, p auth.Principal, rt, rid string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO pins(principal_type,principal_id,resource_type,resource_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, string(p.Type), p.ID, rt, rid)
	return err
}
func (s *Store) DeletePin(ctx context.Context, p auth.Principal, rt, rid string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM pins WHERE principal_type=$1 AND principal_id=$2 AND resource_type=$3 AND resource_id=$4`, string(p.Type), p.ID, rt, rid)
	return err
}
