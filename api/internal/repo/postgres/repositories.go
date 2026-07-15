package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/razorpay/aidocs/api/internal/auth"
	dbsqlc "github.com/razorpay/aidocs/api/internal/db/sqlc"
	"github.com/razorpay/aidocs/api/internal/repo"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func treeKeyForFolderWith(ctx context.Context, q queryer, folderID string) (string, error) {
	var rootID string
	err := q.QueryRow(ctx, `
		WITH RECURSIVE ancestors(id,parent_folder_id,depth) AS (
			SELECT id,parent_folder_id,0 FROM folders WHERE id=$1
			UNION ALL
			SELECT f.id,f.parent_folder_id,a.depth+1
			FROM folders f JOIN ancestors a ON f.id=a.parent_folder_id
			WHERE a.parent_folder_id IS NOT NULL AND a.depth < 64
		)
		SELECT id FROM ancestors ORDER BY depth DESC LIMIT 1`, folderID).Scan(&rootID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", repo.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return "folder:" + rootID, nil
}

func treeKeyForDocumentWith(ctx context.Context, q queryer, documentID string) (string, error) {
	var parentID pgtype.Text
	if err := q.QueryRow(ctx, `SELECT parent_folder_id FROM documents WHERE id=$1`, documentID).Scan(&parentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", repo.ErrNotFound
		}
		return "", err
	}
	if !parentID.Valid {
		return "document:" + documentID, nil
	}
	return treeKeyForFolderWith(ctx, q, parentID.String)
}

func lockTreeKeys(ctx context.Context, tx pgx.Tx, keys ...string) error {
	unique := map[string]bool{}
	for _, key := range keys {
		if key != "" {
			unique[key] = true
		}
	}
	ordered := make([]string, 0, len(unique))
	for key := range unique {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 701239))`, key); err != nil {
			return err
		}
	}
	return nil
}

// lockPullRequestTree is the first lock acquired by every pull-request
// operation that can replace or publish a patchset. Keeping the tree advisory
// lock ahead of pull-request and document row locks prevents patchset,
// rebase, and merge transactions from waiting on each other in opposite order.
func lockPullRequestTree(ctx context.Context, tx pgx.Tx, repositoryID string) (string, error) {
	var rootFolderID string
	if err := tx.QueryRow(ctx, `SELECT root_folder_id FROM repositories WHERE id=$1`, repositoryID).Scan(&rootFolderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", repo.ErrNotFound
		}
		return "", err
	}
	if err := lockTreeKeys(ctx, tx, "folder:"+rootFolderID); err != nil {
		return "", err
	}
	return rootFolderID, nil
}

func verifyFolderTreeKey(ctx context.Context, q queryer, folderID, expected string) error {
	actual, err := treeKeyForFolderWith(ctx, q, folderID)
	if err != nil {
		return err
	}
	if actual != expected {
		return repo.ErrRepositoryBoundary
	}
	return nil
}

func verifyDocumentTreeKey(ctx context.Context, q queryer, documentID, expected string) error {
	actual, err := treeKeyForDocumentWith(ctx, q, documentID)
	if err != nil {
		return err
	}
	if actual != expected {
		return repo.ErrRepositoryBoundary
	}
	return nil
}

func lockRepositoryProtection(ctx context.Context, tx pgx.Tx, repositoryIDs ...string) (map[string]bool, error) {
	unique := map[string]bool{}
	for _, id := range repositoryIDs {
		if id != "" {
			unique[id] = true
		}
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	protected := make(map[string]bool, len(ids))
	for _, id := range ids {
		var value bool
		if err := tx.QueryRow(ctx, `SELECT protected FROM repositories WHERE id=$1 FOR UPDATE`, id).Scan(&value); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, repo.ErrNotFound
			}
			return nil, err
		}
		protected[id] = value
	}
	return protected, nil
}

func repositoryRoleWith(ctx context.Context, q queryer, repositoryID string, principal auth.Principal) (repo.Role, error) {
	var rootFolderID, ownerType, ownerID, scope string
	err := q.QueryRow(ctx, `
		SELECT r.root_folder_id,f.owner_type,f.owner_id,r.scope
		FROM repositories r JOIN folders f ON f.id=r.root_folder_id
		WHERE r.id=$1`, repositoryID).Scan(&rootFolderID, &ownerType, &ownerID, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.RoleNone, repo.ErrNotFound
	}
	if err != nil {
		return repo.RoleNone, err
	}
	best := repo.RoleNone
	if scope == repo.RepositoryScopeOrganization && principal.Type != auth.PrincipalAnonymous && principal.ID != "" {
		best = repo.RoleViewer
	}
	if ownerType == string(principal.Type) && ownerID == principal.ID {
		best = repo.RoleOwner
	}
	rows, err := q.Query(ctx, `
		SELECT role FROM resource_grants
		WHERE resource_type='folder' AND resource_id=$1 AND revoked_at IS NULL
		  AND ((principal_type=$2 AND principal_id=$3) OR principal_type='anyone')`,
		rootFolderID, string(principal.Type), principal.ID)
	if err != nil {
		return repo.RoleNone, err
	}
	defer rows.Close()
	for rows.Next() {
		var role repo.Role
		if err := rows.Scan(&role); err != nil {
			return repo.RoleNone, err
		}
		best = repo.MaxRole(best, role)
	}
	if err := rows.Err(); err != nil {
		return repo.RoleNone, err
	}
	if best == repo.RoleNone {
		return repo.RoleNone, repo.ErrNotFound
	}
	return best, nil
}

func reviewRoleEligible(role repo.Role) bool {
	return role == repo.RoleEditor || role == repo.RoleOwner
}

func eligibleReviews(ctx context.Context, q queryer, pullRequest repo.PullRequest) ([]repo.PullRequestReview, error) {
	eligible := make([]repo.PullRequestReview, 0, len(pullRequest.Reviews))
	for _, review := range pullRequest.Reviews {
		role, err := repositoryRoleWith(ctx, q, pullRequest.RepositoryID, review.Reviewer)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			return nil, err
		}
		if reviewRoleEligible(role) {
			eligible = append(eligible, review)
		}
	}
	return eligible, nil
}

const repositorySelect = `
	SELECT r.id, r.name, r.slug, r.description, r.scope, r.namespace_id,
	       r.namespace_name, r.root_folder_id, r.created_by_type,
	       r.created_by_id, r.created_by_email, r.created_by_name,
	       r.required_approvals, r.protected, r.created_at, r.updated_at
	FROM repositories r`

func scanRepository(row rowScanner) (repo.RepositoryRecord, error) {
	var repository repo.RepositoryRecord
	var creatorType string
	var createdAt, updatedAt time.Time
	err := row.Scan(
		&repository.ID,
		&repository.Name,
		&repository.Slug,
		&repository.Description,
		&repository.Scope,
		&repository.NamespaceID,
		&repository.NamespaceName,
		&repository.RootFolderID,
		&creatorType,
		&repository.CreatedBy.ID,
		&repository.CreatedBy.Email,
		&repository.CreatedBy.Name,
		&repository.RequiredApprovals,
		&repository.Protected,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return repo.RepositoryRecord{}, err
	}
	repository.CreatedBy.Type = auth.PrincipalType(creatorType)
	repository.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	repository.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return repository, nil
}

func repositoryWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return repo.ErrConflict
		case "23503":
			return repo.ErrNotFound
		}
	}
	return err
}

//nolint:gocyclo // Wrapping folders/documents and creating the boundary must remain one lock-ordered transaction.
func (s *Store) CreateRepository(ctx context.Context, creator auth.Principal, params repo.CreateRepositoryParams) (repo.RepositoryRecord, error) {
	if params.Name == "" || params.Slug == "" || (params.Scope != repo.RepositoryScopePersonal && params.Scope != repo.RepositoryScopeOrganization) || (params.RootFolderID != "" && params.DocumentID != "") {
		return repo.RepositoryRecord{}, repo.ErrConflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.RepositoryRecord{}, err
	}
	defer rollback(ctx, tx)
	q := s.q.WithTx(tx)
	if creator.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, creator); err != nil {
			return repo.RepositoryRecord{}, err
		}
	}

	rootID := params.RootFolderID
	if rootID != "" {
		if err := lockTreeKeys(ctx, tx, "folder:"+rootID); err != nil {
			return repo.RepositoryRecord{}, err
		}
		var ownerType, ownerID string
		var parentID pgtype.Text
		err := tx.QueryRow(ctx, `SELECT owner_type, owner_id, parent_folder_id FROM folders WHERE id=$1 FOR UPDATE`, rootID).Scan(&ownerType, &ownerID, &parentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.RepositoryRecord{}, repo.ErrNotFound
		}
		if err != nil {
			return repo.RepositoryRecord{}, err
		}
		if parentID.Valid || ownerType != string(creator.Type) || ownerID != creator.ID {
			return repo.RepositoryRecord{}, repo.ErrForbidden
		}
		var wrapped bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE root_folder_id=$1)`, rootID).Scan(&wrapped); err != nil {
			return repo.RepositoryRecord{}, err
		}
		if wrapped {
			return repo.RepositoryRecord{}, repo.ErrConflict
		}
		var mixedOwnership bool
		err = tx.QueryRow(ctx, `
			WITH RECURSIVE descendants(id,depth) AS (
				SELECT $1::text,0
				UNION ALL
				SELECT f.id,d.depth+1 FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
				WHERE d.depth < 64
			)
			SELECT EXISTS(
				SELECT 1 FROM folders f
				WHERE f.id IN (SELECT id FROM descendants)
				  AND (f.owner_type<>$2 OR f.owner_id<>$3)
				UNION ALL
				SELECT 1 FROM documents d
				WHERE d.parent_folder_id IN (SELECT id FROM descendants)
				  AND (d.owner_type<>$2 OR d.owner_id<>$3)
			)`, rootID, string(creator.Type), creator.ID).Scan(&mixedOwnership)
		if err != nil {
			return repo.RepositoryRecord{}, err
		}
		if mixedOwnership {
			return repo.RepositoryRecord{}, repo.ErrForbidden
		}
	} else {
		if params.DocumentID != "" {
			if err := lockTreeKeys(ctx, tx, "document:"+params.DocumentID); err != nil {
				return repo.RepositoryRecord{}, err
			}
			var ownerType, ownerID string
			var parentID pgtype.Text
			err = tx.QueryRow(ctx, `SELECT owner_type, owner_id, parent_folder_id FROM documents WHERE id=$1 FOR UPDATE`, params.DocumentID).Scan(&ownerType, &ownerID, &parentID)
			if errors.Is(err, pgx.ErrNoRows) {
				return repo.RepositoryRecord{}, repo.ErrNotFound
			}
			if err != nil {
				return repo.RepositoryRecord{}, err
			}
			if parentID.Valid || ownerType != string(creator.Type) || ownerID != creator.ID {
				return repo.RepositoryRecord{}, repo.ErrForbidden
			}
		}
		rootID = newID("fld")
		_, err := tx.Exec(ctx, `INSERT INTO folders(id,title,owner_type,owner_id) VALUES($1,$2,$3,$4)`, rootID, params.Name, string(creator.Type), creator.ID)
		if err != nil {
			return repo.RepositoryRecord{}, repositoryWriteError(err)
		}
		if params.DocumentID != "" {
			if _, err := tx.Exec(ctx, `UPDATE documents SET parent_folder_id=$1, updated_at=now() WHERE id=$2`, rootID, params.DocumentID); err != nil {
				return repo.RepositoryRecord{}, err
			}
		}
	}

	repositoryID := newID("repo")
	_, err = tx.Exec(ctx, `
		INSERT INTO repositories(
			id,name,slug,description,scope,namespace_id,namespace_name,root_folder_id,
			created_by_type,created_by_id,created_by_email,created_by_name,
			required_approvals,protected
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		repositoryID, params.Name, params.Slug, params.Description, params.Scope,
		params.NamespaceID, params.NamespaceName, rootID, string(creator.Type), creator.ID,
		creator.Email, creator.Name, params.RequiredApprovals, params.Protected)
	if err != nil {
		return repo.RepositoryRecord{}, repositoryWriteError(err)
	}
	if params.GrantOrgViewer {
		_, err = tx.Exec(ctx, `
			INSERT INTO resource_grants(
				id,resource_type,resource_id,principal_type,principal_id,role,
				granted_by_type,granted_by_user_id
			) VALUES($1,'folder',$2,'anyone','','viewer',$3,$4)
			ON CONFLICT(resource_type,resource_id,principal_type,principal_id)
			DO UPDATE SET role=CASE
			                         WHEN resource_grants.role IN ('commenter','editor','owner') THEN resource_grants.role
			                         ELSE 'viewer'
			                       END,
			              revoked_at=NULL,
			              granted_by_type=EXCLUDED.granted_by_type,
			              granted_by_user_id=EXCLUDED.granted_by_user_id`,
			newID("gr"), rootID, string(creator.Type), creator.ID)
		if err != nil {
			return repo.RepositoryRecord{}, err
		}
	}

	repository, err := scanRepository(tx.QueryRow(ctx, repositorySelect+` WHERE r.id=$1`, repositoryID))
	if err != nil {
		return repo.RepositoryRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.RepositoryRecord{}, err
	}
	return repository, nil
}

func (s *Store) ListRepositories(ctx context.Context, principal auth.Principal) ([]repo.RepositoryRecord, error) {
	rows, err := s.db.Query(ctx, repositorySelect+`
		JOIN folders root ON root.id=r.root_folder_id
		WHERE (root.owner_type=$1 AND root.owner_id=$2)
		   OR (r.scope='organization' AND $1<>'anonymous' AND $2<>'')
		   OR EXISTS (
		     SELECT 1 FROM resource_grants g
		     WHERE g.resource_type='folder' AND g.resource_id=r.root_folder_id
		       AND g.revoked_at IS NULL
		       AND ((g.principal_type=$1 AND g.principal_id=$2) OR g.principal_type='anyone')
		   )
		ORDER BY lower(r.namespace_name), lower(r.name)`, string(principal.Type), principal.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repo.RepositoryRecord{}
	for rows.Next() {
		repository, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, repository)
	}
	return out, rows.Err()
}

func (s *Store) GetRepository(ctx context.Context, id string) (repo.RepositoryRecord, error) {
	repository, err := scanRepository(s.db.QueryRow(ctx, repositorySelect+` WHERE r.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.RepositoryRecord{}, repo.ErrNotFound
	}
	return repository, err
}

