package connections

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory is the development implementation of Store.
type Memory struct {
	mu       sync.Mutex
	accounts map[string]Account
	now      func() time.Time
}

func NewMemory() *Memory {
	return &Memory{accounts: map[string]Account{}, now: time.Now}
}

func (m *Memory) Put(_ context.Context, account Account) (Account, error) {
	account, err := normalize(account)
	if err != nil {
		return Account{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.accounts[account.ID]; ok {
		if !sameIdentity(existing, account) {
			return Account{}, ErrConflict
		}
		account.CreatedAt = existing.CreatedAt
	} else {
		for id, existing := range m.accounts {
			if id != account.ID && sameIdentity(existing, account) {
				return Account{}, ErrConflict
			}
		}
		account.CreatedAt = m.now().UTC()
	}
	account.UpdatedAt = m.now().UTC()
	m.accounts[account.ID] = clone(account)
	return clone(account), nil
}

func (m *Memory) Get(_ context.Context, id string) (Account, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Account{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	account, ok := m.accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	return clone(account), nil
}

func (m *Memory) ListByUser(_ context.Context, userID string) ([]Account, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Account, 0)
	for _, account := range m.accounts {
		if account.UserID == userID {
			out = append(out, clone(account))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (m *Memory) RefreshCredentials(_ context.Context, account Account, expectedUpdatedAt time.Time) (Account, error) {
	account, err := normalize(account)
	if err != nil {
		return Account{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.accounts[account.ID]
	if !ok || existing.UserID != account.UserID {
		return Account{}, ErrNotFound
	}
	if existing.Status != StatusActive || !sameIdentity(existing, account) || !existing.UpdatedAt.Equal(expectedUpdatedAt) {
		return Account{}, ErrConflict
	}
	existing.CredentialCiphertext = append([]byte(nil), account.CredentialCiphertext...)
	existing.ExpiresAt = account.ExpiresAt
	existing.UpdatedAt = m.now().UTC()
	m.accounts[account.ID] = clone(existing)
	return clone(existing), nil
}

func (m *Memory) Disconnect(_ context.Context, userID, id string) (Account, error) {
	userID, id = strings.TrimSpace(userID), strings.TrimSpace(id)
	if userID == "" || id == "" {
		return Account{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	account, ok := m.accounts[id]
	if !ok || account.UserID != userID {
		// Deliberately hide whether another user's connection exists.
		return Account{}, ErrNotFound
	}
	account.Status = StatusDisconnected
	account.CredentialCiphertext = nil
	account.ExpiresAt = nil
	account.UpdatedAt = m.now().UTC()
	m.accounts[id] = clone(account)
	return clone(account), nil
}

var _ Store = (*Memory)(nil)
