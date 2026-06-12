-- name: UpsertPin :exec
INSERT INTO pins(principal_type, principal_id, resource_type, resource_id)
VALUES($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: DeletePin :exec
DELETE FROM pins
WHERE principal_type=$1 AND principal_id=$2 AND resource_type=$3 AND resource_id=$4;

-- name: ListPins :many
SELECT resource_type, resource_id, pinned_at
FROM pins
WHERE principal_type=$1 AND principal_id=$2
ORDER BY pinned_at DESC;