//nolint:gocyclo // Boundary deletion and dependent blob discovery are intentionally coordinated in one transaction.
func (s *Store) DeleteRepository(ctx context.Context, id string, principal auth.Principal) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	var rootFolderID, creatorType, creatorID string
	err = tx.QueryRow(ctx, `
		SELECT root_folder_id,created_by_type,created_by_id
		FROM repositories
		WHERE id=$1`, id).Scan(&rootFolderID, &creatorType, &creatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.ErrNotFound
	}
	if err != nil {
		return err
	}
	if creatorType != string(principal.Type) || creatorID != principal.ID {
		return repo.ErrForbidden
	}

	// Repository mutations acquire the tree lock before locking repository rows.
	// Keep the same order here so deletion cannot deadlock with a concurrent edit.
	if err := lockTreeKeys(ctx, tx, "folder:"+rootFolderID); err != nil {
		return err
	}
	var lockedRootFolderID, lockedCreatorType, lockedCreatorID string
	err = tx.QueryRow(ctx, `
		SELECT root_folder_id,created_by_type,created_by_id
		FROM repositories
		WHERE id=$1
		FOR UPDATE`, id).Scan(&lockedRootFolderID, &lockedCreatorType, &lockedCreatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.ErrNotFound
	}
	if err != nil {
		return err
	}
	if lockedRootFolderID != rootFolderID {
		return repo.ErrRepositoryBoundary
	}
	if lockedCreatorType != string(principal.Type) || lockedCreatorID != principal.ID {
		return repo.ErrForbidden
	}

	rows, err := tx.Query(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1
			FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		)
		SELECT v.html_blob_key
		FROM versions v
		JOIN documents doc ON doc.id=v.document_id
		WHERE doc.parent_folder_id IN (SELECT id FROM descendants)`, rootFolderID)
	if err != nil {
		return err
	}
	var blobKeys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		blobKeys = append(blobKeys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = tx.Query(ctx, `
		SELECT DISTINCT prc.head_html_blob_key
		FROM pull_request_changes prc
		JOIN pull_requests pr ON pr.id=prc.pull_request_id
		WHERE pr.repository_id=$1 AND prc.head_html_blob_key IS NOT NULL`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		blobKeys = append(blobKeys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// Pull requests and reviews cascade from the repository. They must be gone
	// before their version references can be removed with the documents.
	if tag, err := tx.Exec(ctx, `DELETE FROM repositories WHERE id=$1`, id); err != nil {
		return err
	} else if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1
			FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		), repository_documents(id) AS (
			SELECT doc.id FROM documents doc
			WHERE doc.parent_folder_id IN (SELECT id FROM descendants)
		)
		DELETE FROM resource_grants grant_row
		WHERE (grant_row.resource_type='folder' AND grant_row.resource_id IN (SELECT id FROM descendants))
		   OR (grant_row.resource_type='document' AND grant_row.resource_id IN (SELECT id FROM repository_documents))`, rootFolderID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1
			FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		), repository_documents(id) AS (
			SELECT doc.id FROM documents doc
			WHERE doc.parent_folder_id IN (SELECT id FROM descendants)
		)
		DELETE FROM pins pin
		WHERE (pin.resource_type='folder' AND pin.resource_id IN (SELECT id FROM descendants))
		   OR (pin.resource_type='document' AND pin.resource_id IN (SELECT id FROM repository_documents))`, rootFolderID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1
			FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		)
		DELETE FROM documents doc
		WHERE doc.parent_folder_id IN (SELECT id FROM descendants)`, rootFolderID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1
			FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		)
		DELETE FROM folders folder
		WHERE folder.id IN (SELECT id FROM descendants)`, rootFolderID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	for _, key := range blobKeys {
		if err := s.blobs.Delete(context.WithoutCancel(ctx), key); err != nil {
			// The database deletion has committed, so returning an error here would
			// invite a retry that can only produce not_found. Keep the API result
			// truthful and surface the storage cleanup failure operationally.
			log.Printf("repository deleted but blob cleanup failed repository_id=%q blob_key=%q error=%v", id, key, err)
		}
	}
	return nil
}

func transferNamespaceName(principal auth.Principal) string {
	if strings.TrimSpace(principal.Name) != "" {
		return strings.TrimSpace(principal.Name)
	}
	if strings.TrimSpace(principal.Email) != "" {
		return strings.TrimSpace(principal.Email)
	}
	return principal.ID
}

//nolint:gocyclo // Namespace checks and recursive ownership updates must commit atomically under the repository lock.
func (s *Store) TransferRepositoryOwnership(ctx context.Context, id string, from, to auth.Principal) (repo.RepositoryRecord, error) {
	if to.ID == "" || to.Type == auth.PrincipalAnonymous || to.Type == auth.PrincipalAnyone {
		return repo.RepositoryRecord{}, repo.ErrConflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.RepositoryRecord{}, err
	}
	defer rollback(ctx, tx)

	var rootFolderID string
	if err := tx.QueryRow(ctx, `SELECT root_folder_id FROM repositories WHERE id=$1`, id).Scan(&rootFolderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.RepositoryRecord{}, repo.ErrNotFound
		}
		return repo.RepositoryRecord{}, err
	}
	// Serialize with every mutation of the repository tree before locking the
	// repository row, matching the deletion and tree-move lock order.
	if err := lockTreeKeys(ctx, tx, "folder:"+rootFolderID); err != nil {
		return repo.RepositoryRecord{}, err
	}
	repository, err := scanRepository(tx.QueryRow(ctx, repositorySelect+` WHERE r.id=$1 FOR UPDATE OF r`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.RepositoryRecord{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.RepositoryRecord{}, err
	}
	if repository.CreatedBy.Type != from.Type || repository.CreatedBy.ID != from.ID {
		return repo.RepositoryRecord{}, repo.ErrForbidden
	}
	q := s.q.WithTx(tx)
	if to.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, to); err != nil {
			return repo.RepositoryRecord{}, err
		}
	}
	if repository.Scope == repo.RepositoryScopePersonal {
		var conflict bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM repositories
				WHERE id<>$1 AND scope='personal' AND namespace_id=$2 AND lower(slug)=lower($3)
			)`, id, to.ID, repository.Slug).Scan(&conflict); err != nil {
			return repo.RepositoryRecord{}, err
		}
		if conflict {
			return repo.RepositoryRecord{}, repo.ErrConflict
		}
	}

	namespaceID, namespaceName := repository.NamespaceID, repository.NamespaceName
	if repository.Scope == repo.RepositoryScopePersonal {
		namespaceID, namespaceName = to.ID, transferNamespaceName(to)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE repositories
		SET created_by_type=$1,created_by_id=$2,created_by_email=$3,created_by_name=$4,
		    namespace_id=$5,namespace_name=$6,updated_at=now()
		WHERE id=$7`, string(to.Type), to.ID, to.Email, to.Name, namespaceID, namespaceName, id); err != nil {
		return repo.RepositoryRecord{}, repositoryWriteError(err)
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1 FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		)
		UPDATE folders SET owner_type=$2,owner_id=$3
		WHERE id IN (SELECT id FROM descendants)`, rootFolderID, string(to.Type), to.ID); err != nil {
		return repo.RepositoryRecord{}, repositoryWriteError(err)
	}
	if _, err := tx.Exec(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1 FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		)
		UPDATE documents SET owner_type=$2,owner_id=$3,updated_at=now()
		WHERE parent_folder_id IN (SELECT id FROM descendants)`, rootFolderID, string(to.Type), to.ID); err != nil {
		return repo.RepositoryRecord{}, repositoryWriteError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.RepositoryRecord{}, repositoryWriteError(err)
	}
	return s.GetRepository(ctx, id)
}

func (s *Store) RoleForRepository(ctx context.Context, principal auth.Principal, repositoryID string) (repo.Role, error) {
	return repositoryRoleWith(ctx, s.db, repositoryID, principal)
}

func (s *Store) RepositoryForFolder(ctx context.Context, folderID string) (repo.RepositoryRecord, error) {
	return repositoryForFolderWith(ctx, s.db, folderID)
}

func repositoryForFolderWith(ctx context.Context, q queryer, folderID string) (repo.RepositoryRecord, error) {
	row := q.QueryRow(ctx, `
		WITH RECURSIVE ancestors(id,parent_folder_id,depth) AS (
			SELECT id,parent_folder_id,0 FROM folders WHERE id=$1
			UNION ALL
			SELECT f.id,f.parent_folder_id,a.depth+1
			FROM folders f JOIN ancestors a ON f.id=a.parent_folder_id
			WHERE a.parent_folder_id IS NOT NULL AND a.depth < 64
		)
		`+repositorySelect+`
		JOIN ancestors a ON a.id=r.root_folder_id
		ORDER BY a.depth
		LIMIT 1`, folderID)
	repository, err := scanRepository(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.RepositoryRecord{}, repo.ErrNotFound
	}
	return repository, err
}

func (s *Store) RepositoryForDocument(ctx context.Context, documentID string) (repo.RepositoryRecord, error) {
	return repositoryForDocumentWith(ctx, s.db, documentID)
}

func repositoryForDocumentWith(ctx context.Context, q queryer, documentID string) (repo.RepositoryRecord, error) {
	row := q.QueryRow(ctx, `
		WITH RECURSIVE ancestors(id,parent_folder_id,depth) AS (
			SELECT f.id,f.parent_folder_id,0
			FROM documents d JOIN folders f ON f.id=d.parent_folder_id
			WHERE d.id=$1
			UNION ALL
			SELECT f.id,f.parent_folder_id,a.depth+1
			FROM folders f JOIN ancestors a ON f.id=a.parent_folder_id
			WHERE a.parent_folder_id IS NOT NULL AND a.depth < 64
		)
		`+repositorySelect+`
		JOIN ancestors a ON a.id=r.root_folder_id
		ORDER BY a.depth
		LIMIT 1`, documentID)
	repository, err := scanRepository(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.RepositoryRecord{}, repo.ErrNotFound
	}
	return repository, err
}

func (s *Store) ListRepositoryDocuments(ctx context.Context, principal auth.Principal, repositoryID string) ([]repo.Document, error) {
	repository, err := s.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	role, err := repositoryRoleWith(ctx, s.db, repositoryID, principal)
	if err != nil || role == repo.RoleNone {
		return nil, repo.ErrNotFound
	}
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1 FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		)
		SELECT d.id,d.title,d.owner_type,d.owner_id,COALESCE(d.current_version_id,''),
		       COALESCE(d.parent_folder_id,''),COALESCE(u.name,sa.name,''),
		       COALESCE(u.email,''),d.updated_at
		FROM documents d
		LEFT JOIN users u ON d.owner_type='user' AND d.owner_id=u.id
		LEFT JOIN service_accounts sa ON d.owner_type='service_account' AND d.owner_id=sa.id
		WHERE d.parent_folder_id IN (SELECT id FROM descendants)
		ORDER BY lower(d.title)`, repository.RootFolderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repo.Document{}
	for rows.Next() {
		var document repo.Document
		var ownerType string
		var updatedAt time.Time
		if err := rows.Scan(&document.ID, &document.Title, &ownerType, &document.Owner.ID,
			&document.CurrentVersionID, &document.ParentFolderID, &document.Owner.Name,
			&document.Owner.Email, &updatedAt); err != nil {
			return nil, err
		}
		document.Owner.Type = auth.PrincipalType(ownerType)
		document.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		out = append(out, document)
	}
	return out, rows.Err()
}

