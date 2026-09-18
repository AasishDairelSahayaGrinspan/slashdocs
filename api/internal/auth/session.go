package auth

import (
	"crypto/hmac"
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
}

func (s SessionCodec) Sign(userID string) string {
	return s.SignForAudience(userID, audSession, 30*24*time.Hour)
}

func (s SessionCodec) SignForAudience(userID, audience string, ttl time.Duration) string {
	p := signedPayload{Subject: userID, Audience: audience, ExpiresAt: time.Now().Add(ttl).Unix()}
	payload, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(payloadB64))
	sig := mac.Sum(nil)
	return "v1." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(sig)
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
