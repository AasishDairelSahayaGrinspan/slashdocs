package server

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
)

const (
	mcpScope                    = "mcp"
	mcpClientIDPrefix           = "aidocs_mcp_client_"
	mcpClientRegistrationAud    = "mcp-client-registration"
	mcpClientRegistrationTTL    = 365 * 24 * time.Hour
	mcpAuthorizationCodeTTLHint = 5 * time.Minute
	mcpAccessTokenTTL           = time.Hour
	maxMCPAuthRequestBytes      = 64 * 1024
	// Dynamic registration is embedded in the signed client ID. These byte
	// limits keep the resulting /oauth/authorize request comfortably below the
	// common 8 KiB proxy request-line/header limit while preserving ordinary
	// HTTPS and loopback callback URLs.
	maxMCPClientNameBytes  = 128
	maxMCPRedirectURIBytes = 1024
	maxMCPClientIDBytes    = 2048
	// Registration bounds the client name in bytes; the consent page bounds
	// what it renders in runes, so no name reaching the template can outgrow
	// the destination it is shown next to.
	maxMCPDisplayNameRunes = maxMCPClientNameBytes
	defaultMCPClientName   = "MCP client"
)

type mcpOAuthClient struct {
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
}

type mcpClientRegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type mcpProtectedResourceMetadataResponse struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported"`
}

type mcpAuthorizationServerMetadataResponse struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ResponseModesSupported            []string `json:"response_modes_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
}

type mcpClientRegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientSecretExpiresAt   int64    `json:"client_secret_expires_at"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type mcpTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

type mcpTokenRequest struct {
	Code         string
	CodeVerifier string
	ClientID     string
	RedirectURI  string
	Resource     string
}

type oauthRequestError struct {
	Status      int
	Code        string
	Description string
}

type oauthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type mcpAuthorizeRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
	Scope               string
	State               string
}

func (h handlers) mcpResource() string { return strings.TrimRight(h.deps.appOrigin, "/") + "/mcp" }

func (h handlers) mcpProtectedResourceMetadata(c *gin.Context) {
	c.JSON(http.StatusOK, mcpProtectedResourceMetadataResponse{
		Resource:               h.mcpResource(),
		AuthorizationServers:   []string{strings.TrimRight(h.deps.appOrigin, "/")},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{mcpScope},
	})
}

func (h handlers) mcpAuthorizationServerMetadata(c *gin.Context) {
	origin := strings.TrimRight(h.deps.appOrigin, "/")
	c.JSON(http.StatusOK, mcpAuthorizationServerMetadataResponse{
		Issuer:                            origin,
		AuthorizationEndpoint:             origin + "/oauth/authorize",
		TokenEndpoint:                     origin + "/oauth/token",
		RegistrationEndpoint:              origin + "/oauth/register",
		ResponseTypesSupported:            []string{"code"},
		ResponseModesSupported:            []string{"query"},
		GrantTypesSupported:               []string{"authorization_code"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		ScopesSupported:                   []string{mcpScope},
	})
}

func (h handlers) mcpRegisterClient(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMCPAuthRequestBytes)
	if !requireJSON(c) {
		return
	}
	var in mcpClientRegistrationRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata", "invalid client registration")
		return
	}
	if message := validateMCPClientRegistration(in); message != "" {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata", message)
		return
	}
	client := mcpOAuthClient{Name: strings.TrimSpace(in.ClientName), RedirectURIs: slices.Clone(in.RedirectURIs)}
	if client.Name == "" {
		client.Name = defaultMCPClientName
	}
	b, err := json.Marshal(client)
	if err != nil {
		internalErr(c, err)
		return
	}
	subject := base64.RawURLEncoding.EncodeToString(b)
	clientID := mcpClientIDPrefix + (auth.SessionCodec{Secret: []byte(h.deps.sessionSecret)}).SignForAudience(subject, mcpClientRegistrationAud, mcpClientRegistrationTTL)
	if len(clientID) > maxMCPClientIDBytes {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata", "client metadata produces an oversized client_id")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, mcpClientRegistrationResponse{
		ClientID:                clientID,
		ClientIDIssuedAt:        time.Now().Unix(),
		ClientSecretExpiresAt:   0,
		ClientName:              client.Name,
		RedirectURIs:            client.RedirectURIs,
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code"},
		ResponseTypes:           []string{"code"},
	})
}

