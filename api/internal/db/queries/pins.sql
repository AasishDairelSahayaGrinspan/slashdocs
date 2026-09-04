-- name: ListPins :many
SELECT resource_type, resource_id FROM pins WHERE principal_type= @principal_type AND principal_id= @principal_id;

-- name: PutPin :exec
INSERT INTO pins(principal_type,principal_id,resource_type,resource_id)
VALUES(@principal_type,@principal_id,@resource_type,@resource_id)
ON CONFLICT DO NOTHING;

-- name: DeletePin :exec
DELETE FROM pins WHERE principal_type= @principal_type AND principal_id= @principal_id AND resource_type= @resource_type AND resource_id= @resource_id;
