package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	oauthStateKind           = "state"
	oauthCodeKind            = "cli_code"
	oauthConnectionStateKind = "connection_state"
)

type PostgresStateStore struct{ db *pgxpool.Pool }

func NewPostgresStateStore(db *pgxpool.Pool) *PostgresStateStore {
	return &PostgresStateStore{db: db}
}

func (s *PostgresStateStore) PutState(ctx context.Context, id string, st LoginState) error {
	if st.ExpiresAt.IsZero() {
		st.ExpiresAt = time.Now().Add(defaultStateTTL)
	}
	return s.put(ctx, oauthStateKind, id, st)
}

func (s *PostgresStateStore) TakeState(ctx context.Context, id string) (LoginState, bool, error) {
	return s.take(ctx, oauthStateKind, id)
}

func (s *PostgresStateStore) PutCode(ctx context.Context, code string, st LoginState) error {
	st.ExpiresAt = time.Now().Add(codeTTL)
	return s.put(ctx, oauthCodeKind, code, st)
}

func (s *PostgresStateStore) TakeCode(ctx context.Context, code string) (LoginState, bool, error) {
	return s.take(ctx, oauthCodeKind, code)
}

func (s *PostgresStateStore) PutConnectionState(ctx context.Context, id string, st ConnectionState) error {
	if st.ExpiresAt.IsZero() {
		st.ExpiresAt = time.Now().Add(defaultStateTTL)
	}
	return s.putJSON(ctx, oauthConnectionStateKind, id, st.ExpiresAt, st)
}

func (s *PostgresStateStore) TakeConnectionState(ctx context.Context, id string) (ConnectionState, bool, error) {
	var st ConnectionState
	ok, err := s.takeJSON(ctx, oauthConnectionStateKind, id, &st)
	return st, ok, err
}

func (s *PostgresStateStore) put(ctx context.Context, kind, id string, st LoginState) error {
	return s.putJSON(ctx, kind, id, st.ExpiresAt, st)
}

func (s *PostgresStateStore) putJSON(ctx context.Context, kind, id string, expiresAt time.Time, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO oauth_states(id, kind, state_json, expires_at)
		VALUES($1, $2, $3, $4)
		ON CONFLICT(kind, id) DO UPDATE SET
		  state_json=EXCLUDED.state_json,
		  expires_at=EXCLUDED.expires_at,
		  created_at=now()
	`, id, kind, b, expiresAt)
	return err
}

func (s *PostgresStateStore) take(ctx context.Context, kind, id string) (LoginState, bool, error) {
	var st LoginState
	ok, err := s.takeJSON(ctx, kind, id, &st)
	return st, ok, err
}

func (s *PostgresStateStore) takeJSON(ctx context.Context, kind, id string, out any) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		// Rollback returns pgx.ErrTxClosed after the successful commit below.
		_ = tx.Rollback(ctx)
	}()

	var raw []byte
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `
		DELETE FROM oauth_states
		WHERE kind=$1 AND id=$2
		RETURNING state_json, expires_at
	`, kind, id).Scan(&raw, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Genuinely not found (or already consumed): not an error.
		return false, nil
	}
	if err != nil {
		// A real DB error must be distinguishable from "not found" so callers
		// can alert on OAuth/CLI exchange failures.
		return false, err
	}
	// Commit the delete regardless of expiry so a consumed/expired row is not
	// left behind for CleanupExpired to reap later.
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if time.Now().After(expiresAt) {
		return false, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PostgresStateStore) CleanupExpired(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM oauth_states WHERE expires_at < now()`)
	return err
}

var _ LoginStateStore = (*PostgresStateStore)(nil)
var _ ConnectionStateStore = (*PostgresStateStore)(nil)
