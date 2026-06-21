-- name: GetDocumentOwner :one
SELECT owner_type, owner_id FROM documents WHERE id=$1;

-- name: GetDocumentSource :one
-- Minimal lookup used by RoleForDocument to decide whether aidocs grants are
-- inert (Drive-backed docs are gated by Drive ACLs, not aidocs grants).
SELECT source FROM documents WHERE id=$1;

-- name: GetDocumentGrantRole :one
SELECT role FROM resource_grants WHERE resource_type='document' AND resource_id=$1 AND principal_type=$2 AND principal_id=$3 AND revoked_at IS NULL;

-- name: InsertDocument :exec
INSERT INTO documents(id,title,owner_type,owner_id) VALUES($1,$2,$3,$4);

-- name: UpsertDriveDocument :one
-- One aidocs document per Drive file: insert a thin pointer (no versioned HTML)
-- or, on conflict by drive_file_id, refresh the title and resource key. Drive
-- resource keys can rotate, so we always overwrite rather than coalesce.
INSERT INTO documents (id, title, owner_type, owner_id, source, drive_file_id, drive_resource_key)
VALUES ($1, $2, $3, $4, 'drive', $5, $6)
ON CONFLICT (drive_file_id) WHERE drive_file_id IS NOT NULL
DO UPDATE SET title = EXCLUDED.title, drive_resource_key = EXCLUDED.drive_resource_key, updated_at = now()
RETURNING id, title, current_version_id, owner_type, owner_id, source, drive_file_id, drive_resource_key;

-- name: UpdateDocumentCurrentVersion :exec
UPDATE documents SET current_version_id=$1, updated_at=now() WHERE id=$2;

-- name: GetDocument :one
SELECT d.id,d.title,d.current_version_id,d.owner_type,d.owner_id,d.source,d.drive_file_id,d.drive_resource_key,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM documents d
LEFT JOIN users u ON d.owner_type='user' AND u.id=d.owner_id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
  -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type, not a Google credential.
  ON d.owner_type='service_account' AND sa.id=d.owner_id -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
WHERE d.id=$1;

-- name: GetDocumentVersionForUpdate :one
SELECT current_version_id, COALESCE((SELECT max(number)+1 FROM versions WHERE document_id=$1),1)::int AS next_number
FROM documents WHERE id=$1 FOR UPDATE;

-- name: RecordWorkspaceEntry :exec
-- Records that a principal has opened a document, so a broadly-shared ("anyone")
-- document joins their workspace listing after first open. Idempotent.
INSERT INTO document_workspace_entries(document_id,principal_type,principal_id)
VALUES($1,$2,$3)
ON CONFLICT(document_id,principal_type,principal_id) DO NOTHING;

-- name: ListDocuments :many
-- A document appears in a principal's workspace when they:
--   1. own it, or
--   2. hold an explicit (user/service-account) grant for it, or
--   3. have opened it (a workspace entry exists) AND it is still broadly shared
--      via an active "anyone" grant.
-- A bare "anyone" grant alone does NOT list the document: broad access grants
-- access via discovery/link, it does not place the document in every user's
-- workspace until they open it.
SELECT d.id,d.title,d.current_version_id,d.owner_type,d.owner_id,d.source,d.drive_file_id,d.drive_resource_key,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM documents d
LEFT JOIN users u ON d.owner_type='user' AND u.id=d.owner_id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
  -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type, not a Google credential.
  ON d.owner_type='service_account' AND sa.id=d.owner_id -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
WHERE (d.owner_type=$2 AND d.owner_id=$1)
   OR EXISTS (
       SELECT 1 FROM resource_grants g
       WHERE g.resource_type='document'
         AND g.resource_id=d.id
         AND g.revoked_at IS NULL
         AND g.principal_type=$2 AND g.principal_id=$1
   )
   OR (
       EXISTS (
           SELECT 1 FROM document_workspace_entries w
           WHERE w.document_id=d.id
             AND w.principal_type=$2 AND w.principal_id=$1
       )
       AND EXISTS (
           SELECT 1 FROM resource_grants g
           WHERE g.resource_type='document'
             AND g.resource_id=d.id
             AND g.revoked_at IS NULL
             AND g.principal_type='anyone'
       )
   )
ORDER BY d.updated_at DESC
LIMIT 25;

-- name: UpdateDocumentTitle :exec
UPDATE documents SET title=$1,updated_at=now() WHERE id=$2;

-- name: SearchDocumentsByTitle :many
SELECT DISTINCT d.id,d.title,d.current_version_id,d.owner_type,d.owner_id,d.source,d.drive_file_id,d.drive_resource_key,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,sa.name,'') AS owner_name
FROM documents d
LEFT JOIN users u ON d.owner_type='user' AND u.id=d.owner_id
-- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
LEFT JOIN service_accounts sa -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
  -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type, not a Google credential.
  ON d.owner_type='service_account' AND sa.id=d.owner_id -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type/table, not a Google credential.
WHERE d.title ILIKE '%' || $1 || '%'
  AND (
    (d.owner_type=$3 AND d.owner_id=$2)
    OR EXISTS (
      SELECT 1 FROM resource_grants g
      WHERE g.resource_type='document'
        AND g.resource_id=d.id
        AND g.revoked_at IS NULL
        AND g.principal_type=$3 AND g.principal_id=$2
    )
    OR (
      $4::boolean
      AND EXISTS (
        SELECT 1 FROM resource_grants g
        WHERE g.resource_type='document'
          AND g.resource_id=d.id
          AND g.revoked_at IS NULL
          AND g.principal_type='anyone'
      )
    )
  )
ORDER BY d.title
LIMIT $5;

-- name: DeleteDocument :exec
DELETE FROM documents WHERE id=$1;
