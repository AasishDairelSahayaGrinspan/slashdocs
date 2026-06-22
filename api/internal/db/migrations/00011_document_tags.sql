-- +goose Up
-- +goose NO TRANSACTION
-- Adding a column with a constant DEFAULT is non-blocking in Postgres 11+ (no
-- table rewrite). The GIN index is created CONCURRENTLY so it does not lock
-- the table, making this migration safe to apply against a live instance.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_documents_tags ON documents USING GIN(tags);

-- +goose Down
-- +goose NO TRANSACTION
DROP INDEX CONCURRENTLY IF EXISTS idx_documents_tags;
ALTER TABLE documents DROP COLUMN IF EXISTS tags;
