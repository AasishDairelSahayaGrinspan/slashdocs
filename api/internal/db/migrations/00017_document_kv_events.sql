-- +goose Up
CREATE TABLE document_kv_events (
  document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL,
  changes JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by_type TEXT NOT NULL DEFAULT '',
  updated_by_id TEXT NOT NULL DEFAULT '',
  updated_by_email TEXT NOT NULL DEFAULT '',
  updated_by_name TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (document_id, revision)
);

-- +goose Down
DROP TABLE IF EXISTS document_kv_events;