func validateMCPClientRegistration(in mcpClientRegistrationRequest) string {
	if len(strings.TrimSpace(in.ClientName)) > maxMCPClientNameBytes {
		return "client_name must not exceed 128 bytes"
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 10 {
		return "redirect_uris must contain between 1 and 10 entries"
	}
	for _, raw := range in.RedirectURIs {
		if len(raw) > maxMCPRedirectURIBytes {
			return "each redirect_uri must not exceed 1024 bytes"
		}
		if !validMCPRedirectURI(raw) {
			return "redirect_uris must use HTTPS, loopback HTTP, or a private-use app scheme, without fragments or credentials"
		}
	}
	if in.TokenEndpointAuthMethod != "" && in.TokenEndpointAuthMethod != "none" {
		return "only public clients with token_endpoint_auth_method=none are supported"
	}
	if message := validateMCPGrantTypes(in.GrantTypes); message != "" {
		return message
	}
	if len(in.ResponseTypes) > 0 && !slices.Equal(in.ResponseTypes, []string{"code"}) {
		return "only the code response type is supported"
	}
	return ""
}

// validateMCPGrantTypes requires the authorization_code grant and tolerates a
// co-registered refresh_token grant, which conformant MCP clients (mcp2cli,
// VS Code) request by default during dynamic registration. The server never
// issues refresh tokens, so the grant is accepted here but never exercised:
// clients simply re-run the authorization_code flow when the short-lived
// access token expires. Rejecting the registration outright would lock those
// clients out entirely.
func validateMCPGrantTypes(grantTypes []string) string {
	if len(grantTypes) == 0 {
		return ""
	}
	if !slices.Contains(grantTypes, "authorization_code") {
		return "the authorization_code grant is required"
	}
	for _, grant := range grantTypes {
		if grant != "authorization_code" && grant != "refresh_token" {
			return "only the authorization_code and refresh_token grants are supported"
		}
	}
	return ""
}

// redirectURIMatchesRegistration checks a request's redirect_uri against the
// URIs frozen into the signed client_id. RFC 8252 §7.3 requires the
// authorization server to allow any port on a loopback IP redirect URI, since
// native apps bind an ephemeral port at request time and a client_id cached
// across runs carries a stale port. Loopback IP literals therefore match on
// scheme, host, path, and query while ignoring the port; everything else
// (including https and localhost) still requires an exact match, because only
// loopback delivery to the user's own machine makes a free port safe.
func redirectURIMatchesRegistration(registered []string, requested string) bool {
	if slices.Contains(registered, requested) {
		return true
	}
	req, err := url.Parse(requested)
	if err != nil || !isLoopbackIPHost(req.Hostname()) {
		return false
	}
	for _, raw := range registered {
		reg, err := url.Parse(raw)
		if err != nil || !isLoopbackIPHost(reg.Hostname()) {
			continue
		}
		if strings.EqualFold(reg.Scheme, req.Scheme) &&
			strings.EqualFold(reg.Hostname(), req.Hostname()) &&
			reg.Path == req.Path && reg.RawQuery == req.RawQuery {
			return true
		}
	}
	return false
}

func isLoopbackIPHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validMCPRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	switch scheme := strings.ToLower(u.Scheme); scheme {
	case "https":
		return u.Host != ""
	case "http":
		// Loopback only (RFC 8252 §7.3): native apps listening on localhost.
		host := u.Hostname()
		if strings.EqualFold(host, "localhost") {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	default:
		// Private-use app schemes (RFC 8252 §7.1): cursor://, vscode://, and
		// friends. The OS resolves the scheme to the installed client, and an
		// intercepted code is useless without the PKCE verifier. Refuse
		// schemes a browser could interpret as active or local content.
		switch scheme {
		case "javascript", "data", "file", "blob", "about", "vbscript", "filesystem", "chrome", "chrome-extension":
			return false
		}
		return true
	}
}

func (h handlers) parseMCPClient(clientID string) (mcpOAuthClient, bool) {
	if len(clientID) > maxMCPClientIDBytes || !strings.HasPrefix(clientID, mcpClientIDPrefix) {
		return mcpOAuthClient{}, false
	}
	subject, ok := (auth.SessionCodec{Secret: []byte(h.deps.sessionSecret)}).VerifyAudience(
		strings.TrimPrefix(clientID, mcpClientIDPrefix),
		mcpClientRegistrationAud,
	)
	if !ok {
		return mcpOAuthClient{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(subject)
	if err != nil {
		return mcpOAuthClient{}, false
	}
	var client mcpOAuthClient
	if json.Unmarshal(b, &client) != nil || validateMCPClientRegistration(mcpClientRegistrationRequest{RedirectURIs: client.RedirectURIs}) != "" {
		return mcpOAuthClient{}, false
	}
	return client, true
}

func mcpAuthorizeRequestFrom(c *gin.Context) mcpAuthorizeRequest {
	value := c.Query
	if c.Request.Method == http.MethodPost {
		value = c.PostForm
	}
	return mcpAuthorizeRequest{
		ClientID:            value("client_id"),
		RedirectURI:         value("redirect_uri"),
		ResponseType:        value("response_type"),
		CodeChallenge:       value("code_challenge"),
		CodeChallengeMethod: value("code_challenge_method"),
		Resource:            value("resource"),
		Scope:               value("scope"),
		State:               value("state"),
	}
}

func (h handlers) validateMCPAuthorizeRequest(in mcpAuthorizeRequest) (mcpOAuthClient, string) {
	client, ok := h.parseMCPClient(in.ClientID)
	if !ok {
		return mcpOAuthClient{}, "unknown or expired client_id"
	}
	if !redirectURIMatchesRegistration(client.RedirectURIs, in.RedirectURI) {
		return mcpOAuthClient{}, "redirect_uri is not registered for this client"
	}
	if in.ResponseType != "code" {
		return mcpOAuthClient{}, "response_type must be code"
	}
	if in.CodeChallengeMethod != "S256" || !validPKCEChallenge(in.CodeChallenge) {
		return mcpOAuthClient{}, "S256 PKCE is required"
	}
	if in.Resource != h.mcpResource() {
		return mcpOAuthClient{}, "resource must identify this MCP server"
	}
	if in.Scope != "" && in.Scope != mcpScope {
		return mcpOAuthClient{}, "unsupported scope"
	}
	return client, ""
}

func validPKCEChallenge(challenge string) bool {
	if len(challenge) < 43 || len(challenge) > 128 {
		return false
	}
	for _, r := range challenge {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~", r) {
			return false
		}
	}
	return true
}

// mcpConsentTarget describes, for the consent page, where an approval actually
// sends the authorization code. Registration is unauthenticated, so the client
// name beside it carries no weight; the destination is what a user can refuse
// on.
type mcpConsentTarget struct {
	Destination string
	Note        string
}

// mcpConsentTargetFor describes a redirect URI that has already been validated
// against the signed client registration. It is derived from that same URI —
// the one bound into the authorization code and used to deliver it — so the
// page cannot show one destination while the code goes to another.
func mcpConsentTargetFor(redirect *url.URL) mcpConsentTarget {
	scheme := strings.ToLower(redirect.Scheme)
	if scheme != "http" && scheme != "https" {
		// A private-use app scheme (RFC 8252 §7.1) names no host: the operating
		// system hands the code to whichever installed app claims the scheme,
		// so the scheme is the whole destination.
		return mcpConsentTarget{
			Destination: scheme + "://",
			Note:        "An app on this computer that is registered to open these links.",
		}
	}
	target := mcpConsentTarget{
		Destination: mcpRedirectDisplayHost(redirect),
		Note:        "A website on the internet.",
	}
	if host := redirect.Hostname(); strings.EqualFold(host, "localhost") || isLoopbackIPHost(host) {
		target.Note = "An application running on this computer."
	}
	return target
}

// mcpRedirectDisplayHost renders a redirect host the way a browser resolves it:
// IDNA-encoded to ASCII so a homograph lookalike reads as its xn-- form,
// bracketed when it is an IPv6 literal, and carrying the port only when it is
// not the scheme default. A host that cannot be canonicalized is shown as it
// was received; the template escapes it either way.
func mcpRedirectDisplayHost(redirect *url.URL) string {
	host, ok := canonicalHTTPHostname(redirect.Hostname())
	if !ok {
		host = redirect.Hostname()
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := redirect.Port(); port != "" && port != defaultHTTPPort(strings.ToLower(redirect.Scheme)) {
		host = host + ":" + port
	}
	return host
}

// mcpDisplayClientName neutralizes a self-reported client name for display.
// Registration caps the name's length but accepts any runes in it, so strip
// here what can visually rewrite the consent page: control runes become
// spaces, and format runes — bidi overrides and isolates such as U+202E,
// zero-width joiners — are dropped entirely. html/template escapes the rest.
func mcpDisplayClientName(name string) string {
	var display strings.Builder
	runes := 0
	for _, r := range name {
		if r == utf8.RuneError || unicode.Is(unicode.Cf, r) {
			continue
		}
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			r = ' '
		}
		if runes == maxMCPDisplayNameRunes {
			display.WriteRune('…')
			break
		}
		display.WriteRune(r)
		runes++
	}
	collapsed := strings.Join(strings.Fields(display.String()), " ")
	if collapsed == "" {
		return defaultMCPClientName
	}
	return collapsed
}

var mcpConsentTemplate = template.Must(template.New("mcp-consent").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connect {{.ClientName}} to aidocs</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:36rem;margin:10vh auto;padding:0 1.5rem;color:#171717}section{border:1px solid #ddd;border-radius:12px;padding:1.5rem}h1{font-size:1.4rem;margin-bottom:.5rem}p{color:#525252}.target{font:700 1.6rem/1.3 ui-monospace,SFMono-Regular,Menlo,monospace;color:#171717;margin:.25rem 0 0;overflow-wrap:anywhere}.target-note{margin:.25rem 0 1.25rem}.actions{display:flex;gap:.75rem;margin-top:1.5rem}button{padding:.65rem 1rem;border-radius:8px;border:1px solid #bbb;background:#fff;font:inherit}button[value=approve]{background:#171717;color:#fff;border-color:#171717}</style></head>
<body><section><h1>Connect {{.ClientName}}?</h1><p>That name is self-reported by the application and unverified by aidocs. Approving sends your authorization code to:</p>
<p class="target">{{.Destination}}</p><p class="target-note">{{.DestinationNote}}</p>
<p>This gives the client access to aidocs as <strong>{{.User}}</strong>. It can read and change the same documents, comments, folders, sharing settings, and service accounts that you can.</p>
<form method="post" action="/oauth/authorize">{{range $k,$v := .Fields}}<input type="hidden" name="{{$k}}" value="{{$v}}">{{end}}<div class="actions"><button type="submit" name="decision" value="deny">Cancel</button><button type="submit" name="decision" value="approve">Connect</button></div></form></section></body></html>`))

func (h handlers) mcpAuthorize(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMCPAuthRequestBytes)
	if current(c) == nil || current(c).Type != auth.PrincipalUser {
		oauthError(c, http.StatusForbidden, "access_denied", "MCP browser authorization requires a user account")
		return
	}
	in := mcpAuthorizeRequestFrom(c)
	client, message := h.validateMCPAuthorizeRequest(in)
	if message != "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", message)
		return
	}
	if c.Request.Method == http.MethodGet {
		p := current(c)
		user := p.Name
		if user == "" {
			user = p.Email
		}
		fields := map[string]string{
			"client_id": in.ClientID, "redirect_uri": in.RedirectURI, "response_type": in.ResponseType,
			"code_challenge": in.CodeChallenge, "code_challenge_method": in.CodeChallengeMethod,
			"resource": in.Resource, "scope": firstNonEmpty(in.Scope, mcpScope), "state": in.State,
		}
		redirectURL, _ := url.Parse(in.RedirectURI) // already validated against the signed client registration
		target := mcpConsentTargetFor(redirectURL)
		redirectOrigin := redirectURL.Scheme + "://" + redirectURL.Host
		if !strings.EqualFold(redirectURL.Scheme, "http") && !strings.EqualFold(redirectURL.Scheme, "https") {
			// Private-use app schemes are expressed as scheme-only CSP
			// sources; browsers do not reliably match host sources on
			// unregistered schemes.
			redirectOrigin = redirectURL.Scheme + ":"
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		// form-action is checked again after redirects, so allow the registered
		// callback origin as well as the same-origin consent POST.
		c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self' "+redirectOrigin+"; frame-ancestors 'none'; base-uri 'none'")
		c.Header("X-Frame-Options", "DENY")
		// Keep same-origin form submissions compatible with the application's
		// cookie CSRF check without leaking authorization parameters off-origin.
		c.Header("Referrer-Policy", "same-origin")
		if err := mcpConsentTemplate.Execute(c.Writer, map[string]any{
			"ClientName": mcpDisplayClientName(client.Name), "User": user, "Fields": fields,
			"Destination": target.Destination, "DestinationNote": target.Note,
		}); err != nil {
			internalErr(c, err)
		}
		return
	}

	if c.PostForm("decision") != "approve" {
		redirectOAuthResult(c, in.RedirectURI, in.State, "", "access_denied")
		return
	}
	code, err := auth.RandomURLToken()
	if err != nil {
		internalErr(c, err)
		return
	}
	if err := h.deps.stateStore.PutCode(c.Request.Context(), code, auth.LoginState{
		Mode: "mcp", CodeChallenge: in.CodeChallenge, UserID: current(c).ID,
		OAuthClientID: in.ClientID, OAuthRedirectURI: in.RedirectURI,
		OAuthResource: in.Resource, OAuthScope: firstNonEmpty(in.Scope, mcpScope),
		ExpiresAt: time.Now().Add(mcpAuthorizationCodeTTLHint),
	}); err != nil {
		internalErr(c, err)
		return
	}
	redirectOAuthResult(c, in.RedirectURI, in.State, code, "")
}

func redirectOAuthResult(c *gin.Context, redirectURI, state, code, oauthErr string) {
	u, _ := url.Parse(redirectURI) // validated against the signed client registration
	q := u.Query()
	if code != "" {
		q.Set("code", code)
	}
	if oauthErr != "" {
		q.Set("error", oauthErr)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

func (h handlers) mcpToken(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMCPAuthRequestBytes)
	in, requestErr := parseMCPTokenRequest(c)
	if requestErr != nil {
		oauthError(c, requestErr.Status, requestErr.Code, requestErr.Description)
		return
	}
	if _, ok := h.parseMCPClient(in.ClientID); !ok {
		oauthError(c, http.StatusBadRequest, "invalid_client", "unknown or expired client_id")
		return
	}
	st, ok, err := h.deps.stateStore.TakeCode(c.Request.Context(), in.Code)
	if err != nil {
		internalErr(c, err)
		return
	}
	if !h.validMCPCodeGrant(st, ok, in) {
		incAuth("mcp_exchange", "failure")
		oauthError(c, http.StatusBadRequest, "invalid_grant", "authorization code is invalid, expired, or does not match the PKCE request")
		return
	}
	token := auth.NewMCPAccessToken(st.UserID, in.Resource, h.deps.sessionSecret, mcpAccessTokenTTL)
	incAuth("mcp_exchange", "success")
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, mcpTokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(mcpAccessTokenTTL / time.Second),
		Scope:       firstNonEmpty(st.OAuthScope, mcpScope),
	})
}

func parseMCPTokenRequest(c *gin.Context) (mcpTokenRequest, *oauthRequestError) {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/x-www-form-urlencoded") {
		return mcpTokenRequest{}, &oauthRequestError{http.StatusUnsupportedMediaType, "invalid_request", "token requests must be form encoded"}
	}
	if c.PostForm("grant_type") != "authorization_code" {
		return mcpTokenRequest{}, &oauthRequestError{http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported"}
	}
	in := mcpTokenRequest{
		Code: c.PostForm("code"), CodeVerifier: c.PostForm("code_verifier"),
		ClientID: c.PostForm("client_id"), RedirectURI: c.PostForm("redirect_uri"), Resource: c.PostForm("resource"),
	}
	if in.Code == "" || in.CodeVerifier == "" || in.ClientID == "" || in.RedirectURI == "" || in.Resource == "" {
		return mcpTokenRequest{}, &oauthRequestError{http.StatusBadRequest, "invalid_request", "code, code_verifier, client_id, redirect_uri, and resource are required"}
	}
	return in, nil
}

func (h handlers) validMCPCodeGrant(st auth.LoginState, found bool, in mcpTokenRequest) bool {
	return found && st.Mode == "mcp" && st.OAuthClientID == in.ClientID &&
		st.OAuthRedirectURI == in.RedirectURI && st.OAuthResource == in.Resource &&
		in.Resource == h.mcpResource() && auth.VerifyPKCES256(in.CodeVerifier, st.CodeChallenge)
}

func oauthError(c *gin.Context, status int, code, description string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, oauthErrorResponse{Error: code, ErrorDescription: description})
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func mcpResourceMetadataURL(appOrigin string) string {
	return strings.TrimRight(appOrigin, "/") + "/.well-known/oauth-protected-resource/mcp"
}

func mcpWWWAuthenticate(appOrigin string) string {
	return `Bearer resource_metadata="` + mcpResourceMetadataURL(appOrigin) + `"`
}
