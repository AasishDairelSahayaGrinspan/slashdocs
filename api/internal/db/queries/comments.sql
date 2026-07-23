-- name: InsertComment :exec
INSERT INTO comments(id,document_id,created_on_version_id,author_type,author_id,author_email,author_name,body,selected_text,anchor_json,status,root_comment_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'open',$1);

-- name: InsertCommentReply :exec
INSERT INTO comments(id,document_id,created_on_version_id,author_type,author_id,author_email,author_name,body,selected_text,anchor_json,status,parent_comment_id,root_comment_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,'','{}'::jsonb,'open',$9,$10);

-- name: ListComments :many
SELECT id,document_id,created_on_version_id,author_type,author_id,author_email,author_name,body,selected_text,anchor_json,status,created_at,resolved_at,parent_comment_id,root_comment_id
FROM comments WHERE document_id=$1 AND ($2='' OR status=$2) AND ($3='' OR created_on_version_id=$3) ORDER BY created_at;

-- name: UpdateCommentBody :exec
UPDATE comments SET body=$1 WHERE id=$2;

-- name: UpdateCommentStatus :exec
UPDATE comments SET status=$1, resolved_at=CASE WHEN $1='resolved' THEN now() ELSE NULL END WHERE id=$2;

-- name: DeleteComment :exec
DELETE FROM comments WHERE id=$1;

-- name: GetComment :one
SELECT id, document_id, created_on_version_id, author_type, author_id, author_email, author_name, body, selected_text, anchor_json, status, created_at, resolved_at, parent_comment_id, root_comment_id
FROM comments
WHERE id = $1;
