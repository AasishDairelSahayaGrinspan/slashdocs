// Package connections stores the external accounts that individual aidocs
// users authorize for connected knowledge sources.
package connections

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("connection not found")
	ErrConflict = errors.New("connection conflict")
	ErrInvalid  = errors.New("invalid connection")
)

const (
	ProviderGitHub      = "github"
	ProviderSlack       = "slack"
	ProviderDevRev      = "devrev"
	ProviderGoogleDrive = "google_drive"
)

var providers = map[string]bool{
	ProviderGitHub:      true,
	ProviderSlack:       true,
	ProviderDevRev:      true,
	ProviderGoogleDrive: true,
}

type Status string

const (
	StatusActive         Status = "active"
	StatusNeedsReconnect Status = "needs_reconnect"
	StatusDisconnected   Status = "disconnected"
	// StatusDemo is reserved for the in-memory development server. Production
	// OAuth and token connections are stored as active or needs_reconnect.
	StatusDemo Status = "demo"
)

// Account is safe to serialize: the encrypted credential is deliberately
// excluded from JSON so neither API responses nor audit payloads can expose it.
type Account struct {
	ID                   string     `json:"id"`
	UserID               string     `json:"user_id"`
	Provider             string     `json:"provider"`
	ExternalAccountID    string     `json:"external_account_id"`
	DisplayName          string     `json:"display_name"`
	WorkspaceID          string     `json:"workspace_id,omitempty"`
	WorkspaceName        string     `json:"workspace_name,omitempty"`
	Scopes               []string   `json:"scopes"`
	Status               Status     `json:"status"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CredentialCiphertext []byte     `json:"-"`
}

// Credentials is the plaintext credential payload passed only to Cipher.
// Store implementations persist Account.CredentialCiphertext and never this
// structure directly.
type Credentials struct {
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
	TokenType    string `json:"-"`
}

// Store is the persistence boundary for per-user external accounts.
type Store interface {
	Put(context.Context, Account) (Account, error)
	Get(context.Context, string) (Account, error)
	ListByUser(context.Context, string) ([]Account, error)
	// RefreshCredentials atomically replaces credentials only if the account is
	// still the same active record that initiated the provider refresh. This
	// makes disconnect/reconnect win over an older in-flight refresh.
	RefreshCredentials(context.Context, Account, time.Time) (Account, error)
	Disconnect(context.Context, string, string) (Account, error)
}

func normalize(account Account) (Account, error) {
	account.ID = strings.TrimSpace(account.ID)
	account.UserID = strings.TrimSpace(account.UserID)
	account.Provider = strings.ToLower(strings.TrimSpace(account.Provider))
	account.ExternalAccountID = strings.TrimSpace(account.ExternalAccountID)
	account.DisplayName = strings.TrimSpace(account.DisplayName)
	account.WorkspaceID = strings.TrimSpace(account.WorkspaceID)
	account.WorkspaceName = strings.TrimSpace(account.WorkspaceName)
	if account.Status == "" {
		account.Status = StatusActive
	}
	if account.ID == "" || account.UserID == "" || account.ExternalAccountID == "" || !providers[account.Provider] {
		return Account{}, ErrInvalid
	}
	switch account.Status {
	case StatusActive, StatusNeedsReconnect, StatusDisconnected, StatusDemo:
	default:
		return Account{}, ErrInvalid
	}
	if account.Status == StatusActive && len(account.CredentialCiphertext) == 0 {
		return Account{}, ErrInvalid
	}
	if account.Status == StatusDisconnected {
		account.CredentialCiphertext = nil
		account.ExpiresAt = nil
	}
	account.Scopes = normalizeScopes(account.Scopes)
	if account.ExpiresAt != nil {
		expires := account.ExpiresAt.UTC()
		account.ExpiresAt = &expires
	}
	return account, nil
}

func normalizeScopes(scopes []string) []string {
	seen := make(map[string]bool, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func clone(account Account) Account {
	account.Scopes = append([]string{}, account.Scopes...)
	account.CredentialCiphertext = append([]byte(nil), account.CredentialCiphertext...)
	if account.ExpiresAt != nil {
		expires := *account.ExpiresAt
		account.ExpiresAt = &expires
	}
	return account
}

func sameIdentity(a, b Account) bool {
	return a.UserID == b.UserID && a.Provider == b.Provider && a.WorkspaceID == b.WorkspaceID && a.ExternalAccountID == b.ExternalAccountID
}
