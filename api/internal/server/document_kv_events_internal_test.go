package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/testdb"
)

type streamAuthorizationCheck struct {
	principal  auth.Principal
	documentID string
}

type streamAuthorizationRepository struct {
	role  repo.Role
	err   error
	calls chan streamAuthorizationCheck
}

func (r *streamAuthorizationRepository) ListDocumentKVEvents(context.Context, string, int64, int32) ([]repo.DocumentKVEvent, error) {
	return nil, nil
}

func (r *streamAuthorizationRepository) RoleForDocument(_ context.Context, principal auth.Principal, documentID string) (repo.Role, error) {
	r.calls <- streamAuthorizationCheck{principal: principal, documentID: documentID}
	return r.role, r.err
}

type streamResponseWriter struct {
	header http.Header
	writes chan string
}

func newStreamResponseWriter() *streamResponseWriter {
	return &streamResponseWriter{header: make(http.Header), writes: make(chan string, 1)}
}

func (w *streamResponseWriter) Header() http.Header { return w.header }

func (*streamResponseWriter) WriteHeader(int) {}

func (w *streamResponseWriter) Write(body []byte) (int, error) {
	w.writes <- string(body)
	return len(body), nil
}

func (*streamResponseWriter) Flush() {}

// TestDocumentKVStreamRejectsRevokedSessionCookie drives the stream's periodic
// re-authorization with the production cookie authenticator and a real store,
// because a long-lived SSE connection is the one place a signed-out session
// would otherwise survive.
func TestDocumentKVStreamRejectsRevokedSessionCookie(t *testing.T) {
	ctx := context.Background()
	store, err := pgrepo.Connect(ctx, testdb.New(t))
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(store.Close)
	const secret = "test-session-signing-material"
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "usr_stream", Email: "stream@example.com", Name: "Stream"}
	if _, err := store.UpsertGoogleUser(ctx, viewer.ID, viewer.Email, viewer.Name, "sub-"+viewer.ID, ""); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	doc, _, err := store.CreateDocument(ctx, viewer, "stream", []byte("<html><body>stream</body></html>"))
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}

	token := (auth.SessionCodec{Secret: []byte(secret)}).Sign(viewer.ID)
	request := httptest.NewRequest(http.MethodGet, "/v1/documents/"+doc.ID+"/kv/events", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	authenticator := auth.DBAuthenticator{Resolver: store, SessionSecret: secret}
	stream := &documentKVStream{
		ctx:        ctx,
		repository: store,
		authenticate: func(ctx context.Context) (*auth.Principal, error) {
			return authenticator.Authenticate(request.Clone(ctx))
		},
		principal:  viewer,
		documentID: doc.ID,
	}

	if !stream.reauthorize() {
		t.Fatal("stream dropped a live session cookie")
	}

	_, expiresAt, ok := (auth.SessionCodec{Secret: []byte(secret)}).VerifySessionExpiry(token)
	if !ok {
		t.Fatal("test session token did not verify")
	}
	if err := store.RevokeSession(ctx, auth.HashToken(token), expiresAt); err != nil {
		t.Fatalf("revoke session: %v", err)
	}

	if stream.reauthorize() {
		t.Fatal("stream kept a revoked session cookie connected")
	}
}

func TestDocumentKVStreamReauthorizesOnKeepalive(t *testing.T) {
	principal := auth.Principal{Type: auth.PrincipalUser, ID: "viewer_1"}
	lookupErr := errors.New("authorization lookup failed")
	credentialErr := errors.New("credential lookup failed")

	for _, tc := range []struct {
		name                   string
		authenticatedPrincipal auth.Principal
		authenticateErr        error
		role                   repo.Role
		roleErr                error
		checksRole             bool
		continues              bool
	}{
		{
			name:                   "viewer remains connected",
			authenticatedPrincipal: principal,
			role:                   repo.RoleViewer,
			checksRole:             true,
			continues:              true,
		},
		{
			name:                   "revoked viewer is disconnected",
			authenticatedPrincipal: principal,
			role:                   repo.RoleNone,
			checksRole:             true,
		},
		{
			name:                   "authorization lookup failure closes fail-safe",
			authenticatedPrincipal: principal,
			roleErr:                lookupErr,
			checksRole:             true,
		},
		{name: "revoked credential is disconnected", authenticateErr: auth.ErrUnauthorized},
		{name: "credential lookup failure closes fail-safe", authenticateErr: credentialErr},
		{
			name:                   "changed principal is disconnected",
			authenticatedPrincipal: auth.Principal{Type: auth.PrincipalUser, ID: "viewer_2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			keepAlive := make(chan time.Time, 1)
			authenticateCalls := make(chan struct{}, 1)
			repository := &streamAuthorizationRepository{
				role:  tc.role,
				err:   tc.roleErr,
				calls: make(chan streamAuthorizationCheck, 1),
			}
			writer := newStreamResponseWriter()
			stream := &documentKVStream{
				ctx:        ctx,
				writer:     writer,
				flusher:    writer,
				repository: repository,
				authenticate: func(context.Context) (*auth.Principal, error) {
					authenticateCalls <- struct{}{}
					if tc.authenticateErr != nil {
						return nil, tc.authenticateErr
					}
					p := tc.authenticatedPrincipal
					return &p, nil
				},
				principal:  principal,
				documentID: "doc_stream",
				keepAlive:  keepAlive,
			}
			done := make(chan struct{})
			go func() {
				stream.run(make(chan repo.DocumentKVEvent))
				close(done)
			}()

			keepAlive <- time.Now()
			select {
			case <-authenticateCalls:
			case <-time.After(time.Second):
				t.Fatal("stream did not re-check credentials on keepalive")
			}
			if tc.checksRole {
				select {
				case check := <-repository.calls:
					if check.principal.Type != principal.Type || check.principal.ID != principal.ID || check.documentID != stream.documentID {
						t.Fatalf("authorization check = %#v, want principal %#v document %q", check, principal, stream.documentID)
					}
				case <-time.After(time.Second):
					t.Fatal("stream did not re-check document authorization on keepalive")
				}
			}

			if tc.continues {
				select {
				case write := <-writer.writes:
					if write != ": keepalive\n\n" {
						t.Fatalf("keepalive write = %q", write)
					}
				case <-time.After(time.Second):
					t.Fatal("authorized stream did not send a keepalive")
				}
				cancel()
			} else {
				select {
				case write := <-writer.writes:
					t.Fatalf("unauthorized stream wrote %q before closing", write)
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("unauthorized stream did not close")
				}
				if !tc.checksRole {
					select {
					case check := <-repository.calls:
						t.Fatalf("credential failure performed document authorization check %#v", check)
					default:
					}
				}
			}

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("stream did not stop")
			}
		})
	}
}
