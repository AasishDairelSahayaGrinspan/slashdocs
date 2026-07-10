-- +goose Up
CREATE TABLE document_kv (
  document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  key TEXT NOT NULL,
  value JSONB NOT NULL,
  revision BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by_type TEXT NOT NULL DEFAULT '',
  updated_by_id TEXT NOT NULL DEFAULT '',
  updated_by_email TEXT NOT NULL DEFAULT '',
  updated_by_name TEXT NOT NULL DEFAULT '',
  deleted_at TIMESTAMPTZ,
  PRIMARY KEY (document_id, key)
);

CREATE INDEX document_kv_document_revision_idx ON document_kv(document_id, revision DESC);

-- +goose Down
DROP INDEX IF EXISTS document_kv_document_revision_idx;
DROP TABLE IF EXISTS document_kv;
