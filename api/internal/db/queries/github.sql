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

-- name: ListGitHubRepoConsents :many
-- Repository consents are written and compared lowercased; callers normalize
-- owner and repo before reaching these queries.
SELECT owner, repo, created_at FROM github_repo_consents
WHERE user_id= @user_id AND document_id= @document_id
ORDER BY owner, repo;

-- name: HasGitHubRepoConsent :one
SELECT EXISTS(
  SELECT 1 FROM github_repo_consents
  WHERE user_id= @user_id AND document_id= @document_id AND owner= @owner AND repo= @repo
);

-- name: GrantGitHubRepoConsent :one
INSERT INTO github_repo_consents(user_id, document_id, owner, repo)
VALUES(@user_id, @document_id, @owner, @repo)
-- A repeated approval keeps the original row, so created_at stays the moment
-- the user first approved this repository for this document.
ON CONFLICT(user_id, document_id, owner, repo) DO UPDATE SET owner=EXCLUDED.owner
RETURNING owner, repo, created_at;

-- name: RevokeGitHubRepoConsent :exec
DELETE FROM github_repo_consents
WHERE user_id= @user_id AND document_id= @document_id AND owner= @owner AND repo= @repo;

-- name: DeleteGitHubRepoConsents :exec
DELETE FROM github_repo_consents WHERE user_id= @user_id;
