-- +goose Up
-- Provider credentials belong to a user profile, while knowledge sources may
-- optionally reference one of those accounts. Only authenticated ciphertext is
-- stored here; OAuth access and refresh tokens must never be written directly.
CREATE TABLE connected_accounts (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL CHECK (provider IN ('github','slack','devrev','google_drive')),
  external_account_id TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  workspace_name TEXT NOT NULL DEFAULT '',
  scopes JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(scopes) = 'array'),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','demo','needs_reconnect','disconnected')),
  credential_ciphertext BYTEA,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT connected_accounts_identity_key UNIQUE(user_id, provider, workspace_id, external_account_id),
  CHECK (status <> 'active' OR credential_ciphertext IS NOT NULL)
);

CREATE INDEX connected_accounts_user_idx
  ON connected_accounts(user_id, created_at DESC);

ALTER TABLE connected_knowledge_records
  ADD COLUMN connection_id TEXT REFERENCES connected_accounts(id) ON DELETE SET NULL;

CREATE INDEX connected_knowledge_records_connection_idx
  ON connected_knowledge_records(connection_id)
  WHERE connection_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS connected_knowledge_records_connection_idx;
ALTER TABLE connected_knowledge_records DROP COLUMN IF EXISTS connection_id;
DROP TABLE IF EXISTS connected_accounts;
