package connections

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
)

const credentialEnvelopeVersion byte = 1

// Cipher encrypts provider credentials with AES-256-GCM. The connection's
// immutable ownership fields are authenticated as additional data, preventing
// a ciphertext from being copied to another user or provider account.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher constructs an AES-256-GCM credential cipher. Requiring a 32-byte
// key keeps configuration mistakes from silently selecting a weaker key size.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: credential encryption key must be 32 bytes", ErrInvalid)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts credentials for account. The returned envelope contains only
// a format version, random nonce, and authenticated ciphertext.
func (c *Cipher) Seal(account Account, credentials Credentials) ([]byte, error) {
	if c == nil || c.aead == nil || account.ID == "" || account.UserID == "" || account.Provider == "" || credentials.AccessToken == "" {
		return nil, ErrInvalid
	}
	plaintext, err := json.Marshal(credentialJSON{
		AccessToken:  credentials.AccessToken,
		RefreshToken: credentials.RefreshToken,
		TokenType:    credentials.TokenType,
	})
	if err != nil {
		return nil, fmt.Errorf("encode credentials: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	out := make([]byte, 1, 1+len(nonce)+len(plaintext)+c.aead.Overhead())
	out[0] = credentialEnvelopeVersion
	out = append(out, nonce...)
	out = c.aead.Seal(out, nonce, plaintext, credentialAAD(account))
	return out, nil
}

// Open decrypts a credential envelope only when it belongs to account.
func (c *Cipher) Open(account Account, envelope []byte) (Credentials, error) {
	if c == nil || c.aead == nil || account.ID == "" || account.UserID == "" || account.Provider == "" {
		return Credentials{}, ErrInvalid
	}
	nonceSize := c.aead.NonceSize()
	if len(envelope) < 1+nonceSize+c.aead.Overhead() || envelope[0] != credentialEnvelopeVersion {
		return Credentials{}, ErrInvalid
	}
	nonce := envelope[1 : 1+nonceSize]
	ciphertext := envelope[1+nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, credentialAAD(account))
	if err != nil {
		return Credentials{}, fmt.Errorf("decrypt credentials: %w", ErrInvalid)
	}
	var decoded credentialJSON
	if err := json.Unmarshal(plaintext, &decoded); err != nil || decoded.AccessToken == "" {
		return Credentials{}, ErrInvalid
	}
	return Credentials{
		AccessToken:  decoded.AccessToken,
		RefreshToken: decoded.RefreshToken,
		TokenType:    decoded.TokenType,
	}, nil
}

type credentialJSON struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
}

func credentialAAD(account Account) []byte {
	// JSON provides unambiguous field boundaries and deterministic ordering for
	// this fixed struct. Mutable display/workspace labels are intentionally not
	// included so renaming an account does not invalidate its credential.
	b, _ := json.Marshal(struct {
		Version  int    `json:"version"`
		ID       string `json:"id"`
		UserID   string `json:"user_id"`
		Provider string `json:"provider"`
	}{1, account.ID, account.UserID, account.Provider})
	return b
}
