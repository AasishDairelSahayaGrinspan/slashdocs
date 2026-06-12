// Hand-written query layer for folders (mirrors sqlc generated style).
// source: folders.sql

package dbsqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const insertFolder = `INSERT INTO folders(id, title, owner_type, owner_id) VALUES($1, $2, $3, $4)`

type InsertFolderParams struct {
	ID        string
	Title     string
	OwnerType string
	OwnerID   string
}

func (q *Queries) InsertFolder(ctx context.Context, arg InsertFolderParams) error {
	_, err := q.db.Exec(ctx, insertFolder, arg.ID, arg.Title, arg.OwnerType, arg.OwnerID)
	return err
}

const getFolder = `SELECT f.id, f.title, f.owner_type, f.owner_id,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM folders f
LEFT JOIN users u ON f.owner_type='user' AND u.id=f.owner_id
LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND sa.id=f.owner_id
WHERE f.id=$1`

type GetFolderRow struct {
	ID        string
	Title     string
	OwnerType string
	OwnerID   string
	OwnerEmail string
	OwnerName  string
}

func (q *Queries) GetFolder(ctx context.Context, id string) (GetFolderRow, error) {
	row := q.db.QueryRow(ctx, getFolder, id)
	var i GetFolderRow
	err := row.Scan(&i.ID, &i.Title, &i.OwnerType, &i.OwnerID, &i.OwnerEmail, &i.OwnerName)
	return i, err
}

const getFolderOwner = `SELECT owner_type, owner_id FROM folders WHERE id=$1`

type GetFolderOwnerRow struct {
	OwnerType string
	OwnerID   string
}

func (q *Queries) GetFolderOwner(ctx context.Context, id string) (GetFolderOwnerRow, error) {
	row := q.db.QueryRow(ctx, getFolderOwner, id)
	var i GetFolderOwnerRow
	err := row.Scan(&i.OwnerType, &i.OwnerID)
	return i, err
}

const listFolders = `SELECT f.id, f.title, f.owner_type, f.owner_id,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM folders f
LEFT JOIN users u ON f.owner_type='user' AND u.id=f.owner_id
LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND sa.id=f.owner_id
WHERE (f.owner_type=$2 AND f.owner_id=$1)
   OR EXISTS (
       SELECT 1 FROM resource_grants g
       WHERE g.resource_type='folder' AND g.resource_id=f.id AND g.revoked_at IS NULL
         AND g.principal_type=$2 AND g.principal_id=$1
   )
   OR EXISTS (
       SELECT 1 FROM resource_grants g
       WHERE g.resource_type='folder' AND g.resource_id=f.id AND g.revoked_at IS NULL
         AND g.principal_type='anyone'
   )
ORDER BY f.updated_at DESC`

type ListFoldersParams struct {
	OwnerID   string
	OwnerType string
}

type ListFoldersRow struct {
	ID         string
	Title      string
	OwnerType  string
	OwnerID    string
	OwnerEmail string
	OwnerName  string
}

