package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

const audSession = "session"

type SessionCodec struct{ Secret []byte }

type signedPayload struct {
	Subject   string `json:"sub"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	// Nonce makes every minted session token distinct. Without it, two logins
	// in the same second would produce the same string, and revoking one of
	// them on sign-out would revoke the other too. Older tokens carry no nonce
	// and keep verifying.
	Nonce string `json:"jti,omitempty"`
}

// SessionTTL is how long a browser session token is accepted. The session
// cookie's lifetime is derived from it, so the cookie and the token it carries
// cannot drift apart.
const SessionTTL = 30 * 24 * time.Hour

func (s SessionCodec) Sign(userID string) string {
	return s.sign(signedPayload{
		Subject:   userID,
		Audience:  audSession,
		ExpiresAt: time.Now().Add(SessionTTL).Unix(),
		Nonce:     sessionNonce(),
	})
}

func (s SessionCodec) SignForAudience(userID, audience string, ttl time.Duration) string {
	return s.sign(signedPayload{Subject: userID, Audience: audience, ExpiresAt: time.Now().Add(ttl).Unix()})
}

func (s SessionCodec) sign(p signedPayload) string {
	payload, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(payloadB64))
	sig := mac.Sum(nil)
	return "v1." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func sessionNonce() string {
	b := make([]byte, 16)
	// crypto/rand.Read never returns an error and always fills b.
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s SessionCodec) VerifySession(v string) (string, bool) {
	return s.VerifyAudience(v, audSession)
}

// VerifySessionExpiry verifies a session token and also returns the instant it
// stops being accepted. Revoking a session records that expiry, so a denylist
// row can be dropped once the token would have expired on its own.
func (s SessionCodec) VerifySessionExpiry(v string) (string, time.Time, bool) {
	p, ok := s.verify(v, audSession)
	if !ok {
		return "", time.Time{}, false
	}
	return p.Subject, time.Unix(p.ExpiresAt, 0), true
}

func (s SessionCodec) VerifyAudience(v, audience string) (string, bool) {
	p, ok := s.verify(v, audience)
	return p.Subject, ok
}

func (s SessionCodec) verify(v, audience string) (signedPayload, bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return signedPayload{}, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return signedPayload{}, false
	}
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return signedPayload{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return signedPayload{}, false
	}
	var p signedPayload
	if err := json.Unmarshal(payload, &p); err != nil || p.Subject == "" || p.Audience != audience || p.ExpiresAt < time.Now().Unix() {
		return signedPayload{}, false
	}
	return p, true
}
