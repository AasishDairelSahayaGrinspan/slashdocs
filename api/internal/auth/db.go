package auth

import (
	"context"
	"errors"
	"net/http"
)

type TokenResolver interface {
	ResolveBearerToken(ctx context.Context, tokenHash string) (Principal, error)
}
type UserResolver interface {
	ResolveUser(ctx context.Context, id string) (Principal, error)
}

// SessionRevocations reports whether a session token was revoked by a sign-out.
// Sessions are stateless signed tokens, so this denylist is the only thing that
// can end one before it expires. It is keyed by HashToken of the cookie value,
// never by the token itself.
type SessionRevocations interface {
	SessionRevoked(ctx context.Context, tokenHash string) (bool, error)
}

type DBAuthenticator struct {
	// The resolver carries the revocation lookup as well, so a session
	// authenticator cannot be wired without it.
	Resolver interface {
		TokenResolver
		UserResolver
		SessionRevocations
	}
	SessionSecret string
}

func (a DBAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	if token := BearerFromHeader(r.Header.Get("Authorization")); token != "" {
		p, err := a.Resolver.ResolveBearerToken(r.Context(), HashToken(token))
		if err != nil {
			// Only a genuine "not found" means the credential is invalid.
			// Propagate everything else (e.g. DB outages) as a 500 so real
			// failures are not masked as 401 Unauthorized.
			if errors.Is(err, ErrNotFound) {
				return nil, ErrUnauthorized
			}
			return nil, err
		}
		return &p, nil
	}
	if a.SessionSecret != "" {
		if ck, err := r.Cookie("aidocs_session"); err == nil {
			uid, ok := (SessionCodec{Secret: []byte(a.SessionSecret)}).VerifySession(ck.Value)
			if ok {
				// A signed-out session is exactly as unusable as an invalid
				// one. A failed lookup is an infrastructure fault rather than
				// a verdict, so it is propagated instead of admitting the
				// request on an unchecked token.
				revoked, err := a.Resolver.SessionRevoked(r.Context(), HashToken(ck.Value))
				if err != nil {
					return nil, err
				}
				if revoked {
					return nil, ErrUnauthorized
				}
				p, err := a.Resolver.ResolveUser(r.Context(), uid)
				if err != nil {
					// As with bearer auth, only a genuine "not found"
					// means the session is invalid; surface everything
					// else (e.g. DB outages) instead of masking as 401.
					if errors.Is(err, ErrNotFound) {
						return nil, ErrUnauthorized
					}
					return nil, err
				}
				return &p, nil
			}
		}
	}
	return nil, ErrUnauthorized
}
