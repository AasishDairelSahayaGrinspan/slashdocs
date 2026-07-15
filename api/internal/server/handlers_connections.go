package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/connections"
	"github.com/razorpay/aidocs/api/internal/knowledge"
)

const (
	connectionStateTTL = 10 * time.Minute
	maxProviderBody    = 2 << 20
)

type connectionResponse struct {
	ID                string             `json:"id"`
	Provider          string             `json:"provider"`
	ExternalAccountID string             `json:"external_account_id,omitempty"`
	DisplayName       string             `json:"display_name"`
	WorkspaceID       string             `json:"workspace_id,omitempty"`
	WorkspaceName     string             `json:"workspace_name,omitempty"`
	Scopes            []string           `json:"scopes,omitempty"`
	Status            connections.Status `json:"status"`
	ExpiresAt         *time.Time         `json:"expires_at,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type connectionIdentity struct {
	Provider          string
	ExternalAccountID string
	DisplayName       string
	WorkspaceID       string
	WorkspaceName     string
	Scopes            []string
	AccessToken       string
	RefreshToken      string
	TokenType         string
	ExpiresAt         *time.Time
}

type providerToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

type slackTokenPayload struct {
	OK           bool   `json:"ok"`
	Error        string `json:"error"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Team         struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	AuthedUser struct {
		ID           string `json:"id"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	} `json:"authed_user"`
}

func (p slackTokenPayload) rotatingToken() providerToken {
	if p.AuthedUser.AccessToken != "" {
		return providerToken{
			AccessToken: p.AuthedUser.AccessToken, RefreshToken: p.AuthedUser.RefreshToken,
			TokenType: p.AuthedUser.TokenType, Scope: p.AuthedUser.Scope, ExpiresIn: p.AuthedUser.ExpiresIn,
		}
	}
	return providerToken{AccessToken: p.AccessToken, RefreshToken: p.RefreshToken, TokenType: p.TokenType, ExpiresIn: p.ExpiresIn}
}

// NewConnectionAccessTokenResolver builds the credential boundary consumed by
// provider inspectors. It verifies connection health, decrypts only for the
// duration of one request, and refreshes expiring Google credentials before
// returning an access token.
func NewConnectionAccessTokenResolver(store connections.Store, cipher *connections.Cipher, providers ConnectionProviders, client *http.Client) knowledge.AccessTokenResolver {
	return func(ctx context.Context, connectionID string) (string, error) {
		if store == nil || cipher == nil {
			return "", knowledge.ErrProviderCredentialUnavailable
		}
		account, err := store.Get(ctx, connectionID)
		if err != nil || (account.Status != connections.StatusActive && account.Status != connections.StatusDemo) {
			return "", knowledge.ErrProviderCredentialUnavailable
		}
		credentials, err := cipher.Open(account, account.CredentialCiphertext)
		if err != nil {
			return "", knowledge.ErrProviderCredentialUnavailable
		}
		if account.ExpiresAt != nil && time.Now().Add(time.Minute).After(*account.ExpiresAt) {
			h := handlers{deps: dependencies{connections: store, connectionCipher: cipher, connectionProviders: providers, connectionHTTPClient: client}}
			credentials, _, err = h.refreshConnectionCredentials(ctx, account, credentials)
			if err != nil {
				return "", knowledge.ErrProviderCredentialUnavailable
			}
		}
		return credentials.AccessToken, nil
	}
}

func safeConnection(account connections.Account) connectionResponse {
	return connectionResponse{
		ID: account.ID, Provider: account.Provider,
		ExternalAccountID: account.ExternalAccountID, DisplayName: account.DisplayName,
		WorkspaceID: account.WorkspaceID, WorkspaceName: account.WorkspaceName,
		Scopes: append([]string{}, account.Scopes...), Status: account.Status,
		ExpiresAt: account.ExpiresAt, CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
	}
}

func (h handlers) requireConnectionUser(c *gin.Context) (*auth.Principal, bool) {
	p := current(c)
	if p == nil || p.Type != auth.PrincipalUser {
		forbidden(c, "personal connections require a user account")
		return nil, false
	}
	return p, true
}

func (h handlers) listConnections(c *gin.Context) {
	p, ok := h.requireConnectionUser(c)
	if !ok {
		return
	}
	accounts, err := h.deps.connections.ListByUser(c.Request.Context(), p.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	out := make([]connectionResponse, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, safeConnection(account))
	}
	c.JSON(http.StatusOK, gin.H{"connections": out})
}

func (h handlers) startConnection(c *gin.Context) {
	p, ok := h.requireConnectionUser(c)
	if !ok {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	if provider == connections.ProviderDevRev {
		badRequest(c, "DevRev uses a personal access token")
		return
	}
	cfg, ok := h.connectionOAuthProvider(provider)
	if !ok || strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" || h.deps.connectionStates == nil || h.deps.connectionCipher == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse("connection_unavailable", "this connection is not configured", nil))
		return
	}
	redirect := c.Query("redirect")
	if redirect == "" {
		redirect = "/app/settings/connections"
	}
	if !safeWebRedirect(redirect) {
		badRequest(c, "invalid redirect")
		return
	}
	state, err := auth.RandomURLToken()
	if err != nil {
		internalErr(c, err)
		return
	}
	verifier, err := auth.RandomURLToken()
	if err != nil {
		internalErr(c, err)
		return
	}
	if provider == connections.ProviderSlack {
		// Slack OAuth v2 does not currently document PKCE for installed apps.
		verifier = ""
	}
	connectionState := auth.ConnectionState{
		Provider: provider, UserID: p.ID, Redirect: redirect,
		CodeVerifier: verifier, ExpiresAt: time.Now().Add(connectionStateTTL),
	}
	if err := h.deps.connectionStates.PutConnectionState(c.Request.Context(), state, connectionState); err != nil {
		internalErr(c, err)
		return
	}
	h.setWebOAuthStateCookie(c, state, connectionStateTTL)

	authURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		internalErr(c, errors.New("invalid connection authorization endpoint"))
		return
	}
	query := authURL.Query()
	query.Set("client_id", cfg.ClientID)
	query.Set("redirect_uri", h.connectionCallbackURL(provider))
	query.Set("response_type", "code")
	query.Set("state", state)
	switch provider {
	case connections.ProviderGitHub:
		query.Set("scope", "repo")
	case connections.ProviderSlack:
		query.Set("user_scope", "channels:history,groups:history")
	case connections.ProviderGoogleDrive:
		query.Set("scope", "openid email profile https://www.googleapis.com/auth/drive.file")
		query.Set("access_type", "offline")
		query.Set("prompt", "consent")
	}
	if verifier != "" {
		query.Set("code_challenge", pkceChallenge(verifier))
		query.Set("code_challenge_method", "S256")
	}
	authURL.RawQuery = query.Encode()
	c.Redirect(http.StatusFound, authURL.String())
}

func (h handlers) connectionCallback(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	cfg, configured := h.connectionOAuthProvider(provider)
	if !configured || h.deps.connectionStates == nil || h.deps.connectionCipher == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse("connection_unavailable", "this connection is not configured", nil))
		return
	}
	stateID := c.Query("state")
	state, ok, err := h.deps.connectionStates.TakeConnectionState(c.Request.Context(), stateID)
	if err != nil {
		internalErr(c, err)
		return
	}
	if !ok || state.Provider != provider || state.UserID == "" || !safeWebRedirect(state.Redirect) || !h.validConnectionStateBinding(c, stateID) {
		badRequest(c, "invalid oauth state")
		return
	}
	if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
		c.JSON(http.StatusUnauthorized, errorResponse("connection_failed", "provider authorization was not completed", nil))
		return
	}
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		badRequest(c, "authorization code is required")
		return
	}
	identity, err := h.exchangeConnection(c.Request.Context(), provider, cfg, code, state.CodeVerifier)
	if err != nil {
		c.JSON(http.StatusBadGateway, errorResponse("connection_failed", "provider authorization failed", nil))
		return
	}
	account, err := h.saveConnection(c.Request.Context(), state.UserID, identity)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.restoreConnectionSources(c.Request.Context(), account.ID)
	redirect := state.Redirect
	if redirect == "" {
		redirect = "/app/settings/connections"
	}
	u, _ := url.Parse(redirect)
	query := u.Query()
	query.Set("connected", account.Provider)
	u.RawQuery = query.Encode()
	c.Redirect(http.StatusFound, u.String())
}

func (h handlers) connectDevRev(c *gin.Context) {
	p, ok := h.requireConnectionUser(c)
	if !ok {
		return
	}
	if h.deps.connectionCipher == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse("connection_unavailable", "personal connections are not configured", nil))
		return
	}
	if !requireJSON(c) {
		return
	}
	var input struct {
		PersonalAccessToken string `json:"personal_access_token"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.PersonalAccessToken) == "" {
		badRequest(c, "personal_access_token is required")
		return
	}
	identity, err := h.inspectDevRevToken(c.Request.Context(), strings.TrimSpace(input.PersonalAccessToken))
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("connection_failed", "DevRev token validation failed", nil))
		return
	}
	account, err := h.saveConnection(c.Request.Context(), p.ID, identity)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.restoreConnectionSources(c.Request.Context(), account.ID)
	c.JSON(http.StatusCreated, safeConnection(account))
}

