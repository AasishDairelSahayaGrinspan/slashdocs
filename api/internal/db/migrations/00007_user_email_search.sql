-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_users_email_trgm
ON users USING gin (lower(email) gin_trgm_ops)
WHERE google_sub IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_email_trgm;
