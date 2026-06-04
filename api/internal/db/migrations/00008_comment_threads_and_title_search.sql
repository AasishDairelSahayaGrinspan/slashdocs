-- +goose Up
ALTER TABLE comments
  ADD COLUMN parent_comment_id TEXT REFERENCES comments(id) ON DELETE CASCADE,
  ADD COLUMN root_comment_id TEXT REFERENCES comments(id) ON DELETE CASCADE;

UPDATE comments SET root_comment_id = id WHERE root_comment_id IS NULL;

ALTER TABLE comments
  ALTER COLUMN root_comment_id SET NOT NULL;

CREATE INDEX idx_comments_document_root_created
  ON comments(document_id, root_comment_id, created_at);

CREATE INDEX idx_comments_parent_created
  ON comments(parent_comment_id, created_at)
  WHERE parent_comment_id IS NOT NULL;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_documents_title_trgm
  ON documents USING GIN (title gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_documents_title_trgm;
DROP INDEX IF EXISTS idx_comments_parent_created;
DROP INDEX IF EXISTS idx_comments_document_root_created;
ALTER TABLE comments DROP COLUMN IF EXISTS root_comment_id;
ALTER TABLE comments DROP COLUMN IF EXISTS parent_comment_id;