//nolint:gocyclo // First-document bootstrap combines protection checks, blob compensation, and two persisted records.
func (s *Store) CreateRepositoryDocument(ctx context.Context, repositoryID string, creator auth.Principal, title string, html []byte) (repo.Document, repo.Version, error) {
	documentID, versionID := newID("doc"), newID("ver")
	blobKey := fmt.Sprintf("docs/%s/%s.html", documentID, versionID)
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

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	defer rollback(ctx, tx)
	q := s.q.WithTx(tx)
	if creator.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, creator); err != nil {
			return repo.Document{}, repo.Version{}, err
		}
	}
	var rootFolderID string
	if err := tx.QueryRow(ctx, `SELECT root_folder_id FROM repositories WHERE id=$1 FOR UPDATE`, repositoryID).Scan(&rootFolderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Document{}, repo.Version{}, repo.ErrNotFound
		}
		return repo.Document{}, repo.Version{}, err
	}
	role, err := repositoryRoleWith(ctx, tx, repositoryID, creator)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return repo.Document{}, repo.Version{}, err
	}
	if role != repo.RoleEditor && role != repo.RoleOwner {
		return repo.Document{}, repo.Version{}, repo.ErrForbidden
	}
	var repositoryNotEmpty bool
	err = tx.QueryRow(ctx, `
		WITH RECURSIVE descendants(id,depth) AS (
			SELECT $1::text,0
			UNION ALL
			SELECT f.id,d.depth+1 FROM folders f JOIN descendants d ON f.parent_folder_id=d.id
			WHERE d.depth < 64
		)
		SELECT EXISTS(SELECT 1 FROM documents WHERE parent_folder_id IN (SELECT id FROM descendants))`, rootFolderID).Scan(&repositoryNotEmpty)
	if err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	if repositoryNotEmpty {
		return repo.Document{}, repo.Version{}, repo.ErrRepositoryNotEmpty
	}
	if _, err := tx.Exec(ctx, `INSERT INTO documents(id,title,owner_type,owner_id,parent_folder_id) VALUES($1,$2,$3,$4,$5)`, documentID, title, string(creator.Type), creator.ID, rootFolderID); err != nil {
		return repo.Document{}, repo.Version{}, repositoryWriteError(err)
	}
	if err := q.InsertInitialVersion(ctx, dbsqlc.InsertInitialVersionParams{ID: versionID, DocumentID: documentID, HtmlBlobKey: blobKey, Sha256: sum, CreatedByType: string(creator.Type), CreatedByID: creator.ID}); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	if err := q.UpdateDocumentCurrentVersion(ctx, dbsqlc.UpdateDocumentCurrentVersionParams{CurrentVersionID: pgtype.Text{String: versionID, Valid: true}, ID: documentID}); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	committed = true
	document, err := s.GetDocument(ctx, documentID)
	if err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	version, err := s.GetVersion(ctx, versionID)
	if err != nil {
		return repo.Document{}, repo.Version{}, err
	}
	return document, version, nil
}

