-- +goose Up
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_owner_id_fkey;

ALTER TABLE documents
  ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'user';

CREATE INDEX idx_documents_owner
  ON documents(owner_type, owner_id);

-- +goose Down
DROP INDEX IF EXISTS idx_documents_owner;

ALTER TABLE documents DROP COLUMN IF EXISTS owner_type;

ALTER TABLE documents
  ADD CONSTRAINT documents_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id);
