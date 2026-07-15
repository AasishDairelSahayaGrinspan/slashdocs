-- +goose Up
-- Durable records for connected knowledge sources and synchronization runs.
-- Connected knowledge deliberately stores provider-specific metadata in JSONB.
-- The typed API layer owns validation while this table provides durable,
-- indexed lifecycle storage without a migration for every provider field.
CREATE TABLE connected_knowledge_records (
  kind TEXT NOT NULL CHECK (kind IN ('settings','source','sync_run','audit','notification','team')),
  id TEXT NOT NULL,
  repository_id TEXT REFERENCES repositories(id) ON DELETE CASCADE,
  user_id TEXT,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kind, id)
);

CREATE INDEX connected_knowledge_records_repository_idx
  ON connected_knowledge_records(repository_id, kind, created_at DESC);
CREATE INDEX connected_knowledge_records_user_idx
  ON connected_knowledge_records(user_id, kind, created_at DESC)
  WHERE user_id IS NOT NULL;
CREATE INDEX connected_knowledge_records_payload_idx
  ON connected_knowledge_records USING GIN(payload);

CREATE TABLE connected_knowledge_webhook_deliveries (
  provider TEXT NOT NULL,
  delivery_id TEXT NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(provider, delivery_id)
);

CREATE TABLE connected_knowledge_provenance (
  id TEXT PRIMARY KEY,
  repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
  source_id TEXT NOT NULL,
  sync_run_id TEXT NOT NULL,
  document_id TEXT REFERENCES documents(id) ON DELETE CASCADE,
  version_id TEXT REFERENCES versions(id) ON DELETE CASCADE,
  provider_url TEXT NOT NULL DEFAULT '',
  commit_sha TEXT NOT NULL DEFAULT '',
  paths JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX connected_knowledge_provenance_document_idx
  ON connected_knowledge_provenance(document_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS connected_knowledge_provenance;
DROP TABLE IF EXISTS connected_knowledge_webhook_deliveries;
DROP TABLE IF EXISTS connected_knowledge_records;
