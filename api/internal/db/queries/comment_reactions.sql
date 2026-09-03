-- name: ListCommentReactions :many
-- Ordering by created_at before emoji lets the JSON layer aggregate in
-- first-reaction order, so the emoji a thread used first stays leftmost.
SELECT comment_id, principal_type, principal_id, emoji
FROM comment_reactions
WHERE comment_id = ANY(@comment_ids::text[])
ORDER BY comment_id, created_at, emoji;

-- name: PutCommentReaction :exec
INSERT INTO comment_reactions(comment_id, principal_type, principal_id, emoji)
VALUES(@comment_id, @principal_type, @principal_id, @emoji)
ON CONFLICT DO NOTHING;

-- name: DeleteCommentReaction :exec
DELETE FROM comment_reactions
WHERE comment_id = @comment_id
  AND principal_type = @principal_type
  AND principal_id = @principal_id
  AND emoji = @emoji;
