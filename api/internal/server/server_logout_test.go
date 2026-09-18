package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/server"
)

const sessionTestOrigin = "https://app.example"

// deferredAuthenticator forwards to the production cookie authenticator,
// which can only be built once the test environment's store exists.
type deferredAuthenticator struct{ inner auth.Authenticator }

func (a *deferredAuthenticator) Authenticate(r *http.Request) (*auth.Principal, error) {
	return a.inner.Authenticate(r)
}

// newSessionTestEnv wires the canonical fixture graph to the production
// cookie authenticator, so tests exercise real signed session cookies instead
// of the X-Test-Principal header.
func newSessionTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return newResolvedSessionTestEnv(t, nil)
}

// newResolvedSessionTestEnv lets a test substitute the authenticator's
// resolver (e.g. to simulate a database outage) while keeping the fixture
// graph backed by the real store.
func newResolvedSessionTestEnv(t *testing.T, resolve func(*pgrepo.Store) auth.DBAuthenticator) *testEnv {
	t.Helper()
	authenticator := &deferredAuthenticator{}
	e := newConfiguredTestEnv(t, server.Config{
		Environment:   "test",
		AppOrigin:     sessionTestOrigin,
		SessionSecret: oauthTestSigningMaterial,
	}, server.WithAuthenticator(authenticator))
	if resolve == nil {
		resolve = func(store *pgrepo.Store) auth.DBAuthenticator {
			return auth.DBAuthenticator{Resolver: store, SessionSecret: oauthTestSigningMaterial}
		}
	}
	authenticator.inner = resolve(e.store)
	return e
}

// signIn mints a session cookie value exactly as the OIDC callback does.
func signIn(p auth.Principal) string {
	return (auth.SessionCodec{Secret: []byte(oauthTestSigningMaterial)}).Sign(p.ID)
}

func sessionCookie(token string) string {
	//nolint:gosec // G124: this test-only cookie is injected directly into httptest requests; no network transport is used.
	return (&http.Cookie{Name: "aidocs_session", Value: token}).String()
}

// cookieHeaders presents a session cookie on a safe request.
func cookieHeaders(token string) map[string]string {
	return map[string]string{"Cookie": sessionCookie(token)}
}

// logoutHeaders presents a session cookie on a same-origin unsafe request,
// which is what the browser sends when a user signs out.
func logoutHeaders(token string) map[string]string {
	return map[string]string{"Cookie": sessionCookie(token), "Origin": sessionTestOrigin}
}

func responseCookie(t *testing.T, rr *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response did not set a %q cookie", name)
	return nil
}

func TestLogoutMakesThePresentedSessionCookieUnusable(t *testing.T) {
	e := newSessionTestEnv(t)
	token := signIn(fixtureUser)
	assertStatus(t, do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(token)), http.StatusOK)

	assertStatus(t, do(t, e.h, http.MethodPost, "/v1/auth/logout", "", logoutHeaders(token)), http.StatusNoContent)

	replay := do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(token))
	assertStatus(t, replay, http.StatusUnauthorized)
	assertJSON(t, replay.Body.Bytes(), `{
	  "error": { "code": "unauthorized", "message": "authentication required" }
	}`)
}

func TestLogoutClearsTheSessionCookieWithTheAttributesItWasSetWith(t *testing.T) {
	e := newSessionTestEnv(t)

	rr := do(t, e.h, http.MethodPost, "/v1/auth/logout", "", logoutHeaders(signIn(fixtureUser)))

	assertStatus(t, rr, http.StatusNoContent)
	cookie := responseCookie(t, rr, "aidocs_session")
	if cookie.Value != "" || cookie.MaxAge >= 0 {
		t.Fatalf("logout did not expire the session cookie: %#v", cookie)
	}
	if cookie.Path != "/" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" {
		t.Fatalf("cleared session cookie does not match the login cookie attributes: %#v", cookie)
	}
}

