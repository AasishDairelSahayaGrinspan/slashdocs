-- +goose Up
-- One row per repository a user approved a document to read with their
-- connected GitHub account. GitHub owner and repository names are
-- case-insensitive, so both are stored lowercased and compared that way.
CREATE TABLE github_repo_consents (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  owner TEXT NOT NULL,
  repo TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, document_id, owner, repo),
  CHECK (owner = lower(owner) AND char_length(owner) BETWEEN 1 AND 39),
  CHECK (repo = lower(repo) AND char_length(repo) BETWEEN 1 AND 100)
);

CREATE INDEX github_repo_consents_document_idx ON github_repo_consents(document_id);

-- +goose Down
DROP TABLE IF EXISTS github_repo_consents;
