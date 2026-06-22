-- +goose Up
ALTER TABLE documents ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX idx_documents_tags ON documents USING GIN(tags);

-- +goose Down
DROP INDEX IF EXISTS idx_documents_tags;
ALTER TABLE documents DROP COLUMN tags;
