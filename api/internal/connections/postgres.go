package connections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const accountIdentityConstraint = "connected_accounts_identity_key"

// Postgres stores encrypted connected-account credentials. It never receives
// or persists plaintext provider tokens.
type Postgres struct {
	db *pgxpool.Pool
}

func NewPostgres(db *pgxpool.Pool) *Postgres { return &Postgres{db: db} }

func (s *Postgres) Put(ctx context.Context, account Account) (Account, error) {
	account, err := normalize(account)
	if err != nil {
		return Account{}, err
	}
	scopes, err := json.Marshal(account.Scopes)
	if err != nil {
		return Account{}, fmt.Errorf("encode connection scopes: %w", err)
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO connected_accounts(
			id, user_id, provider, external_account_id, display_name,
			workspace_id, workspace_name, scopes, status,
			credential_ciphertext, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11)
		ON CONFLICT(id) DO UPDATE SET
			display_name=EXCLUDED.display_name,
			workspace_name=EXCLUDED.workspace_name,
			scopes=EXCLUDED.scopes,
			status=EXCLUDED.status,
			credential_ciphertext=EXCLUDED.credential_ciphertext,
			expires_at=EXCLUDED.expires_at,
			updated_at=now()
		WHERE connected_accounts.user_id=EXCLUDED.user_id
		  AND connected_accounts.provider=EXCLUDED.provider
		  AND connected_accounts.workspace_id=EXCLUDED.workspace_id
		  AND connected_accounts.external_account_id=EXCLUDED.external_account_id
		RETURNING id,user_id,provider,external_account_id,display_name,
		          workspace_id,workspace_name,scopes,status,
		          credential_ciphertext,expires_at,created_at,updated_at
	`, account.ID, account.UserID, account.Provider, account.ExternalAccountID, account.DisplayName,
		account.WorkspaceID, account.WorkspaceName, scopes, account.Status,
		account.CredentialCiphertext, account.ExpiresAt)
	stored, err := scanAccount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrConflict
	}
	if isIdentityConflict(err) {
		return Account{}, ErrConflict
	}
	return stored, err
}

func (s *Postgres) Get(ctx context.Context, id string) (Account, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Account{}, ErrInvalid
	}
	account, err := scanAccount(s.db.QueryRow(ctx, `
		SELECT id,user_id,provider,external_account_id,display_name,
		       workspace_id,workspace_name,scopes,status,
		       credential_ciphertext,expires_at,created_at,updated_at
		FROM connected_accounts WHERE id=$1
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return account, err
}

func (s *Postgres) ListByUser(ctx context.Context, userID string) ([]Account, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrInvalid
	}
	rows, err := s.db.Query(ctx, `
		SELECT id,user_id,provider,external_account_id,display_name,
		       workspace_id,workspace_name,scopes,status,
		       credential_ciphertext,expires_at,created_at,updated_at
		FROM connected_accounts
		WHERE user_id=$1
		ORDER BY created_at DESC, id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]Account, 0)
	for rows.Next() {
		account, scanErr := scanAccount(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *Postgres) RefreshCredentials(ctx context.Context, account Account, expectedUpdatedAt time.Time) (Account, error) {
	account, err := normalize(account)
	if err != nil {
		return Account{}, err
	}
	refreshed, err := scanAccount(s.db.QueryRow(ctx, `
		UPDATE connected_accounts
		SET credential_ciphertext=$4, expires_at=$5, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND status=$3 AND updated_at=$6
		  AND provider=$7 AND workspace_id=$8 AND external_account_id=$9
		RETURNING id,user_id,provider,external_account_id,display_name,
		          workspace_id,workspace_name,scopes,status,
		          credential_ciphertext,expires_at,created_at,updated_at
	`, account.ID, account.UserID, StatusActive, account.CredentialCiphertext,
		account.ExpiresAt, expectedUpdatedAt, account.Provider, account.WorkspaceID, account.ExternalAccountID))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := s.Get(ctx, account.ID); errors.Is(getErr, ErrNotFound) {
			return Account{}, ErrNotFound
		}
		return Account{}, ErrConflict
	}
	return refreshed, err
}

func (s *Postgres) Disconnect(ctx context.Context, userID, id string) (Account, error) {
	userID, id = strings.TrimSpace(userID), strings.TrimSpace(id)
	if userID == "" || id == "" {
		return Account{}, ErrInvalid
	}
	account, err := scanAccount(s.db.QueryRow(ctx, `
		UPDATE connected_accounts
		SET status=$3, credential_ciphertext=NULL, expires_at=NULL, updated_at=now()
		WHERE user_id=$1 AND id=$2
		RETURNING id,user_id,provider,external_account_id,display_name,
		          workspace_id,workspace_name,scopes,status,
		          credential_ciphertext,expires_at,created_at,updated_at
	`, userID, id, StatusDisconnected))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return account, err
}

type accountScanner interface {
	Scan(...any) error
}

func scanAccount(row accountScanner) (Account, error) {
	var account Account
	var scopes []byte
	if err := row.Scan(
		&account.ID, &account.UserID, &account.Provider, &account.ExternalAccountID, &account.DisplayName,
		&account.WorkspaceID, &account.WorkspaceName, &scopes, &account.Status,
		&account.CredentialCiphertext, &account.ExpiresAt, &account.CreatedAt, &account.UpdatedAt,
	); err != nil {
		return Account{}, err
	}
	if err := json.Unmarshal(scopes, &account.Scopes); err != nil {
		return Account{}, fmt.Errorf("decode connection scopes: %w", err)
	}
	return clone(account), nil
}

func isIdentityConflict(err error) bool {
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) && postgresErr.Code == "23505" && postgresErr.ConstraintName == accountIdentityConstraint
}

var _ Store = (*Postgres)(nil)
