-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_users_name_trgm
ON users USING gin (lower(name) gin_trgm_ops)
WHERE google_sub IS NOT NULL AND name <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_users_name_trgm;
