-- +goose Up
CREATE TABLE github_connections (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  github_user_id BIGINT NOT NULL,
  login TEXT NOT NULL,
  avatar_url TEXT NOT NULL DEFAULT '',
  access_token_ciphertext BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE document_labels (
  document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  created_by_type TEXT NOT NULL,
  created_by_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (document_id, label),
  CHECK (label = lower(label)),
  CHECK (char_length(label) BETWEEN 1 AND 48)
);

CREATE INDEX document_labels_label_idx ON document_labels(label);

-- +goose Down
DROP TABLE IF EXISTS document_labels;
DROP TABLE IF EXISTS github_connections;
