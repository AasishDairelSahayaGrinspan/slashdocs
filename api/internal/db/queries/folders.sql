-- name: InsertFolder :exec
INSERT INTO folders(id, title, owner_type, owner_id) VALUES($1, $2, $3, $4);

-- name: GetFolder :one
SELECT f.id, f.title, f.owner_type, f.owner_id,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM folders f
LEFT JOIN users u ON f.owner_type='user' AND u.id=f.owner_id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND sa.id=f.owner_id
WHERE f.id=$1;

-- name: GetFolderOwner :one
SELECT owner_type, owner_id FROM folders WHERE id=$1;

-- name: ListFolders :many
-- A folder appears in a principal's workspace when they own it or hold an explicit grant.
SELECT f.id, f.title, f.owner_type, f.owner_id,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM folders f
LEFT JOIN users u ON f.owner_type='user' AND u.id=f.owner_id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa ON f.owner_type='service_account' AND sa.id=f.owner_id
WHERE (f.owner_type=$2 AND f.owner_id=$1)
   OR EXISTS (
       SELECT 1 FROM resource_grants g
       WHERE g.resource_type='folder'
         AND g.resource_id=f.id
         AND g.revoked_at IS NULL
         AND g.principal_type=$2 AND g.principal_id=$1
   )
   OR EXISTS (
       SELECT 1 FROM resource_grants g
       WHERE g.resource_type='folder'
         AND g.resource_id=f.id
         AND g.revoked_at IS NULL
         AND g.principal_type='anyone'
   )
ORDER BY f.updated_at DESC;

-- name: UpdateFolderTitle :exec
UPDATE folders SET title=$1, updated_at=now() WHERE id=$2;

-- name: DeleteFolder :exec
DELETE FROM folders WHERE id=$1;

-- name: GetFolderGrantRole :one
SELECT role FROM resource_grants WHERE resource_type='folder' AND resource_id=$1 AND principal_type=$2 AND principal_id=$3 AND revoked_at IS NULL;

-- name: UpsertFolderGrant :one
INSERT INTO resource_grants(id,resource_type,resource_id,principal_type,principal_id,role,granted_by_type,granted_by_user_id)
VALUES($1,'folder',$2,$3,$4,$5,$6,$7)
ON CONFLICT(resource_type,resource_id,principal_type,principal_id) DO UPDATE SET role=EXCLUDED.role, revoked_at=NULL, granted_by_type=EXCLUDED.granted_by_type, granted_by_user_id=EXCLUDED.granted_by_user_id
RETURNING id;

-- name: ListFolderGrants :many
SELECT
  g.id, g.principal_type, g.principal_id,
  COALESCE(u.email,'') AS principal_email,
  COALESCE(u.name,sa.name,'') AS principal_name,
  COALESCE(u.picture_url,'') AS principal_picture_url,
  g.role, g.granted_by_type, g.granted_by_user_id,
  COALESCE(gu.email,'') AS granted_by_email,
  COALESCE(gu.name,gsa.name,'') AS granted_by_name
FROM resource_grants g
LEFT JOIN users u ON g.principal_type='user' AND g.principal_id=u.id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa ON g.principal_type='service_account' AND g.principal_id=sa.id
LEFT JOIN users gu ON g.granted_by_type='user' AND g.granted_by_user_id=gu.id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts gsa ON g.granted_by_type='service_account' AND g.granted_by_user_id=gsa.id
WHERE g.resource_type='folder' AND g.resource_id=$1 AND g.revoked_at IS NULL;

-- name: UpdateFolderGrantRole :one
WITH updated AS (
  UPDATE resource_grants
  SET role=$1
  WHERE resource_grants.id=$2 AND resource_type='folder' AND resource_id=$3 AND revoked_at IS NULL
  RETURNING id, resource_type, resource_id, principal_type, principal_id, role, granted_by_user_id, created_at, revoked_at, granted_by_type
)
SELECT
  g.id, g.resource_id, g.principal_type, g.principal_id,
  COALESCE(u.email,'') AS principal_email,
  COALESCE(u.name,sa.name,'') AS principal_name,
  COALESCE(u.picture_url,'') AS principal_picture_url,
  g.role, g.granted_by_type, g.granted_by_user_id,
  COALESCE(gu.email,'') AS granted_by_email,
  COALESCE(gu.name,gsa.name,'') AS granted_by_name
FROM updated g
LEFT JOIN users u ON g.principal_type='user' AND g.principal_id=u.id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa ON g.principal_type='service_account' AND g.principal_id=sa.id
LEFT JOIN users gu ON g.granted_by_type='user' AND g.granted_by_user_id=gu.id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts gsa ON g.granted_by_type='service_account' AND g.granted_by_user_id=gsa.id;

-- name: DeleteFolderGrant :one
UPDATE resource_grants SET revoked_at=now()
WHERE id=$1 AND resource_type='folder' AND resource_id=$2 AND revoked_at IS NULL
RETURNING id;

-- name: AddDocumentToFolder :exec
INSERT INTO folder_documents(folder_id, document_id, added_by_type, added_by_id)
VALUES($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: RemoveDocumentFromFolder :exec
DELETE FROM folder_documents WHERE folder_id=$1 AND document_id=$2;

-- name: ListFolderDocuments :many
SELECT d.id, d.title, d.current_version_id, d.owner_type, d.owner_id,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM folder_documents fd
JOIN documents d ON d.id=fd.document_id
LEFT JOIN users u ON d.owner_type='user' AND u.id=d.owner_id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa ON d.owner_type='service_account' AND sa.id=d.owner_id
WHERE fd.folder_id=$1
ORDER BY fd.added_at ASC;

-- name: GetDocumentFolderGrants :many
-- Returns (folder_id, role) pairs for all folders containing document_id where principal has a role.
SELECT fd.folder_id, g.role
FROM folder_documents fd
JOIN resource_grants g ON g.resource_type='folder' AND g.resource_id=fd.folder_id AND g.revoked_at IS NULL
WHERE fd.document_id=$1
  AND (
    (g.principal_type=$2 AND g.principal_id=$3)
    OR g.principal_type='anyone'
  );

-- name: GetDocumentFolderOwnership :many
-- Returns folder_ids of folders containing document_id that the principal owns.
SELECT fd.folder_id
FROM folder_documents fd
JOIN folders f ON f.id=fd.folder_id
WHERE fd.document_id=$1 AND f.owner_type=$2 AND f.owner_id=$3;

-- name: GetDocumentFolderIDs :many
-- Returns all folder IDs that contain a given document.
SELECT folder_id FROM folder_documents WHERE document_id=$1;