const pullRequestSelect = `
	SELECT pr.id,pr.repository_id,pr.number,COALESCE(pr.document_id,''),COALESCE(pr.base_version_id,''),
	       COALESCE(pr.head_version_id,''),pr.patchset,pr.title,pr.body,pr.change_summary,pr.status,
	       pr.author_type,pr.author_id,pr.author_email,pr.author_name,
	       COALESCE(pr.merged_by_type,''),COALESCE(pr.merged_by_id,''),
	       COALESCE(pr.merged_by_email,''),COALESCE(pr.merged_by_name,''),
	       pr.created_at,pr.updated_at,pr.merged_at,r.required_approvals
	FROM pull_requests pr
	JOIN repositories r ON r.id=pr.repository_id`

func scanPullRequest(row rowScanner) (repo.PullRequest, error) {
	var pullRequest repo.PullRequest
	var authorType, mergedByType, mergedByID, mergedByEmail, mergedByName string
	var createdAt, updatedAt time.Time
	var mergedAt pgtype.Timestamptz
	err := row.Scan(
		&pullRequest.ID,
		&pullRequest.RepositoryID,
		&pullRequest.Number,
		&pullRequest.DocumentID,
		&pullRequest.BaseVersionID,
		&pullRequest.HeadVersionID,
		&pullRequest.Patchset,
		&pullRequest.Title,
		&pullRequest.Body,
		&pullRequest.ChangeSummary,
		&pullRequest.Status,
		&authorType,
		&pullRequest.Author.ID,
		&pullRequest.Author.Email,
		&pullRequest.Author.Name,
		&mergedByType,
		&mergedByID,
		&mergedByEmail,
		&mergedByName,
		&createdAt,
		&updatedAt,
		&mergedAt,
		&pullRequest.RequiredApprovals,
	)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest.Author.Type = auth.PrincipalType(authorType)
	if mergedByID != "" {
		pullRequest.MergedBy = &auth.Principal{Type: auth.PrincipalType(mergedByType), ID: mergedByID, Email: mergedByEmail, Name: mergedByName}
	}
	pullRequest.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	pullRequest.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	if mergedAt.Valid {
		pullRequest.MergedAt = mergedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	return pullRequest, nil
}

func loadPullRequestReviews(ctx context.Context, q queryer, pullRequestID string) ([]repo.PullRequestReview, error) {
	rows, err := q.Query(ctx, `
		SELECT id,pull_request_id,COALESCE(head_version_id,''),patchset,reviewer_type,reviewer_id,
		       reviewer_email,reviewer_name,state,body,submitted_at
		FROM pull_request_reviews
		WHERE pull_request_id=$1
		ORDER BY submitted_at`, pullRequestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reviews := []repo.PullRequestReview{}
	for rows.Next() {
		var review repo.PullRequestReview
		var reviewerType string
		var submittedAt time.Time
		if err := rows.Scan(&review.ID, &review.PullRequestID, &review.HeadVersionID, &review.Patchset,
			&reviewerType, &review.Reviewer.ID, &review.Reviewer.Email, &review.Reviewer.Name,
			&review.State, &review.Body, &submittedAt); err != nil {
			return nil, err
		}
		review.Reviewer.Type = auth.PrincipalType(reviewerType)
		review.SubmittedAt = submittedAt.UTC().Format(time.RFC3339Nano)
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}

func loadPullRequestChanges(ctx context.Context, q queryer, pullRequest repo.PullRequest) ([]repo.PullRequestChange, error) {
	rows, err := q.Query(ctx, `
		SELECT id,pull_request_id,patchset,operation,COALESCE(document_id,''),
		       COALESCE(folder_id,''),COALESCE(base_version_id,''),COALESCE(head_version_id,''),
		       base_title,title,base_path,path
		FROM pull_request_changes
		WHERE pull_request_id=$1 AND patchset=$2
		ORDER BY position`, pullRequest.ID, pullRequest.Patchset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changes := []repo.PullRequestChange{}
	for rows.Next() {
		var change repo.PullRequestChange
		if err := rows.Scan(&change.ID, &change.PullRequestID, &change.Patchset, &change.Operation,
			&change.DocumentID, &change.FolderID, &change.BaseVersionID, &change.HeadVersionID,
			&change.BaseTitle, &change.Title, &change.BasePath, &change.Path); err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(changes) == 0 && pullRequest.DocumentID != "" {
		changes = append(changes, repo.PullRequestChange{
			PullRequestID: pullRequest.ID, Patchset: pullRequest.Patchset,
			Operation: repo.PullRequestChangeUpdate, DocumentID: pullRequest.DocumentID,
			BaseVersionID: pullRequest.BaseVersionID, HeadVersionID: pullRequest.HeadVersionID,
		})
	}
	return changes, nil
}

func pullRequestStaleWith(ctx context.Context, q queryer, pullRequest repo.PullRequest) (bool, error) {
	for _, change := range pullRequest.Changes {
		if change.Operation == repo.PullRequestChangeCreate {
			var documentExists, folderInRepository bool
			if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents WHERE id=$1)`, change.DocumentID).Scan(&documentExists); err != nil {
				return false, err
			}
			if err := q.QueryRow(ctx, `
				WITH RECURSIVE ancestors(id,parent_folder_id,depth) AS (
					SELECT id,parent_folder_id,0 FROM folders WHERE id=$1
					UNION ALL
					SELECT f.id,f.parent_folder_id,a.depth+1 FROM folders f JOIN ancestors a ON f.id=a.parent_folder_id
					WHERE a.parent_folder_id IS NOT NULL AND a.depth < 64
				)
				SELECT EXISTS(SELECT 1 FROM repositories r JOIN ancestors a ON a.id=r.root_folder_id WHERE r.id=$2)`, change.FolderID, pullRequest.RepositoryID).Scan(&folderInRepository); err != nil {
				return false, err
			}
			if documentExists || !folderInRepository {
				return true, nil
			}
			continue
		}
		var currentVersionID string
		err := q.QueryRow(ctx, `
			WITH RECURSIVE ancestors(id,parent_folder_id,depth) AS (
				SELECT f.id,f.parent_folder_id,0 FROM documents d JOIN folders f ON f.id=d.parent_folder_id WHERE d.id=$1
				UNION ALL
				SELECT f.id,f.parent_folder_id,a.depth+1 FROM folders f JOIN ancestors a ON f.id=a.parent_folder_id
				WHERE a.parent_folder_id IS NOT NULL AND a.depth < 64
			)
			SELECT COALESCE(d.current_version_id,'') FROM documents d
			WHERE d.id=$1 AND EXISTS(SELECT 1 FROM repositories r JOIN ancestors a ON a.id=r.root_folder_id WHERE r.id=$2)`, change.DocumentID, pullRequest.RepositoryID).Scan(&currentVersionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if currentVersionID != change.BaseVersionID {
			return true, nil
		}
		if change.Operation == repo.PullRequestChangeMove {
			target, err := repositoryForFolderWith(ctx, q, change.FolderID)
			if errors.Is(err, repo.ErrNotFound) || (err == nil && target.ID != pullRequest.RepositoryID) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

func computePullRequestMergeability(pullRequest repo.PullRequest, stale bool) repo.PullRequest {
	pullRequest.Mergeable = false
	pullRequest.MergeBlockedReason = ""
	if pullRequest.Status != repo.PullRequestStatusOpen {
		pullRequest.MergeBlockedReason = repo.ErrPullRequestClosed.Error()
		return pullRequest
	}
	if stale {
		pullRequest.MergeBlockedReason = repo.ErrStalePullRequest.Error()
		return pullRequest
	}
	approvals := 0
	for _, review := range pullRequest.Reviews {
		if review.Patchset != pullRequest.Patchset {
			continue
		}
		if review.State == repo.ReviewStateChangesRequested {
			pullRequest.MergeBlockedReason = repo.ErrChangesRequested.Error()
			return pullRequest
		}
		if review.State == repo.ReviewStateApproved && (review.Reviewer.Type != pullRequest.Author.Type || review.Reviewer.ID != pullRequest.Author.ID) {
			approvals++
		}
	}
	if approvals < pullRequest.RequiredApprovals {
		pullRequest.MergeBlockedReason = repo.ErrApprovalRequired.Error()
		return pullRequest
	}
	pullRequest.Mergeable = true
	return pullRequest
}

func decoratePullRequest(ctx context.Context, q queryer, pullRequest repo.PullRequest) (repo.PullRequest, error) {
	changes, err := loadPullRequestChanges(ctx, q, pullRequest)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest.Changes = changes
	reviews, err := loadPullRequestReviews(ctx, q, pullRequest.ID)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest.Reviews = reviews
	mergeReviews, err := eligibleReviews(ctx, q, pullRequest)
	if err != nil {
		return repo.PullRequest{}, err
	}
	stale, err := pullRequestStaleWith(ctx, q, pullRequest)
	if err != nil {
		return repo.PullRequest{}, err
	}
	checked := pullRequest
	checked.Reviews = mergeReviews
	checked = computePullRequestMergeability(checked, stale)
	checked.Reviews = reviews
	return checked, nil
}

type storedPullRequestChange struct {
	change   repo.PullRequestChange
	position int
	blobKey  string
	sha256   string
}

func postgresPullRequestOperationValid(operation string) bool {
	switch operation {
	case repo.PullRequestChangeUpdate, repo.PullRequestChangeCreate, repo.PullRequestChangeRename, repo.PullRequestChangeMove, repo.PullRequestChangeDelete:
		return true
	default:
		return false
	}
}

//nolint:gocyclo // The operation matrix is kept linear so every change is validated against the same locked tree snapshot.
func (s *Store) preparePullRequestChanges(ctx context.Context, tx pgx.Tx, pullRequestID string, patchset int, repositoryID, rootFolderID, changeSummary string, inputs []repo.PullRequestChangeInput, author auth.Principal) ([]storedPullRequestChange, []string, error) {
	if len(inputs) == 0 {
		return nil, nil, repo.ErrInvalidPullRequestChange
	}
	if err := lockTreeKeys(ctx, tx, "folder:"+rootFolderID); err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	prepared := make([]storedPullRequestChange, 0, len(inputs))
	uploaded := []string{}
	for index, input := range inputs {
		if !postgresPullRequestOperationValid(input.Operation) {
			return nil, uploaded, repo.ErrInvalidPullRequestChange
		}
		documentID, folderID := input.DocumentID, input.FolderID
		change := repo.PullRequestChange{
			ID:            fmt.Sprintf("%s_change_%d_%d", pullRequestID, patchset, index+1),
			PullRequestID: pullRequestID, Patchset: patchset, Operation: input.Operation,
			DocumentID: documentID, FolderID: folderID, BaseVersionID: input.BaseVersionID,
			Title: strings.TrimSpace(input.Title), Path: strings.TrimSpace(input.Path),
		}
		if input.Operation == repo.PullRequestChangeCreate {
			if change.Title == "" || input.HTML == nil {
				return nil, uploaded, repo.ErrInvalidPullRequestChange
			}
			if documentID == "" {
				documentID = newID("doc")
				change.DocumentID = documentID
			}
			if seen[documentID] {
				return nil, uploaded, repo.ErrInvalidPullRequestChange
			}
			seen[documentID] = true
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents WHERE id=$1)`, documentID).Scan(&exists); err != nil {
				return nil, uploaded, err
			}
			if exists {
				return nil, uploaded, repo.ErrConflict
			}
			if folderID == "" {
				folderID = rootFolderID
				change.FolderID = folderID
			}
			if err := verifyFolderTreeKey(ctx, tx, folderID, "folder:"+rootFolderID); err != nil {
				if errors.Is(err, repo.ErrNotFound) {
					return nil, uploaded, repo.ErrRepositoryBoundary
				}
				return nil, uploaded, err
			}
		} else {
			if documentID == "" || seen[documentID] {
				return nil, uploaded, repo.ErrInvalidPullRequestChange
			}
			seen[documentID] = true
			if err := verifyDocumentTreeKey(ctx, tx, documentID, "folder:"+rootFolderID); err != nil {
				if errors.Is(err, repo.ErrNotFound) {
					return nil, uploaded, repo.ErrNotFound
				}
				return nil, uploaded, err
			}
			var currentVersionID pgtype.Text
			if err := tx.QueryRow(ctx, `SELECT title,COALESCE(parent_folder_id,''),current_version_id FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&change.BaseTitle, &change.BasePath, &currentVersionID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, uploaded, repo.ErrNotFound
				}
				return nil, uploaded, err
			}
			if !currentVersionID.Valid || currentVersionID.String != input.BaseVersionID {
				return nil, uploaded, &repo.VersionConflictError{LatestVersionID: currentVersionID.String}
			}
			if change.Title == "" {
				change.Title = change.BaseTitle
			}
			if change.FolderID == "" {
				change.FolderID = change.BasePath
			}
			switch input.Operation {
			case repo.PullRequestChangeUpdate:
				if input.HTML == nil {
					return nil, uploaded, repo.ErrInvalidPullRequestChange
				}
			case repo.PullRequestChangeRename:
				if strings.TrimSpace(input.Title) == "" {
					return nil, uploaded, repo.ErrInvalidPullRequestChange
				}
			case repo.PullRequestChangeMove:
				if input.FolderID == "" {
					return nil, uploaded, repo.ErrInvalidPullRequestChange
				}
				if err := verifyFolderTreeKey(ctx, tx, input.FolderID, "folder:"+rootFolderID); err != nil {
					if errors.Is(err, repo.ErrNotFound) {
						return nil, uploaded, repo.ErrRepositoryBoundary
					}
					return nil, uploaded, err
				}
			}
		}

		stored := storedPullRequestChange{change: change, position: index + 1}
		if input.Operation == repo.PullRequestChangeUpdate || input.Operation == repo.PullRequestChangeCreate {
			versionID := newID("ver")
			stored.change.HeadVersionID = versionID
			stored.blobKey = fmt.Sprintf("docs/%s/%s.html", documentID, versionID)
			stored.sha256 = sha(input.HTML)
			if err := s.blobs.Put(ctx, stored.blobKey, htmlContentType, input.HTML); err != nil {
				return nil, uploaded, err
			}
			uploaded = append(uploaded, stored.blobKey)
			if input.Operation == repo.PullRequestChangeUpdate {
				if _, err := tx.Exec(ctx, `
					INSERT INTO versions(id,document_id,number,html_blob_key,sha256,parent_version_id,created_by_type,created_by_id,change_summary)
					VALUES($1,$2,NULL,$3,$4,$5,$6,$7,$8)`, versionID, documentID, stored.blobKey, stored.sha256,
					input.BaseVersionID, string(author.Type), author.ID, changeSummary); err != nil {
					return nil, uploaded, err
				}
			}
		} else if input.Operation != repo.PullRequestChangeDelete {
			stored.change.HeadVersionID = input.BaseVersionID
		}
		prepared = append(prepared, stored)
	}
	return prepared, uploaded, nil
}

func insertPullRequestChanges(ctx context.Context, tx pgx.Tx, changes []storedPullRequestChange) error {
	for _, stored := range changes {
		change := stored.change
		if _, err := tx.Exec(ctx, `
			INSERT INTO pull_request_changes(
				id,pull_request_id,patchset,position,operation,document_id,folder_id,
				base_version_id,head_version_id,base_title,title,base_path,path,head_html_blob_key,head_sha256
			) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$11,$12,$13,NULLIF($14,''),NULLIF($15,''))`,
			change.ID, change.PullRequestID, change.Patchset, stored.position, change.Operation,
			change.DocumentID, change.FolderID, change.BaseVersionID, change.HeadVersionID,
			change.BaseTitle, change.Title, change.BasePath, change.Path, stored.blobKey, stored.sha256); err != nil {
			return err
		}
	}
	return nil
}

func compatibilityChangeValues(changes []storedPullRequestChange) (documentID, baseVersionID, headVersionID any) {
	if len(changes) == 0 {
		return nil, nil, nil
	}
	first := changes[0].change
	if first.Operation == repo.PullRequestChangeCreate {
		return nil, nil, nil
	}
	toNullable := func(value string) any {
		if value == "" {
			return nil
		}
		return value
	}
	return toNullable(first.DocumentID), toNullable(first.BaseVersionID), toNullable(first.HeadVersionID)
}

func (s *Store) CreatePullRequest(ctx context.Context, repositoryID, documentID, baseVersionID, title, body, changeSummary string, html []byte, author auth.Principal) (repo.PullRequest, error) {
	pullRequest, err := s.CreatePullRequestChangeSet(ctx, repositoryID, title, body, changeSummary, []repo.PullRequestChangeInput{{
		Operation: repo.PullRequestChangeUpdate, DocumentID: documentID, BaseVersionID: baseVersionID, HTML: html,
	}}, author)
	if errors.Is(err, repo.ErrRepositoryBoundary) {
		return repo.PullRequest{}, repo.ErrNotFound
	}
	return pullRequest, err
}

func (s *Store) CreatePullRequestChangeSet(ctx context.Context, repositoryID, title, body, changeSummary string, inputs []repo.PullRequestChangeInput, author auth.Principal) (repo.PullRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.PullRequest{}, err
	}
	defer rollback(ctx, tx)
	q := s.q.WithTx(tx)
	if author.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, author); err != nil {
			return repo.PullRequest{}, err
		}
	}
	var number int64
	rootFolderID, err := lockPullRequestTree(ctx, tx, repositoryID)
	if err != nil {
		return repo.PullRequest{}, err
	}
	if _, err := lockRepositoryProtection(ctx, tx, repositoryID); err != nil {
		return repo.PullRequest{}, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE repositories
		SET next_pull_request_number=next_pull_request_number+1,updated_at=now()
		WHERE id=$1
		RETURNING next_pull_request_number-1`, repositoryID).Scan(&number)
	if err != nil {
		return repo.PullRequest{}, err
	}

	pullRequestID := newID("pr")
	changes, uploaded, err := s.preparePullRequestChanges(ctx, tx, pullRequestID, 1, repositoryID, rootFolderID, changeSummary, inputs, author)
	if err != nil {
		for _, key := range uploaded {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
		}
		return repo.PullRequest{}, err
	}
	committed := false
	defer func() {
		if !committed {
			for _, key := range uploaded {
				_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
			}
		}
	}()
	documentIDValue, baseVersionIDValue, headVersionIDValue := compatibilityChangeValues(changes)
	_, err = tx.Exec(ctx, `
		INSERT INTO pull_requests(
			id,repository_id,number,document_id,base_version_id,head_version_id,patchset,
			title,body,change_summary,author_type,author_id,author_email,author_name
		) VALUES($1,$2,$3,$4,$5,$6,1,$7,$8,$9,$10,$11,$12,$13)`,
		pullRequestID, repositoryID, number, documentIDValue, baseVersionIDValue, headVersionIDValue,
		title, body, changeSummary, string(author.Type), author.ID, author.Email, author.Name)
	if err != nil {
		return repo.PullRequest{}, err
	}
	if err := insertPullRequestChanges(ctx, tx, changes); err != nil {
		return repo.PullRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.PullRequest{}, err
	}
	committed = true
	return s.GetPullRequest(ctx, repositoryID, number)
}

func (s *Store) CreatePullRequestPatchset(ctx context.Context, repositoryID string, number int64, changeSummary string, inputs []repo.PullRequestChangeInput, author auth.Principal) (repo.PullRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.PullRequest{}, err
	}
	defer rollback(ctx, tx)
	q := s.q.WithTx(tx)
	if author.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, author); err != nil {
			return repo.PullRequest{}, err
		}
	}
	rootFolderID, err := lockPullRequestTree(ctx, tx, repositoryID)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest, err := scanPullRequest(tx.QueryRow(ctx, pullRequestSelect+` WHERE pr.repository_id=$1 AND pr.number=$2 FOR UPDATE OF pr`, repositoryID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.PullRequest{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.PullRequest{}, err
	}
	if pullRequest.Status != repo.PullRequestStatusOpen {
		return repo.PullRequest{}, repo.ErrPullRequestClosed
	}
	nextPatchset := pullRequest.Patchset + 1
	changes, uploaded, err := s.preparePullRequestChanges(ctx, tx, pullRequest.ID, nextPatchset, repositoryID, rootFolderID, changeSummary, inputs, author)
	if err != nil {
		for _, key := range uploaded {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
		}
		return repo.PullRequest{}, err
	}
	committed := false
	defer func() {
		if !committed {
			for _, key := range uploaded {
				_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
			}
		}
	}()
	if err := insertPullRequestChanges(ctx, tx, changes); err != nil {
		return repo.PullRequest{}, err
	}
	documentID, baseVersionID, headVersionID := compatibilityChangeValues(changes)
	if _, err := tx.Exec(ctx, `
		UPDATE pull_requests
		SET patchset=$1,document_id=$2,base_version_id=$3,head_version_id=$4,
		    change_summary=CASE WHEN $5='' THEN change_summary ELSE $5 END,updated_at=now()
		WHERE id=$6`, nextPatchset, documentID, baseVersionID, headVersionID, changeSummary, pullRequest.ID); err != nil {
		return repo.PullRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.PullRequest{}, err
	}
	committed = true
	return s.GetPullRequest(ctx, repositoryID, number)
}

func loadStoredPullRequestChanges(ctx context.Context, q queryer, pullRequest repo.PullRequest) ([]storedPullRequestChange, error) {
	rows, err := q.Query(ctx, `
		SELECT id,pull_request_id,patchset,position,operation,COALESCE(document_id,''),COALESCE(folder_id,''),
		       COALESCE(base_version_id,''),COALESCE(head_version_id,''),base_title,title,base_path,path,
		       COALESCE(head_html_blob_key,''),COALESCE(head_sha256,'')
		FROM pull_request_changes WHERE pull_request_id=$1 AND patchset=$2 ORDER BY position`, pullRequest.ID, pullRequest.Patchset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []storedPullRequestChange{}
	for rows.Next() {
		var stored storedPullRequestChange
		if err := rows.Scan(&stored.change.ID, &stored.change.PullRequestID, &stored.change.Patchset, &stored.position,
			&stored.change.Operation, &stored.change.DocumentID, &stored.change.FolderID,
			&stored.change.BaseVersionID, &stored.change.HeadVersionID, &stored.change.BaseTitle,
			&stored.change.Title, &stored.change.BasePath, &stored.change.Path, &stored.blobKey, &stored.sha256); err != nil {
			return nil, err
		}
		items = append(items, stored)
	}
	return items, rows.Err()
}

//nolint:gocyclo // Rebase validates each operation and replaces the patchset atomically under tree and PR locks.
func (s *Store) RebasePullRequest(ctx context.Context, repositoryID string, number int64, author auth.Principal) (repo.PullRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.PullRequest{}, err
	}
	defer rollback(ctx, tx)
	q := s.q.WithTx(tx)
	if author.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, author); err != nil {
			return repo.PullRequest{}, err
		}
	}
	rootFolderID, err := lockPullRequestTree(ctx, tx, repositoryID)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest, err := scanPullRequest(tx.QueryRow(ctx, pullRequestSelect+` WHERE pr.repository_id=$1 AND pr.number=$2 FOR UPDATE OF pr`, repositoryID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.PullRequest{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.PullRequest{}, err
	}
	if pullRequest.Status != repo.PullRequestStatusOpen {
		return repo.PullRequest{}, repo.ErrPullRequestClosed
	}
	stored, err := loadStoredPullRequestChanges(ctx, tx, pullRequest)
	if err != nil {
		return repo.PullRequest{}, err
	}
	if len(stored) == 0 {
		return repo.PullRequest{}, repo.ErrStalePullRequest
	}
	inputs := make([]repo.PullRequestChangeInput, 0, len(stored))
	for _, existing := range stored {
		change := existing.change
		input := repo.PullRequestChangeInput{Operation: change.Operation, DocumentID: change.DocumentID, FolderID: change.FolderID, Title: change.Title, Path: change.Path}
		if change.Operation != repo.PullRequestChangeCreate {
			var currentVersionID pgtype.Text
			if err := tx.QueryRow(ctx, `SELECT current_version_id FROM documents WHERE id=$1 FOR UPDATE`, change.DocumentID).Scan(&currentVersionID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return repo.PullRequest{}, repo.ErrStalePullRequest
				}
				return repo.PullRequest{}, err
			}
			input.BaseVersionID = currentVersionID.String
		}
		if change.Operation == repo.PullRequestChangeUpdate || change.Operation == repo.PullRequestChangeCreate {
			if existing.blobKey == "" {
				return repo.PullRequest{}, repo.ErrStalePullRequest
			}
			html, _, err := s.blobs.Get(ctx, existing.blobKey)
			if err != nil {
				return repo.PullRequest{}, err
			}
			input.HTML = html
		}
		inputs = append(inputs, input)
	}
	nextPatchset := pullRequest.Patchset + 1
	changes, uploaded, err := s.preparePullRequestChanges(ctx, tx, pullRequest.ID, nextPatchset, repositoryID, rootFolderID, pullRequest.ChangeSummary, inputs, author)
	if err != nil {
		for _, key := range uploaded {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
		}
		if errors.Is(err, repo.ErrVersionConflict) {
			return repo.PullRequest{}, repo.ErrStalePullRequest
		}
		return repo.PullRequest{}, err
	}
	committed := false
	defer func() {
		if !committed {
			for _, key := range uploaded {
				_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
			}
		}
	}()
	if err := insertPullRequestChanges(ctx, tx, changes); err != nil {
		return repo.PullRequest{}, err
	}
	documentID, baseVersionID, headVersionID := compatibilityChangeValues(changes)
	if _, err := tx.Exec(ctx, `
		UPDATE pull_requests SET patchset=$1,document_id=$2,base_version_id=$3,head_version_id=$4,updated_at=now()
		WHERE id=$5`, nextPatchset, documentID, baseVersionID, headVersionID, pullRequest.ID); err != nil {
		return repo.PullRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.PullRequest{}, err
	}
	committed = true
	return s.GetPullRequest(ctx, repositoryID, number)
}

func (s *Store) ListPullRequests(ctx context.Context, repositoryID string) ([]repo.PullRequest, error) {
	rows, err := s.db.Query(ctx, pullRequestSelect+` WHERE pr.repository_id=$1 ORDER BY pr.number DESC`, repositoryID)
	if err != nil {
		return nil, err
	}
	var items []repo.PullRequest
	for rows.Next() {
		pullRequest, err := scanPullRequest(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, pullRequest)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if items == nil {
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE id=$1)`, repositoryID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, repo.ErrNotFound
		}
		items = []repo.PullRequest{}
	}
	for index := range items {
		items[index], err = decoratePullRequest(ctx, s.db, items[index])
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *Store) GetPullRequest(ctx context.Context, repositoryID string, number int64) (repo.PullRequest, error) {
	pullRequest, err := scanPullRequest(s.db.QueryRow(ctx, pullRequestSelect+` WHERE pr.repository_id=$1 AND pr.number=$2`, repositoryID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.PullRequest{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.PullRequest{}, err
	}
	return decoratePullRequest(ctx, s.db, pullRequest)
}

func (s *Store) ReviewPullRequest(ctx context.Context, repositoryID string, number int64, reviewer auth.Principal, state, body string) (repo.PullRequest, error) {
	if state != repo.ReviewStateApproved && state != repo.ReviewStateChangesRequested {
		return repo.PullRequest{}, repo.ErrConflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.PullRequest{}, err
	}
	defer rollback(ctx, tx)
	pullRequest, err := scanPullRequest(tx.QueryRow(ctx, pullRequestSelect+` WHERE pr.repository_id=$1 AND pr.number=$2 FOR UPDATE OF pr`, repositoryID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.PullRequest{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.PullRequest{}, err
	}
	if pullRequest.Status != repo.PullRequestStatusOpen {
		return repo.PullRequest{}, repo.ErrPullRequestClosed
	}
	if pullRequest.Author.Type == reviewer.Type && pullRequest.Author.ID == reviewer.ID {
		return repo.PullRequest{}, repo.ErrSelfReview
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO pull_request_reviews(
			id,pull_request_id,head_version_id,patchset,reviewer_type,reviewer_id,
			reviewer_email,reviewer_name,state,body
		) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT(pull_request_id,reviewer_type,reviewer_id)
		DO UPDATE SET head_version_id=EXCLUDED.head_version_id,
		              patchset=EXCLUDED.patchset,
		              reviewer_email=EXCLUDED.reviewer_email,
		              reviewer_name=EXCLUDED.reviewer_name,
		              state=EXCLUDED.state,body=EXCLUDED.body,submitted_at=now()`,
		newID("review"), pullRequest.ID, pullRequest.HeadVersionID, pullRequest.Patchset, string(reviewer.Type),
		reviewer.ID, reviewer.Email, reviewer.Name, state, body)
	if err != nil {
		return repo.PullRequest{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE pull_requests SET updated_at=now() WHERE id=$1`, pullRequest.ID); err != nil {
		return repo.PullRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.PullRequest{}, err
	}
	return s.GetPullRequest(ctx, repositoryID, number)
}

func mergeBlockError(pullRequest repo.PullRequest) error {
	switch pullRequest.MergeBlockedReason {
	case repo.ErrPullRequestClosed.Error():
		return repo.ErrPullRequestClosed
	case repo.ErrStalePullRequest.Error():
		return repo.ErrStalePullRequest
	case repo.ErrChangesRequested.Error():
		return repo.ErrChangesRequested
	default:
		return repo.ErrApprovalRequired
	}
}

//nolint:gocyclo // Applying the operation matrix and publishing versions must remain one all-or-nothing transaction.
func (s *Store) MergePullRequest(ctx context.Context, repositoryID string, number int64, mergedBy auth.Principal) (repo.PullRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.PullRequest{}, err
	}
	defer rollback(ctx, tx)
	rootFolderID, err := lockPullRequestTree(ctx, tx, repositoryID)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest, err := scanPullRequest(tx.QueryRow(ctx, pullRequestSelect+` WHERE pr.repository_id=$1 AND pr.number=$2 FOR UPDATE OF pr`, repositoryID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.PullRequest{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.PullRequest{}, err
	}
	changes, err := loadStoredPullRequestChanges(ctx, tx, pullRequest)
	if err != nil {
		return repo.PullRequest{}, err
	}
	if len(changes) == 0 && pullRequest.DocumentID != "" {
		changes = []storedPullRequestChange{{change: repo.PullRequestChange{
			PullRequestID: pullRequest.ID, Patchset: pullRequest.Patchset,
			Operation: repo.PullRequestChangeUpdate, DocumentID: pullRequest.DocumentID,
			BaseVersionID: pullRequest.BaseVersionID, HeadVersionID: pullRequest.HeadVersionID,
		}}}
	}
	pullRequest.Changes = make([]repo.PullRequestChange, 0, len(changes))
	for _, stored := range changes {
		pullRequest.Changes = append(pullRequest.Changes, stored.change)
	}
	reviews, err := loadPullRequestReviews(ctx, tx, pullRequest.ID)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest.Reviews = reviews
	mergeReviews, err := eligibleReviews(ctx, tx, pullRequest)
	if err != nil {
		return repo.PullRequest{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT root_folder_id FROM repositories WHERE id=$1 FOR UPDATE`, repositoryID).Scan(&rootFolderID); err != nil {
		return repo.PullRequest{}, err
	}
	documentIDs := []string{}
	for _, stored := range changes {
		if stored.change.Operation != repo.PullRequestChangeCreate {
			documentIDs = append(documentIDs, stored.change.DocumentID)
		}
	}
	sort.Strings(documentIDs)
	for _, documentID := range documentIDs {
		var ignored pgtype.Text
		if err := tx.QueryRow(ctx, `SELECT current_version_id FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&ignored); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return repo.PullRequest{}, repo.ErrStalePullRequest
			}
			return repo.PullRequest{}, err
		}
	}
	stale, err := pullRequestStaleWith(ctx, tx, pullRequest)
	if err != nil {
		return repo.PullRequest{}, err
	}
	pullRequest.Reviews = mergeReviews
	pullRequest = computePullRequestMergeability(pullRequest, stale)
	if !pullRequest.Mergeable {
		return repo.PullRequest{}, mergeBlockError(pullRequest)
	}
	// Validate all proposed content before applying any tree mutation. The SQL
	// transaction and row/advisory locks make the complete change set atomic.
	deletedBlobKeys := []string{}
	for _, stored := range changes {
		change := stored.change
		switch change.Operation {
		case repo.PullRequestChangeUpdate:
			var proposed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM versions WHERE id=$1 AND document_id=$2 AND number IS NULL)`, change.HeadVersionID, change.DocumentID).Scan(&proposed); err != nil {
				return repo.PullRequest{}, err
			}
			if !proposed {
				return repo.PullRequest{}, repo.ErrStalePullRequest
			}
		case repo.PullRequestChangeCreate:
			if stored.blobKey == "" || stored.sha256 == "" || change.Title == "" || change.FolderID == "" {
				return repo.PullRequest{}, repo.ErrInvalidPullRequestChange
			}
		case repo.PullRequestChangeDelete:
			rows, err := tx.Query(ctx, `SELECT html_blob_key FROM versions WHERE document_id=$1`, change.DocumentID)
			if err != nil {
				return repo.PullRequest{}, err
			}
			for rows.Next() {
				var key string
				if err := rows.Scan(&key); err != nil {
					rows.Close()
					return repo.PullRequest{}, err
				}
				deletedBlobKeys = append(deletedBlobKeys, key)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return repo.PullRequest{}, err
			}
			rows.Close()
		}
	}
	for _, stored := range changes {
		change := stored.change
		switch change.Operation {
		case repo.PullRequestChangeUpdate:
			var nextVersionNumber int32
			if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(number),0)+1 FROM versions WHERE document_id=$1 AND number IS NOT NULL`, change.DocumentID).Scan(&nextVersionNumber); err != nil {
				return repo.PullRequest{}, err
			}
			if _, err := tx.Exec(ctx, `UPDATE versions SET number=$1 WHERE id=$2 AND number IS NULL`, nextVersionNumber, change.HeadVersionID); err != nil {
				return repo.PullRequest{}, err
			}
			if _, err := tx.Exec(ctx, `UPDATE documents SET current_version_id=$1,updated_at=now() WHERE id=$2`, change.HeadVersionID, change.DocumentID); err != nil {
				return repo.PullRequest{}, err
			}
		case repo.PullRequestChangeCreate:
			if _, err := tx.Exec(ctx, `INSERT INTO documents(id,title,owner_type,owner_id,parent_folder_id) VALUES($1,$2,$3,$4,$5)`, change.DocumentID, change.Title, string(pullRequest.Author.Type), pullRequest.Author.ID, change.FolderID); err != nil {
				return repo.PullRequest{}, repositoryWriteError(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO versions(id,document_id,number,html_blob_key,sha256,parent_version_id,created_by_type,created_by_id,change_summary)
				VALUES($1,$2,1,$3,$4,NULL,$5,$6,$7)`, change.HeadVersionID, change.DocumentID,
				stored.blobKey, stored.sha256, string(pullRequest.Author.Type), pullRequest.Author.ID, pullRequest.ChangeSummary); err != nil {
				return repo.PullRequest{}, err
			}
			if _, err := tx.Exec(ctx, `UPDATE documents SET current_version_id=$1,updated_at=now() WHERE id=$2`, change.HeadVersionID, change.DocumentID); err != nil {
				return repo.PullRequest{}, err
			}
		case repo.PullRequestChangeRename:
			if _, err := tx.Exec(ctx, `UPDATE documents SET title=$1,updated_at=now() WHERE id=$2`, change.Title, change.DocumentID); err != nil {
				return repo.PullRequest{}, repositoryWriteError(err)
			}
		case repo.PullRequestChangeMove:
			if _, err := tx.Exec(ctx, `UPDATE documents SET parent_folder_id=$1,updated_at=now() WHERE id=$2`, change.FolderID, change.DocumentID); err != nil {
				return repo.PullRequest{}, repositoryWriteError(err)
			}
		case repo.PullRequestChangeDelete:
			if _, err := tx.Exec(ctx, `DELETE FROM resource_grants WHERE resource_type='document' AND resource_id=$1`, change.DocumentID); err != nil {
				return repo.PullRequest{}, err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM pins WHERE resource_type='document' AND resource_id=$1`, change.DocumentID); err != nil {
				return repo.PullRequest{}, err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE id=$1`, change.DocumentID); err != nil {
				return repo.PullRequest{}, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pull_requests
		SET status='merged',merged_by_type=$1,merged_by_id=$2,
		    merged_by_email=$3,merged_by_name=$4,merged_at=now(),updated_at=now()
		WHERE id=$5`, string(mergedBy.Type), mergedBy.ID, mergedBy.Email, mergedBy.Name, pullRequest.ID); err != nil {
		return repo.PullRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.PullRequest{}, err
	}
	for _, key := range deletedBlobKeys {
		if err := s.blobs.Delete(context.WithoutCancel(ctx), key); err != nil {
			log.Printf("pull request merged but deleted-document blob cleanup failed repository_id=%q pull_request=%d blob_key=%q error=%v", repositoryID, number, key, err)
		}
	}
	return s.GetPullRequest(ctx, repositoryID, number)
}

func (s *Store) ClosePullRequest(ctx context.Context, repositoryID string, number int64, _ auth.Principal) (repo.PullRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.PullRequest{}, err
	}
	defer rollback(ctx, tx)
	pullRequest, err := scanPullRequest(tx.QueryRow(ctx, pullRequestSelect+` WHERE pr.repository_id=$1 AND pr.number=$2 FOR UPDATE OF pr`, repositoryID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return repo.PullRequest{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.PullRequest{}, err
	}
	if pullRequest.Status != repo.PullRequestStatusOpen {
		return repo.PullRequest{}, repo.ErrPullRequestClosed
	}
	if _, err := tx.Exec(ctx, `UPDATE pull_requests SET status='closed',closed_at=now(),updated_at=now() WHERE id=$1`, pullRequest.ID); err != nil {
		return repo.PullRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repo.PullRequest{}, err
	}
	return s.GetPullRequest(ctx, repositoryID, number)
}

var _ repo.Repository = (*Store)(nil)
var _ repo.Repository = (*repo.Memory)(nil)
