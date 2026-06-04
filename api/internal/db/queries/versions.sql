-- name: InsertInitialVersion :exec
INSERT INTO versions(id,document_id,number,html_blob_key,sha256,parent_version_id,created_by_type,created_by_id,change_summary)
VALUES($1,$2,1,$3,$4,NULL,$5,$6,'');

-- name: InsertVersion :exec
INSERT INTO versions(id,document_id,number,html_blob_key,sha256,parent_version_id,created_by_type,created_by_id,change_summary)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: ListVersions :many
SELECT
  v.id,
  v.number,
  v.document_id,
  v.created_by_type,
  v.created_by_id,
  COALESCE(u.email, '') AS created_by_email,
  COALESCE(u.name, sa.name, '') AS created_by_name,
  COALESCE(u.picture_url, '') AS created_by_picture_url,
  v.change_summary,
  v.sha256
FROM versions v
LEFT JOIN users u
  ON v.created_by_type = 'user' AND v.created_by_id = u.id
LEFT JOIN service_accounts sa
  -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type, not a Google credential.
  ON v.created_by_type = 'service_account' AND v.created_by_id = sa.id
WHERE v.document_id=$1
ORDER BY v.number;

-- name: GetVersion :one
SELECT
  v.id,
  v.number,
  v.document_id,
  v.created_by_type,
  v.created_by_id,
  COALESCE(u.email, '') AS created_by_email,
  COALESCE(u.name, sa.name, '') AS created_by_name,
  COALESCE(u.picture_url, '') AS created_by_picture_url,
  v.change_summary,
  v.sha256
FROM versions v
LEFT JOIN users u
  ON v.created_by_type = 'user' AND v.created_by_id = u.id
LEFT JOIN service_accounts sa
  -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type, not a Google credential.
  ON v.created_by_type = 'service_account' AND v.created_by_id = sa.id
WHERE v.id=$1;

-- name: GetVersionBlobKey :one
SELECT html_blob_key FROM versions WHERE id=$1;
