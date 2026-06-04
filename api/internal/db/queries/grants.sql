-- name: UpsertResourceGrant :one
INSERT INTO resource_grants(id,resource_type,resource_id,principal_type,principal_id,role,granted_by_user_id)
VALUES($1,'document',$2,$3,$4,$5,$6)
ON CONFLICT(resource_type,resource_id,principal_type,principal_id) DO UPDATE SET role=EXCLUDED.role, revoked_at=NULL
RETURNING id;

-- name: ListGrants :many
SELECT
  g.id,
  g.principal_type,
  g.principal_id,
  COALESCE(u.email, '') AS principal_email,
  COALESCE(u.name, sa.name, '') AS principal_name,
  COALESCE(u.picture_url, '') AS principal_picture_url,
  g.role,
  g.granted_by_user_id
FROM resource_grants g
LEFT JOIN users u
  ON g.principal_type = 'user' AND g.principal_id = u.id
LEFT JOIN service_accounts sa
  -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type, not a Google credential.
  ON g.principal_type = 'service_account' AND g.principal_id = sa.id
WHERE g.resource_type='document' AND g.resource_id=$1 AND g.revoked_at IS NULL;

-- name: UpdateGrantRole :one
WITH updated AS (
  UPDATE resource_grants
  SET role=$1
  WHERE resource_grants.id=$2 AND resource_type='document' AND resource_id=$3 AND revoked_at IS NULL
  RETURNING *
)
SELECT
  g.id,
  g.resource_id,
  g.principal_type,
  g.principal_id,
  COALESCE(u.email, '') AS principal_email,
  COALESCE(u.name, sa.name, '') AS principal_name,
  COALESCE(u.picture_url, '') AS principal_picture_url,
  g.role,
  g.granted_by_user_id
FROM updated g
LEFT JOIN users u
  ON g.principal_type = 'user' AND g.principal_id = u.id
LEFT JOIN service_accounts sa
  -- nosemgrep: detected-google-gcm-service-account - aidocs-native principal type, not a Google credential.
  ON g.principal_type = 'service_account' AND g.principal_id = sa.id;

-- name: GetGrant :one
SELECT id,resource_id,principal_type,principal_id,role,granted_by_user_id FROM resource_grants WHERE id=$1 AND resource_type='document' AND resource_id=$2 AND revoked_at IS NULL;

-- name: DeleteGrant :one
UPDATE resource_grants SET revoked_at=now() WHERE id=$1 AND resource_type='document' AND resource_id=$2 AND revoked_at IS NULL
RETURNING id;
