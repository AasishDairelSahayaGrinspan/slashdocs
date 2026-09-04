package auth

import "errors"

// OIDCSettings is the login provider configuration resolved from environment
// variables.
type OIDCSettings struct {
	IssuerURL         string
	InternalIssuerURL string
	ClientID          string
	ClientSecret      string
	// ProviderName labels the login button ("Continue with <ProviderName>").
	ProviderName string
}

// ResolveOIDCSettings resolves the generic AIDOCS_OIDC_* variables and the
// legacy GOOGLE_OAUTH_CLIENT_ID/GOOGLE_OAUTH_CLIENT_SECRET pair into one
// provider configuration. The Google pair keeps working as an alias for
// issuer https://accounts.google.com so existing deployments need no changes;
// mixing the two families is a configuration error. getenv is os.Getenv in
// production.
func ResolveOIDCSettings(getenv func(string) string) (OIDCSettings, error) {
	google := OIDCSettings{
		IssuerURL:    GoogleIssuer,
		ClientID:     getenv("GOOGLE_OAUTH_CLIENT_ID"),
		ClientSecret: getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
	}
	generic := OIDCSettings{
		IssuerURL:         getenv("AIDOCS_OIDC_ISSUER_URL"),
		InternalIssuerURL: getenv("AIDOCS_OIDC_INTERNAL_ISSUER_URL"),
		ClientID:          getenv("AIDOCS_OIDC_CLIENT_ID"),
		ClientSecret:      getenv("AIDOCS_OIDC_CLIENT_SECRET"),
	}
	providerName := getenv("AIDOCS_OIDC_PROVIDER_NAME")

	googleSet := google.ClientID != "" || google.ClientSecret != ""
	genericSet := generic.IssuerURL != "" || generic.InternalIssuerURL != "" || generic.ClientID != "" || generic.ClientSecret != ""
	switch {
	case googleSet && genericSet:
		return OIDCSettings{}, errors.New("configure either GOOGLE_OAUTH_CLIENT_ID/GOOGLE_OAUTH_CLIENT_SECRET or the AIDOCS_OIDC_* variables, not both")
	case googleSet:
		if google.ClientID == "" || google.ClientSecret == "" {
			return OIDCSettings{}, errors.New("GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET are both required")
		}
		google.ProviderName = defaultString(providerName, "Google")
		return google, nil
	case genericSet:
		if generic.IssuerURL == "" || generic.ClientID == "" || generic.ClientSecret == "" {
			return OIDCSettings{}, errors.New("AIDOCS_OIDC_ISSUER_URL, AIDOCS_OIDC_CLIENT_ID, and AIDOCS_OIDC_CLIENT_SECRET are all required")
		}
		generic.ProviderName = defaultString(providerName, "SSO")
		return generic, nil
	default:
		return OIDCSettings{}, errors.New("login is not configured: set AIDOCS_OIDC_ISSUER_URL, AIDOCS_OIDC_CLIENT_ID, and AIDOCS_OIDC_CLIENT_SECRET (or GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET for Google)")
	}
}

func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
