//go:build aidocsdev

package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDevAuthenticatorSelectsOnlyConfiguredIdentities(t *testing.T) {
	authenticator, err := NewDevAuthenticator(map[string]Principal{
		"Owner":    {Type: PrincipalUser, ID: "dev_owner", Email: "owner@local.test"},
		"Reviewer": {Type: PrincipalUser, ID: "dev_reviewer", Email: "reviewer@local.test"},
	}, "OWNER")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		header  string
		cookie  string
		wantID  string
		wantErr error
	}{
		{name: "default", wantID: "dev_owner"},
		{name: "header", header: " Reviewer ", wantID: "dev_reviewer"},
		{name: "cookie", cookie: "reviewer", wantID: "dev_reviewer"},
		{name: "header wins", header: "owner", cookie: "reviewer", wantID: "dev_owner"},
		{name: "unknown header", header: "intruder", wantErr: ErrUnauthorized},
		{name: "unknown cookie", cookie: "intruder", wantErr: ErrUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://127.0.0.1/v1/me", nil)
			if tt.header != "" {
				req.Header.Set(DevIdentityHeader, tt.header)
			}
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: DevIdentityCookie, Value: tt.cookie})
			}
			principal, err := authenticator.Authenticate(req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Authenticate() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && principal.ID != tt.wantID {
				t.Fatalf("Authenticate() principal = %#v, want id %q", principal, tt.wantID)
			}
		})
	}

	if got, want := authenticator.IdentityNames(), []string{"owner", "reviewer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IdentityNames() = %v, want %v", got, want)
	}
}

func TestNewDevAuthenticatorRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewDevAuthenticator(nil, "owner"); err == nil {
		t.Fatal("NewDevAuthenticator() accepted a missing default identity")
	}
	if _, err := NewDevAuthenticator(map[string]Principal{"owner": {}}, "owner"); err == nil {
		t.Fatal("NewDevAuthenticator() accepted an incomplete principal")
	}
}
