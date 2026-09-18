-- name: RevokeSession :exec
INSERT INTO revoked_sessions(token_hash,expires_at) VALUES($1,$2)
ON CONFLICT(token_hash) DO UPDATE SET expires_at=EXCLUDED.expires_at, revoked_at=now();

-- name: DeleteExpiredRevokedSessions :exec
DELETE FROM revoked_sessions WHERE expires_at<=now();

-- name: SessionRevoked :one
SELECT EXISTS(SELECT 1 FROM revoked_sessions WHERE token_hash=$1)::bool;
