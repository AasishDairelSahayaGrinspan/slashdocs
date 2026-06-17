-- name: CreateList :one
INSERT INTO lists (id, name, owner_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetList :one
SELECT * FROM lists WHERE id = $1;

-- name: ListListsByOwner :many
SELECT * FROM lists WHERE owner_id = $1 ORDER BY created_at DESC;

-- name: UpdateList :one
UPDATE lists SET name = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteList :exec
DELETE FROM lists WHERE id = $1;

-- name: AddDocumentToList :exec
INSERT INTO list_items (list_id, document_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveDocumentFromList :exec
DELETE FROM list_items WHERE list_id = $1 AND document_id = $2;

-- name: ListDocumentsByList :many
SELECT d.id, d.title, d.current_version_id, d.owner_type, d.owner_id,
       COALESCE(u.email,'') AS owner_email,
       COALESCE(u.name,'') AS owner_name
FROM documents d
JOIN list_items li ON li.document_id = d.id
LEFT JOIN users u ON d.owner_type='user' AND u.id = d.owner_id
WHERE li.list_id = $1
ORDER BY li.added_at DESC;

-- name: GetListItemCount :one
SELECT COUNT(*) FROM list_items WHERE list_id = $1;
