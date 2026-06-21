-- +goose Up
ALTER TABLE documents
  ADD COLUMN source TEXT NOT NULL DEFAULT 'native',
  ADD COLUMN drive_file_id TEXT,
  ADD COLUMN drive_resource_key TEXT;

-- One aidocs document per Drive file, so re-opening reuses its comments.
CREATE UNIQUE INDEX documents_drive_file_id_key
  ON documents (drive_file_id) WHERE drive_file_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS documents_drive_file_id_key;

ALTER TABLE documents
  DROP COLUMN IF EXISTS drive_resource_key,
  DROP COLUMN IF EXISTS drive_file_id,
  DROP COLUMN IF EXISTS source;