func (h handlers) disconnectConnection(c *gin.Context) {
	p, ok := h.requireConnectionUser(c)
	if !ok {
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if _, err := h.deps.connections.Disconnect(c.Request.Context(), p.ID, id); err != nil {
		if errors.Is(err, connections.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	sources, err := h.deps.knowledge.ListAllSources(c.Request.Context())
	if err != nil {
		internalErr(c, err)
		return
	}
	for _, source := range sources {
		if source.ConnectionID != id || source.Status != "active" {
			continue
		}
		source.Status = "needs_reconnect"
		source.UpdatedAt = knowledge.Now()
		if _, err := h.deps.knowledge.PutSource(c.Request.Context(), source); err != nil {
			internalErr(c, err)
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h handlers) googlePickerToken(c *gin.Context) {
	p, ok := h.requireConnectionUser(c)
	if !ok {
		return
	}
	account, err := h.deps.connections.Get(c.Request.Context(), strings.TrimSpace(c.Param("id")))
	if errors.Is(err, connections.ErrNotFound) || (err == nil && account.UserID != p.ID) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	if account.Provider != connections.ProviderGoogleDrive || account.Status != connections.StatusActive || h.deps.connectionCipher == nil {
		c.JSON(http.StatusConflict, errorResponse("connection_unavailable", "Google Drive connection must be reconnected", nil))
		return
	}
	credentials, err := h.deps.connectionCipher.Open(account, account.CredentialCiphertext)
	if err != nil {
		c.JSON(http.StatusConflict, errorResponse("connection_unavailable", "Google Drive connection must be reconnected", nil))
		return
	}
	if account.ExpiresAt != nil && time.Now().Add(time.Minute).After(*account.ExpiresAt) {
		credentials, account, err = h.refreshConnectionCredentials(c.Request.Context(), account, credentials)
		if err != nil {
			c.JSON(http.StatusConflict, errorResponse("connection_unavailable", "Google Drive connection must be reconnected", nil))
			return
		}
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"access_token": credentials.AccessToken,
		"expires_at":   account.ExpiresAt,
		"api_key":      h.deps.connectionProviders.GooglePickerAPIKey,
		"app_id":       h.deps.connectionProviders.GooglePickerAppID,
	})
}

func (h handlers) saveConnection(ctx context.Context, userID string, identity connectionIdentity) (connections.Account, error) {
	accounts, err := h.deps.connections.ListByUser(ctx, userID)
	if err != nil {
		return connections.Account{}, err
	}
	account := connections.Account{
		ID: knowledgeID("conn"), UserID: userID, Provider: identity.Provider,
		ExternalAccountID: identity.ExternalAccountID, DisplayName: identity.DisplayName,
		WorkspaceID: identity.WorkspaceID, WorkspaceName: identity.WorkspaceName,
		Scopes: identity.Scopes, Status: connections.StatusActive, ExpiresAt: identity.ExpiresAt,
	}
	var oldCredentials connections.Credentials
	for _, existing := range accounts {
		if existing.Provider == account.Provider && existing.ExternalAccountID == account.ExternalAccountID && existing.WorkspaceID == account.WorkspaceID {
			account.ID = existing.ID
			account.CreatedAt = existing.CreatedAt
			if h.deps.connectionCipher != nil && len(existing.CredentialCiphertext) > 0 {
				oldCredentials, _ = h.deps.connectionCipher.Open(existing, existing.CredentialCiphertext)
			}
			break
		}
	}
	if identity.RefreshToken == "" {
		identity.RefreshToken = oldCredentials.RefreshToken
	}
	account.CredentialCiphertext, err = h.deps.connectionCipher.Seal(account, connections.Credentials{
		AccessToken: identity.AccessToken, RefreshToken: identity.RefreshToken, TokenType: identity.TokenType,
	})
	if err != nil {
		return connections.Account{}, err
	}
	return h.deps.connections.Put(ctx, account)
}

func (h handlers) restoreConnectionSources(ctx context.Context, connectionID string) {
	sources, err := h.deps.knowledge.ListAllSources(ctx)
	if err != nil {
		return
	}
	for _, source := range sources {
		if source.ConnectionID != connectionID || source.Status != "needs_reconnect" {
			continue
		}
		source.Status = "active"
		source.Freshness.State = "stale"
		source.UpdatedAt = knowledge.Now()
		_, _ = h.deps.knowledge.PutSource(ctx, source)
	}
}

func (h handlers) exchangeConnection(ctx context.Context, provider string, cfg ConnectionOAuthProvider, code, verifier string) (connectionIdentity, error) {
	if provider == connections.ProviderSlack {
		return h.exchangeSlack(ctx, cfg, code)
	}
	values := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {cfg.ClientID},
		"client_secret": {cfg.ClientSecret}, "code": {code},
		"redirect_uri": {h.connectionCallbackURL(provider)},
	}
	if verifier != "" {
		values.Set("code_verifier", verifier)
	}
	var token providerToken
	if err := h.providerForm(ctx, cfg.TokenURL, values, &token); err != nil || token.Error != "" || token.AccessToken == "" {
		return connectionIdentity{}, errors.New("provider token exchange failed")
	}
	expiresAt := tokenExpiry(token.ExpiresIn)
	switch provider {
	case connections.ProviderGitHub:
		var profile struct {
			ID    json.Number `json:"id"`
			Login string      `json:"login"`
			Name  string      `json:"name"`
		}
		if err := h.providerJSON(ctx, http.MethodGet, strings.TrimRight(cfg.APIBaseURL, "/")+"/user", token.AccessToken, nil, &profile); err != nil || profile.ID.String() == "" {
			return connectionIdentity{}, errors.New("provider identity lookup failed")
		}
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			name = profile.Login
		}
		return connectionIdentity{Provider: provider, ExternalAccountID: profile.ID.String(), DisplayName: name, Scopes: splitScopes(token.Scope), AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType, ExpiresAt: expiresAt}, nil
	case connections.ProviderGoogleDrive:
		var profile struct {
			Sub   string `json:"sub"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		if err := h.providerJSON(ctx, http.MethodGet, strings.TrimRight(cfg.APIBaseURL, "/")+"/userinfo", token.AccessToken, nil, &profile); err != nil || profile.Sub == "" {
			return connectionIdentity{}, errors.New("provider identity lookup failed")
		}
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			name = profile.Email
		}
		return connectionIdentity{Provider: provider, ExternalAccountID: profile.Sub, DisplayName: name, WorkspaceName: profile.Email, Scopes: splitScopes(token.Scope), AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType, ExpiresAt: expiresAt}, nil
	default:
		return connectionIdentity{}, errors.New("unsupported provider")
	}
}

func (h handlers) exchangeSlack(ctx context.Context, cfg ConnectionOAuthProvider, code string) (connectionIdentity, error) {
	values := url.Values{"client_id": {cfg.ClientID}, "client_secret": {cfg.ClientSecret}, "code": {code}, "redirect_uri": {h.connectionCallbackURL(connections.ProviderSlack)}}
	var payload slackTokenPayload
	if err := h.providerForm(ctx, cfg.TokenURL, values, &payload); err != nil || !payload.OK || payload.AuthedUser.ID == "" || payload.AuthedUser.AccessToken == "" {
		return connectionIdentity{}, errors.New("provider token exchange failed")
	}
	return connectionIdentity{
		Provider: connections.ProviderSlack, ExternalAccountID: payload.AuthedUser.ID,
		DisplayName: payload.AuthedUser.ID, WorkspaceID: payload.Team.ID, WorkspaceName: payload.Team.Name,
		Scopes: splitScopes(payload.AuthedUser.Scope), AccessToken: payload.AuthedUser.AccessToken,
		RefreshToken: payload.AuthedUser.RefreshToken, TokenType: payload.AuthedUser.TokenType,
		ExpiresAt: tokenExpiry(payload.AuthedUser.ExpiresIn),
	}, nil
}

func (h handlers) inspectDevRevToken(ctx context.Context, token string) (connectionIdentity, error) {
	base := strings.TrimRight(strings.TrimSpace(h.deps.connectionProviders.DevRevAPIBaseURL), "/")
	if base == "" {
		base = "https://api.devrev.ai"
	}
	var payload struct {
		Organization struct {
			ID        string `json:"id"`
			DisplayID string `json:"display_id"`
			Slug      string `json:"slug"`
		} `json:"organization"`
		Token struct {
			Expiry int64 `json:"expiry"`
		} `json:"token"`
		User struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			FullName    string `json:"full_name"`
			Email       string `json:"email"`
		} `json:"user"`
		// Older DevRev API revisions used these shapes. Keeping decode-only
		// compatibility makes reconnects safe during an API rollout.
		ExpiresAt string `json:"expires_at"`
		CreatedBy struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			FullName    string `json:"full_name"`
		} `json:"created_by"`
		DevUser struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			FullName    string `json:"full_name"`
		} `json:"dev_user"`
		DevOrg struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"dev_org"`
	}
	if err := h.providerJSON(ctx, http.MethodPost, base+"/auth-tokens.info", token, strings.NewReader("{}"), &payload); err != nil {
		return connectionIdentity{}, err
	}
	id := connectionFirstNonEmpty(payload.User.ID, payload.DevUser.ID, payload.CreatedBy.ID)
	if id == "" {
		return connectionIdentity{}, errors.New("provider identity lookup failed")
	}
	name := connectionFirstNonEmpty(payload.User.DisplayName, payload.User.FullName, payload.User.Email, payload.DevUser.DisplayName, payload.DevUser.FullName, payload.CreatedBy.DisplayName, payload.CreatedBy.FullName, id)
	var expiresAt *time.Time
	if payload.Token.Expiry > 0 {
		parsed := time.Unix(payload.Token.Expiry, 0).UTC()
		expiresAt = &parsed
		if time.Now().After(parsed) {
			return connectionIdentity{}, errors.New("provider token expired")
		}
	} else if parsed, err := time.Parse(time.RFC3339, payload.ExpiresAt); err == nil {
		parsed = parsed.UTC()
		expiresAt = &parsed
		if time.Now().After(parsed) {
			return connectionIdentity{}, errors.New("provider token expired")
		}
	}
	workspaceID := connectionFirstNonEmpty(payload.Organization.ID, payload.DevOrg.ID)
	workspaceName := connectionFirstNonEmpty(payload.Organization.Slug, payload.Organization.DisplayID, payload.DevOrg.DisplayName)
	return connectionIdentity{Provider: connections.ProviderDevRev, ExternalAccountID: id, DisplayName: name, WorkspaceID: workspaceID, WorkspaceName: workspaceName, Scopes: []string{}, AccessToken: token, TokenType: "Bearer", ExpiresAt: expiresAt}, nil
}

func (h handlers) refreshConnectionCredentials(ctx context.Context, account connections.Account, credentials connections.Credentials) (connections.Credentials, connections.Account, error) {
	if credentials.RefreshToken == "" {
		return connections.Credentials{}, account, errors.New("refresh token unavailable")
	}
	cfg, ok := h.connectionOAuthProvider(account.Provider)
	if !ok {
		return connections.Credentials{}, account, errors.New("provider OAuth unavailable")
	}
	values := url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {credentials.RefreshToken},
		"client_id": {cfg.ClientID}, "client_secret": {cfg.ClientSecret},
	}
	var token providerToken
	if account.Provider == connections.ProviderSlack {
		var payload slackTokenPayload
		if err := h.providerForm(ctx, cfg.TokenURL, values, &payload); err != nil || !payload.OK || payload.Error != "" {
			return connections.Credentials{}, account, errors.New("provider token refresh failed")
		}
		token = payload.rotatingToken()
	} else if err := h.providerForm(ctx, cfg.TokenURL, values, &token); err != nil || token.Error != "" {
		return connections.Credentials{}, account, errors.New("provider token refresh failed")
	}
	if token.AccessToken == "" {
		return connections.Credentials{}, account, errors.New("provider token refresh failed")
	}
	expectedUpdatedAt := account.UpdatedAt
	credentials.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		credentials.RefreshToken = token.RefreshToken
	}
	if token.TokenType != "" {
		credentials.TokenType = token.TokenType
	}
	account.ExpiresAt = tokenExpiry(token.ExpiresIn)
	ciphertext, err := h.deps.connectionCipher.Seal(account, credentials)
	if err != nil {
		return connections.Credentials{}, account, err
	}
	account.CredentialCiphertext = ciphertext
	updated, err := h.deps.connections.RefreshCredentials(ctx, account, expectedUpdatedAt)
	return credentials, updated, err
}

func (h handlers) providerForm(ctx context.Context, endpoint string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return h.doProviderRequest(req, out)
}

func (h handlers) providerJSON(ctx context.Context, method, endpoint, token string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return h.doProviderRequest(req, out)
}

func (h handlers) doProviderRequest(req *http.Request, out any) error {
	client := h.deps.connectionHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxProviderBody))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func (h handlers) connectionOAuthProvider(provider string) (ConnectionOAuthProvider, bool) {
	var cfg ConnectionOAuthProvider
	switch provider {
	case connections.ProviderGitHub:
		cfg = h.deps.connectionProviders.GitHub
		cfg.AuthURL = valueOrString(cfg.AuthURL, "https://github.com/login/oauth/authorize")
		cfg.TokenURL = valueOrString(cfg.TokenURL, "https://github.com/login/oauth/access_token")
		cfg.APIBaseURL = valueOrString(cfg.APIBaseURL, "https://api.github.com")
	case connections.ProviderSlack:
		cfg = h.deps.connectionProviders.Slack
		cfg.AuthURL = valueOrString(cfg.AuthURL, "https://slack.com/oauth/v2/authorize")
		cfg.TokenURL = valueOrString(cfg.TokenURL, "https://slack.com/api/oauth.v2.access")
		cfg.APIBaseURL = valueOrString(cfg.APIBaseURL, "https://slack.com/api")
	case connections.ProviderGoogleDrive:
		cfg = h.deps.connectionProviders.GoogleDrive
		cfg.AuthURL = valueOrString(cfg.AuthURL, "https://accounts.google.com/o/oauth2/v2/auth")
		cfg.TokenURL = valueOrString(cfg.TokenURL, "https://oauth2.googleapis.com/token")
		cfg.APIBaseURL = valueOrString(cfg.APIBaseURL, "https://openidconnect.googleapis.com/v1")
	default:
		return ConnectionOAuthProvider{}, false
	}
	return cfg, true
}

func (h handlers) connectionCallbackURL(provider string) string {
	return strings.TrimRight(h.deps.appOrigin, "/") + "/v1/connections/" + provider + "/callback"
}

func (h handlers) validConnectionStateBinding(c *gin.Context, state string) bool {
	cookie, err := c.Request.Cookie(webOAuthStateCookieName(state, secureCookiesForOrigin(h.deps.appOrigin)))
	h.setWebOAuthStateCookie(c, state, -1)
	if err != nil {
		return false
	}
	boundState, ok := (auth.SessionCodec{Secret: []byte(h.deps.sessionSecret)}).VerifyAudience(cookie.Value, webOAuthStateAudience)
	return ok && boundState == state
}

func pkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func splitScopes(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" && !seen[field] {
			seen[field] = true
			out = append(out, field)
		}
	}
	return out
}

func tokenExpiry(seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}
	expires := time.Now().Add(time.Duration(seconds) * time.Second).UTC()
	return &expires
}

func valueOrString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimRight(value, "/")
}

func connectionFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
