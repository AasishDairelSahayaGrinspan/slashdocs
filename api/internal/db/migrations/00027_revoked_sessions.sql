-- +goose Up
-- Session cookies are stateless signed tokens, so signing out has to record
-- the tokens that must no longer be accepted. Rows are keyed by the SHA-256 of
-- the token value (never the token itself) and carry the token's own expiry,
-- after which the signature stops verifying anyway and the row can be reaped.
CREATE TABLE revoked_sessions (
  token_hash TEXT PRIMARY KEY,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX revoked_sessions_expires_at ON revoked_sessions(expires_at);

-- +goose Down
DROP TABLE IF EXISTS revoked_sessions;
