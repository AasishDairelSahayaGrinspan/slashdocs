-- name: LockDocumentForKV :one
SELECT id FROM documents WHERE id=$1 FOR UPDATE;

-- name: ListDocumentKV :many
SELECT key,value,revision,updated_at,updated_by_type,updated_by_id,updated_by_email,updated_by_name
FROM document_kv
WHERE document_id=$1 AND deleted_at IS NULL
ORDER BY key;

-- name: GetDocumentKVRevision :one
SELECT COALESCE(MAX(revision), 0)::bigint FROM document_kv WHERE document_id=$1;

-- name: InsertDocumentKVEvent :exec
INSERT INTO document_kv_events(document_id,revision,changes,updated_by_type,updated_by_id,updated_by_email,updated_by_name)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: NotifyDocumentKVEvent :exec
SELECT pg_notify('aidocs_kv_events', sqlc.arg(document_id)::text);

-- name: ListDocumentKVEventsAfter :many
SELECT document_id,revision,changes,updated_at,updated_by_type,updated_by_id,updated_by_email,updated_by_name
FROM document_kv_events
WHERE document_id=$1 AND revision>$2
ORDER BY revision
LIMIT $3;

-- name: UpsertDocumentKV :exec
INSERT INTO document_kv(document_id,key,value,revision,updated_by_type,updated_by_id,updated_by_email,updated_by_name,deleted_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULL)
ON CONFLICT(document_id,key) DO UPDATE
SET value=EXCLUDED.value,
    revision=EXCLUDED.revision,
    updated_at=now(),
    updated_by_type=EXCLUDED.updated_by_type,
    updated_by_id=EXCLUDED.updated_by_id,
    updated_by_email=EXCLUDED.updated_by_email,
    updated_by_name=EXCLUDED.updated_by_name,
    deleted_at=NULL;

-- name: DeleteDocumentKV :exec
INSERT INTO document_kv(document_id,key,value,revision,updated_by_type,updated_by_id,updated_by_email,updated_by_name,deleted_at)
VALUES($1,$2,'null'::jsonb,$3,$4,$5,$6,$7,now())
ON CONFLICT(document_id,key) DO UPDATE
SET value='null'::jsonb,
    revision=EXCLUDED.revision,
    updated_at=now(),
    updated_by_type=EXCLUDED.updated_by_type,
    updated_by_id=EXCLUDED.updated_by_id,
    updated_by_email=EXCLUDED.updated_by_email,
    updated_by_name=EXCLUDED.updated_by_name,
    deleted_at=now();
