//go:build aidocsdev

package auth

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const (
	// DevIdentityHeader lets local API clients select a configured development
	// identity without going through Google OAuth.
	DevIdentityHeader = "X-Aidocs-Dev-Identity"
	// DevIdentityCookie is set by the development server's identity switcher.
	DevIdentityCookie = "aidocs_dev_identity"
)

// DevAuthenticator authenticates requests as one of a fixed set of local-only
// development identities. This type is excluded from normal builds.
type DevAuthenticator struct {
	identities      map[string]Principal
	defaultIdentity string
}

// NewDevAuthenticator constructs a development authenticator from an explicit
// identity allow-list. Identity names are normalized to lower case.
func NewDevAuthenticator(identities map[string]Principal, defaultIdentity string) (*DevAuthenticator, error) {
	configured := make(map[string]Principal, len(identities))
	for name, principal := range identities {
		name = normalizeDevIdentity(name)
		if name == "" || principal.Type == "" || principal.ID == "" {
			return nil, fmt.Errorf("development identities require a name, type, and id")
		}
		if _, exists := configured[name]; exists {
			return nil, fmt.Errorf("duplicate development identity %q", name)
		}
		configured[name] = principal
	}
	defaultIdentity = normalizeDevIdentity(defaultIdentity)
	if _, ok := configured[defaultIdentity]; !ok {
		return nil, fmt.Errorf("default development identity %q is not configured", defaultIdentity)
	}
	return &DevAuthenticator{identities: configured, defaultIdentity: defaultIdentity}, nil
}

// Authenticate implements Authenticator. A request header takes precedence
// over the switcher's cookie; requests without either use the default identity.
func (a *DevAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	name, ok := a.SelectedIdentity(r)
	if !ok {
		return nil, ErrUnauthorized
	}
	principal := a.identities[name]
	return &principal, nil
}

// SelectedIdentity returns the normalized identity selected by a request.
func (a *DevAuthenticator) SelectedIdentity(r *http.Request) (string, bool) {
	name := normalizeDevIdentity(r.Header.Get(DevIdentityHeader))
	if name == "" {
		if cookie, err := r.Cookie(DevIdentityCookie); err == nil {
			name = normalizeDevIdentity(cookie.Value)
		}
	}
	if name == "" {
		name = a.defaultIdentity
	}
	_, ok := a.identities[name]
	return name, ok
}

// IdentityNames returns the configured identity names in stable order.
func (a *DevAuthenticator) IdentityNames() []string {
	names := make([]string, 0, len(a.identities))
	for name := range a.identities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeDevIdentity(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