func (q *Queries) ListFolders(ctx context.Context, arg ListFoldersParams) ([]ListFoldersRow, error) {
	rows, err := q.db.Query(ctx, listFolders, arg.OwnerID, arg.OwnerType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListFoldersRow
	for rows.Next() {
		var i ListFoldersRow
		if err := rows.Scan(&i.ID, &i.Title, &i.OwnerType, &i.OwnerID, &i.OwnerEmail, &i.OwnerName); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const updateFolderTitle = `UPDATE folders SET title=$1, updated_at=now() WHERE id=$2`

type UpdateFolderTitleParams struct {
	Title string
	ID    string
}

func (q *Queries) UpdateFolderTitle(ctx context.Context, arg UpdateFolderTitleParams) error {
	_, err := q.db.Exec(ctx, updateFolderTitle, arg.Title, arg.ID)
	return err
}

const deleteFolder = `DELETE FROM folders WHERE id=$1`

func (q *Queries) DeleteFolder(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, deleteFolder, id)
	return err
}

const getFolderGrantRole = `SELECT role FROM resource_grants WHERE resource_type='folder' AND resource_id=$1 AND principal_type=$2 AND principal_id=$3 AND revoked_at IS NULL`

type GetFolderGrantRoleParams struct {
	ResourceID    string
	PrincipalType string
	PrincipalID   string
}

func (q *Queries) GetFolderGrantRole(ctx context.Context, arg GetFolderGrantRoleParams) (string, error) {
	row := q.db.QueryRow(ctx, getFolderGrantRole, arg.ResourceID, arg.PrincipalType, arg.PrincipalID)
	var role string
	err := row.Scan(&role)
	return role, err
}

const upsertFolderGrant = `INSERT INTO resource_grants(id,resource_type,resource_id,principal_type,principal_id,role,granted_by_type,granted_by_user_id)
VALUES($1,'folder',$2,$3,$4,$5,$6,$7)
ON CONFLICT(resource_type,resource_id,principal_type,principal_id) DO UPDATE SET role=EXCLUDED.role, revoked_at=NULL, granted_by_type=EXCLUDED.granted_by_type, granted_by_user_id=EXCLUDED.granted_by_user_id
RETURNING id`

type UpsertFolderGrantParams struct {
	ID              string
	ResourceID      string
	PrincipalType   string
	PrincipalID     string
	Role            string
	GrantedByType   string
	GrantedByUserID string
}

func (q *Queries) UpsertFolderGrant(ctx context.Context, arg UpsertFolderGrantParams) (string, error) {
	row := q.db.QueryRow(ctx, upsertFolderGrant, arg.ID, arg.ResourceID, arg.PrincipalType, arg.PrincipalID, arg.Role, arg.GrantedByType, arg.GrantedByUserID)
	var id string
	err := row.Scan(&id)
	return id, err
}

const listFolderGrants = `SELECT
  g.id, g.principal_type, g.principal_id,
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
WHERE g.resource_type='folder' AND g.resource_id=$1 AND g.revoked_at IS NULL`

type ListFolderGrantsRow struct {
	ID                  string
	PrincipalType       string
	PrincipalID         string
	PrincipalEmail      string
	PrincipalName       string
	PrincipalPictureUrl string
	Role                string
	GrantedByType       string
	GrantedByUserID     string
	GrantedByEmail      string
	GrantedByName       string
}

func (q *Queries) ListFolderGrants(ctx context.Context, resourceID string) ([]ListFolderGrantsRow, error) {
	rows, err := q.db.Query(ctx, listFolderGrants, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListFolderGrantsRow
	for rows.Next() {
		var i ListFolderGrantsRow
		if err := rows.Scan(
			&i.ID, &i.PrincipalType, &i.PrincipalID,
			&i.PrincipalEmail, &i.PrincipalName, &i.PrincipalPictureUrl,
			&i.Role, &i.GrantedByType, &i.GrantedByUserID,
			&i.GrantedByEmail, &i.GrantedByName,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const updateFolderGrantRole = `WITH updated AS (
  UPDATE resource_grants SET role=$1
  WHERE resource_grants.id=$2 AND resource_type='folder' AND resource_id=$3 AND revoked_at IS NULL
  RETURNING id, resource_type, resource_id, principal_type, principal_id, role, granted_by_user_id, created_at, revoked_at, granted_by_type
)
SELECT g.id, g.resource_id, g.principal_type, g.principal_id,
  COALESCE(u.email,'') AS principal_email,
  COALESCE(u.name,sa.name,'') AS principal_name,
  COALESCE(u.picture_url,'') AS principal_picture_url,
  g.role, g.granted_by_type, g.granted_by_user_id,
  COALESCE(gu.email,'') AS granted_by_email,
  COALESCE(gu.name,gsa.name,'') AS granted_by_name
FROM updated g
LEFT JOIN users u ON g.principal_type='user' AND g.principal_id=u.id
LEFT JOIN service_accounts sa ON g.principal_type='service_account' AND g.principal_id=sa.id
LEFT JOIN users gu ON g.granted_by_type='user' AND g.granted_by_user_id=gu.id
LEFT JOIN service_accounts gsa ON g.granted_by_type='service_account' AND g.granted_by_user_id=gsa.id`

type UpdateFolderGrantRoleParams struct {
	Role       string
	ID         string
	ResourceID string
}

type UpdateFolderGrantRoleRow struct {
	ID                  string
	ResourceID          string
	PrincipalType       string
	PrincipalID         string
	PrincipalEmail      string
	PrincipalName       string
	PrincipalPictureUrl string
	Role                string
	GrantedByType       string
	GrantedByUserID     string
	GrantedByEmail      string
	GrantedByName       string
}

func (q *Queries) UpdateFolderGrantRole(ctx context.Context, arg UpdateFolderGrantRoleParams) (UpdateFolderGrantRoleRow, error) {
	row := q.db.QueryRow(ctx, updateFolderGrantRole, arg.Role, arg.ID, arg.ResourceID)
	var i UpdateFolderGrantRoleRow
	err := row.Scan(
		&i.ID, &i.ResourceID, &i.PrincipalType, &i.PrincipalID,
		&i.PrincipalEmail, &i.PrincipalName, &i.PrincipalPictureUrl,
		&i.Role, &i.GrantedByType, &i.GrantedByUserID,
		&i.GrantedByEmail, &i.GrantedByName,
	)
	return i, err
}

const deleteFolderGrant = `UPDATE resource_grants SET revoked_at=now()
WHERE id=$1 AND resource_type='folder' AND resource_id=$2 AND revoked_at IS NULL
RETURNING id`

type DeleteFolderGrantParams struct {
	ID         string
	ResourceID string
}

func (q *Queries) DeleteFolderGrant(ctx context.Context, arg DeleteFolderGrantParams) (string, error) {
	row := q.db.QueryRow(ctx, deleteFolderGrant, arg.ID, arg.ResourceID)
	var id string
	err := row.Scan(&id)
	return id, err
}

const addDocumentToFolder = `INSERT INTO folder_documents(folder_id, document_id, added_by_type, added_by_id)
VALUES($1, $2, $3, $4)
ON CONFLICT DO NOTHING`

type AddDocumentToFolderParams struct {
	FolderID     string
	DocumentID   string
	AddedByType  string
	AddedByID    string
}

func (q *Queries) AddDocumentToFolder(ctx context.Context, arg AddDocumentToFolderParams) error {
	_, err := q.db.Exec(ctx, addDocumentToFolder, arg.FolderID, arg.DocumentID, arg.AddedByType, arg.AddedByID)
	return err
}

const removeDocumentFromFolder = `DELETE FROM folder_documents WHERE folder_id=$1 AND document_id=$2`

type RemoveDocumentFromFolderParams struct {
	FolderID   string
	DocumentID string
}

func (q *Queries) RemoveDocumentFromFolder(ctx context.Context, arg RemoveDocumentFromFolderParams) error {
	_, err := q.db.Exec(ctx, removeDocumentFromFolder, arg.FolderID, arg.DocumentID)
	return err
}

const listFolderDocuments = `SELECT d.id, d.title, d.current_version_id, d.owner_type, d.owner_id,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM folder_documents fd
JOIN documents d ON d.id=fd.document_id
LEFT JOIN users u ON d.owner_type='user' AND u.id=d.owner_id
LEFT JOIN service_accounts sa ON d.owner_type='service_account' AND sa.id=d.owner_id
WHERE fd.folder_id=$1
ORDER BY fd.added_at ASC`

type ListFolderDocumentsRow struct {
	ID               string
	Title            string
	CurrentVersionID pgtype.Text
	OwnerType        string
	OwnerID          string
	OwnerEmail       string
	OwnerName        string
}

func (q *Queries) ListFolderDocuments(ctx context.Context, folderID string) ([]ListFolderDocumentsRow, error) {
	rows, err := q.db.Query(ctx, listFolderDocuments, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListFolderDocumentsRow
	for rows.Next() {
		var i ListFolderDocumentsRow
		if err := rows.Scan(&i.ID, &i.Title, &i.CurrentVersionID, &i.OwnerType, &i.OwnerID, &i.OwnerEmail, &i.OwnerName); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const getDocumentFolderGrants = `SELECT fd.folder_id, g.role
FROM folder_documents fd
JOIN resource_grants g ON g.resource_type='folder' AND g.resource_id=fd.folder_id AND g.revoked_at IS NULL
WHERE fd.document_id=$1
  AND (
    (g.principal_type=$2 AND g.principal_id=$3)
    OR g.principal_type='anyone'
  )`

type GetDocumentFolderGrantsParams struct {
	DocumentID    string
	PrincipalType string
	PrincipalID   string
}

type GetDocumentFolderGrantsRow struct {
	FolderID string
	Role     string
}

func (q *Queries) GetDocumentFolderGrants(ctx context.Context, arg GetDocumentFolderGrantsParams) ([]GetDocumentFolderGrantsRow, error) {
	rows, err := q.db.Query(ctx, getDocumentFolderGrants, arg.DocumentID, arg.PrincipalType, arg.PrincipalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetDocumentFolderGrantsRow
	for rows.Next() {
		var i GetDocumentFolderGrantsRow
		if err := rows.Scan(&i.FolderID, &i.Role); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const getDocumentFolderOwnership = `SELECT fd.folder_id
FROM folder_documents fd
JOIN folders f ON f.id=fd.folder_id
WHERE fd.document_id=$1 AND f.owner_type=$2 AND f.owner_id=$3`

type GetDocumentFolderOwnershipParams struct {
	DocumentID string
	OwnerType  string
	OwnerID    string
}

func (q *Queries) GetDocumentFolderOwnership(ctx context.Context, arg GetDocumentFolderOwnershipParams) ([]string, error) {
	rows, err := q.db.Query(ctx, getDocumentFolderOwnership, arg.DocumentID, arg.OwnerType, arg.OwnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var folderID string
		if err := rows.Scan(&folderID); err != nil {
			return nil, err
		}
		items = append(items, folderID)
	}
	return items, rows.Err()
}

const getDocumentFolderIDs = `SELECT folder_id FROM folder_documents WHERE document_id=$1`

func (q *Queries) GetDocumentFolderIDs(ctx context.Context, documentID string) ([]string, error) {
	rows, err := q.db.Query(ctx, getDocumentFolderIDs, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var folderID string
		if err := rows.Scan(&folderID); err != nil {
			return nil, err
		}
		items = append(items, folderID)
	}
	return items, rows.Err()
}