func TestLogoutSucceedsWithoutAUsableSession(t *testing.T) {
	e := newSessionTestEnv(t)
	expired := (auth.SessionCodec{Secret: []byte(oauthTestSigningMaterial)}).SignForAudience(fixtureUser.ID, "session", -time.Hour)
	token := signIn(fixtureUser)
	assertStatus(t, do(t, e.h, http.MethodPost, "/v1/auth/logout", "", logoutHeaders(token)), http.StatusNoContent)

	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{"no cookie at all", map[string]string{"Origin": sessionTestOrigin}},
		{"unparsable cookie value", logoutHeaders("not-a-session-token")},
		{"cookie signed with another secret", logoutHeaders((auth.SessionCodec{Secret: []byte("another-secret")}).Sign(fixtureUser.ID))},
		{"expired cookie", logoutHeaders(expired)},
		{"already revoked cookie", logoutHeaders(token)},
		{"bearer caller with no cookie", map[string]string{"Authorization": "Bearer aidocs_cli_absent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertStatus(t, do(t, e.h, http.MethodPost, "/v1/auth/logout", "", tc.headers), http.StatusNoContent)
		})
	}
}

func TestLogoutRejectsCrossSiteRequests(t *testing.T) {
	e := newSessionTestEnv(t)
	token := signIn(fixtureUser)

	rr := do(t, e.h, http.MethodPost, "/v1/auth/logout", "", map[string]string{
		"Cookie": sessionCookie(token),
		"Origin": "https://evil.example",
	})

	assertStatus(t, rr, http.StatusForbidden)
	assertJSON(t, rr.Body.Bytes(), `{
	  "error": { "code": "forbidden", "message": "csrf check failed" }
	}`)
	assertStatus(t, do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(token)), http.StatusOK)
}

func TestLogoutRevokesOnlyThePresentedSession(t *testing.T) {
	e := newSessionTestEnv(t)
	token := signIn(fixtureUser)
	otherDevice := signIn(fixtureUser)
	otherUser := signIn(fixtureOwner)

	assertStatus(t, do(t, e.h, http.MethodPost, "/v1/auth/logout", "", logoutHeaders(token)), http.StatusNoContent)

	assertStatus(t, do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(token)), http.StatusUnauthorized)
	assertStatus(t, do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(otherDevice)), http.StatusOK)
	assertStatus(t, do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(otherUser)), http.StatusOK)
}

func TestLogoutDoesNotRevokeASessionMintedAfterwards(t *testing.T) {
	e := newSessionTestEnv(t)
	token := signIn(fixtureUser)

	assertStatus(t, do(t, e.h, http.MethodPost, "/v1/auth/logout", "", logoutHeaders(token)), http.StatusNoContent)

	// Signing back in hands out a session of its own even when it lands in the
	// same second as the sign-out, so the revocation cannot follow the user
	// into their next session.
	next := signIn(fixtureUser)
	if next == token {
		t.Fatal("signing in again minted the revoked session token")
	}
	assertStatus(t, do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(next)), http.StatusOK)
}

func TestRevokedSessionIsRejectedOnOAuthAuthorize(t *testing.T) {
	e := newSessionTestEnv(t)
	token := signIn(fixtureUser)
	headers := map[string]string{"Cookie": sessionCookie(token), "Accept": "application/json"}
	// Without the authorization parameters the endpoint answers 400; the point
	// is only that the cookie authenticates before logout.
	if rr := do(t, e.h, http.MethodGet, "/oauth/authorize", "", headers); rr.Code == http.StatusUnauthorized {
		t.Fatalf("session cookie was not accepted on /oauth/authorize before logout: %d", rr.Code)
	}

	assertStatus(t, do(t, e.h, http.MethodPost, "/v1/auth/logout", "", logoutHeaders(token)), http.StatusNoContent)

	assertStatus(t, do(t, e.h, http.MethodGet, "/oauth/authorize", "", headers), http.StatusUnauthorized)
}

// failingSessionDenylist resolves principals normally but fails every
// denylist lookup, standing in for a database outage.
type failingSessionDenylist struct {
	*pgrepo.Store
	err error
}

func (f failingSessionDenylist) SessionRevoked(context.Context, string) (bool, error) {
	return false, f.err
}

func TestSessionDenylistLookupFailureIsNotAuthenticated(t *testing.T) {
	denylistDown := errors.New("session denylist unavailable")
	e := newResolvedSessionTestEnv(t, func(store *pgrepo.Store) auth.DBAuthenticator {
		return auth.DBAuthenticator{
			Resolver:      failingSessionDenylist{Store: store, err: denylistDown},
			SessionSecret: oauthTestSigningMaterial,
		}
	})

	rr := do(t, e.h, http.MethodGet, "/v1/me", "", cookieHeaders(signIn(fixtureUser)))

	assertStatus(t, rr, http.StatusInternalServerError)
	assertJSON(t, rr.Body.Bytes(), `{
	  "error": { "code": "internal", "message": "internal server error" }
	}`)
}
