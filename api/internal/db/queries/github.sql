-- name: UpsertGitHubConnection :exec
INSERT INTO github_connections(user_id, github_user_id, login, avatar_url, access_token_ciphertext)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(user_id) DO UPDATE SET
  github_user_id=EXCLUDED.github_user_id,
  login=EXCLUDED.login,
  avatar_url=EXCLUDED.avatar_url,
  access_token_ciphertext=EXCLUDED.access_token_ciphertext,
  updated_at=now();

-- name: GetGitHubConnection :one
SELECT user_id, github_user_id, login, avatar_url, access_token_ciphertext, updated_at
FROM github_connections WHERE user_id=$1;

-- name: DeleteGitHubConnection :execrows
DELETE FROM github_connections WHERE user_id=$1;
