package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

const githubAPIBase = "https://api.github.com"

func (h handlers) githubStatus(c *gin.Context) {
	if current(c).Type != auth.PrincipalUser {
		c.JSON(http.StatusForbidden, errorResponse("forbidden", "GitHub can only be connected by a user", nil))
		return
	}
	conn, err := h.deps.repository.GetGitHubConnection(c.Request.Context(), current(c).ID)
	if errors.Is(err, repo.ErrNotFound) {
		c.JSON(http.StatusOK, gin.H{"connected": false, "configured": h.deps.githubOAuth != nil})
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": true, "configured": h.deps.githubOAuth != nil, "login": conn.Login, "avatar_url": conn.AvatarURL, "updated_at": conn.UpdatedAt})
}

func (h handlers) githubOAuthStart(c *gin.Context) {
	if h.deps.githubOAuth == nil {
		c.JSON(http.StatusServiceUnavailable, errorResponse("github_not_configured", "GitHub OAuth is not configured", nil))
		return
	}
	if current(c).Type != auth.PrincipalUser {
		c.JSON(http.StatusForbidden, errorResponse("forbidden", "GitHub can only be connected by a user", nil))
		return
	}
	redirect := c.DefaultQuery("redirect", "/app/profile")
	if !safeWebRedirect(redirect) {
		badRequest(c, "invalid redirect")
		return
	}
	state, err := auth.RandomURLToken()
	if err != nil {
		internalErr(c, err)
		return
	}
	if err := h.deps.stateStore.PutState(c.Request.Context(), state, auth.LoginState{Mode: "github", UserID: current(c).ID, Redirect: redirect, ExpiresAt: time.Now().Add(10 * time.Minute)}); err != nil {
		internalErr(c, err)
		return
	}
	c.Redirect(http.StatusFound, h.deps.githubOAuth.AuthCodeURL(state))
}

func (h handlers) githubOAuthCallback(c *gin.Context) {
	if h.deps.githubOAuth == nil {
		notFound(c)
		return
	}
	state, ok, err := h.deps.stateStore.TakeState(c.Request.Context(), c.Query("state"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if !ok || state.Mode != "github" || state.UserID != current(c).ID {
		badRequest(c, "invalid oauth state")
		return
	}
	// Every GitHub call, including the token exchange, goes through the
	// configured client.
	ctx := context.WithValue(c.Request.Context(), oauth2.HTTPClient, h.deps.githubHTTPClient)
	token, err := h.deps.githubOAuth.Exchange(ctx, c.Query("code"))
	if err != nil {
		c.JSON(http.StatusBadGateway, errorResponse("github_oauth_failed", "GitHub authorization failed", nil))
		return
	}
	var user struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := h.githubJSON(c.Request.Context(), token.AccessToken, "/user", &user); err != nil {
		c.JSON(http.StatusBadGateway, errorResponse("github_profile_failed", "Could not read the GitHub profile", nil))
		return
	}
	ciphertext, err := sealGitHubToken(h.deps.sessionSecret, token.AccessToken)
	if err != nil {
		internalErr(c, err)
		return
	}
	if err := h.deps.repository.UpsertGitHubConnection(c.Request.Context(), repo.GitHubConnection{UserID: current(c).ID, GitHubUserID: user.ID, Login: user.Login, AvatarURL: user.AvatarURL, AccessTokenCiphertext: ciphertext}); err != nil {
		internalErr(c, err)
		return
	}
	// nosemgrep: go.gin.ssrf.gin-tainted-url-host.gin-tainted-url-host - Redirect was validated by safeWebRedirect before this server-side OAuth state was stored.
	c.Redirect(http.StatusFound, state.Redirect)
}

func (h handlers) githubDisconnect(c *gin.Context) {
	if current(c).Type != auth.PrincipalUser {
		c.JSON(http.StatusForbidden, errorResponse("forbidden", "GitHub can only be connected by a user", nil))
		return
	}
	// Consents approve spending this connection's token, so they must not
	// outlive it: reconnecting would otherwise silently re-arm every document
	// that was approved before.
	if err := h.deps.repository.DeleteGitHubRepoConsents(c.Request.Context(), current(c).ID); err != nil {
		internalErr(c, err)
		return
	}
	err := h.deps.repository.DeleteGitHubConnection(c.Request.Context(), current(c).ID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type githubRawRequest struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Ref   string `json:"ref"`
}

// GitHub logins are alphanumeric with single, non-leading and non-trailing
// hyphens; repository names also allow dots and underscores. Both are bounded
// (39 and 100 characters) and are validated here rather than trusted from the
// caller, because a consent row is only meaningful if the coordinates it
// names are the ones GitHub will resolve.
var (
	githubOwnerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:-?[A-Za-z0-9])*$`)
	githubRepoPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// normalizeGitHubRepoRef validates repository coordinates and returns them
// lowercased. GitHub treats owner and repository names as case-insensitive, so
// consent has to be recorded and compared in a single spelling; otherwise
// approving "acme/docs" would leave "Acme/Docs" unapproved but readable.
func normalizeGitHubRepoRef(owner, repoName string) (string, string, bool) {
	if len(owner) > 39 || !githubOwnerPattern.MatchString(owner) {
		return "", "", false
	}
	if len(repoName) > 100 || repoName == "." || repoName == ".." || !githubRepoPattern.MatchString(repoName) {
		return "", "", false
	}
	return strings.ToLower(owner), strings.ToLower(repoName), true
}

// bindGitHubRawRequest reads and validates the repository coordinates,
// answering the client itself when they are unusable.
func bindGitHubRawRequest(c *gin.Context) (githubRawRequest, bool) {
	var in githubRawRequest
	if err := c.ShouldBindJSON(&in); err != nil || in.Owner == "" || in.Repo == "" || in.Path == "" || in.Ref == "" {
		badRequest(c, "owner, repo, path, and ref are required")
		return githubRawRequest{}, false
	}
	if _, _, ok := normalizeGitHubRepoRef(in.Owner, in.Repo); !ok {
		badRequest(c, "invalid repository owner or name")
		return githubRawRequest{}, false
	}
	if strings.HasPrefix(in.Path, "/") || strings.Contains(in.Path, "..") {
		badRequest(c, "invalid repository path")
		return githubRawRequest{}, false
	}
	return in, true
}

// githubRepoConsentRequest is the body of a consent grant.
type githubRepoConsentRequest struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

// githubConsentDetails names the repository a document still needs approval
// for, so the app shell can prompt for exactly that repository.
type githubConsentDetails struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

// githubConsentUser returns the calling user, answering the client when the
// caller is not one: consent spends a GitHub connection, and only users can
// connect GitHub.
func githubConsentUser(c *gin.Context) (*auth.Principal, bool) {
	p := current(c)
	if p.Type != auth.PrincipalUser {
		c.JSON(http.StatusForbidden, errorResponse("forbidden", "GitHub repository access can only be approved by a user", nil))
		return nil, false
	}
	return p, true
}

// bindGitHubRepoConsent reads owner and repo for a consent grant, answering
// the client itself when they are unusable.
func bindGitHubRepoConsent(c *gin.Context) (string, string, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var in githubRepoConsentRequest
	if err := c.ShouldBindJSON(&in); err != nil || in.Owner == "" || in.Repo == "" {
		badRequest(c, "owner and repo are required")
		return "", "", false
	}
	owner, repoName, ok := normalizeGitHubRepoRef(in.Owner, in.Repo)
	if !ok {
		badRequest(c, "invalid repository owner or name")
		return "", "", false
	}
	return owner, repoName, true
}

// needGitHubRepoConsent reports whether the caller has approved this document
// reading this repository. A viewer role on the document is not that approval:
// the document's own JavaScript picks the coordinates and the server spends
// the reader's GitHub token, so every repository is approved once, by the
// reader, in trusted app UI.
func (h handlers) needGitHubRepoConsent(c *gin.Context, documentID, ownerName, repoName string) bool {
	user, ok := githubConsentUser(c)
	if !ok {
		return false
	}
	owner, name, ok := normalizeGitHubRepoRef(ownerName, repoName)
	if !ok {
		badRequest(c, "invalid repository owner or name")
		return false
	}
	granted, err := h.deps.repository.HasGitHubRepoConsent(c.Request.Context(), user.ID, documentID, owner, name)
	if err != nil {
		internalErr(c, err)
		return false
	}
	if !granted {
		c.JSON(http.StatusForbidden, errorResponse(
			"github_consent_required",
			"Approve this document reading "+owner+"/"+name+" before it can load repository content",
			githubConsentDetails{Owner: owner, Repo: name},
		))
		return false
	}
	return true
}

// listGitHubRepoConsents godoc
// @Summary List the repositories you approved this document to read
// @Description Consents are per user and per document. Another reader's approvals are never returned.
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Success 200 {object} listResponse[repo.GitHubRepoConsent]
// @Router /v1/documents/{id}/github/consents [get]
func (h handlers) listGitHubRepoConsents(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	user, ok := githubConsentUser(c)
	if !ok {
		return
	}
	items, err := h.deps.repository.ListGitHubRepoConsents(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[repo.GitHubRepoConsent]{Items: items})
}

// grantGitHubRepoConsent godoc
// @Summary Approve this document reading one GitHub repository
// @Description Records that you allow this document to read files from owner/repo with your connected GitHub account. Owner and repo are stored lowercased. Repeating a grant is idempotent.
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Param id path string true "Document ID"
// @Param body body githubRepoConsentRequest true "Repository owner and name"
// @Success 201 {object} repo.GitHubRepoConsent
// @Router /v1/documents/{id}/github/consents [post]
func (h handlers) grantGitHubRepoConsent(c *gin.Context) {
	// JSON is required so a cross-origin form or text/plain POST, which a
	// browser sends without a preflight, cannot record an approval.
	if !requireJSON(c) {
		return
	}
	if !h.needDocRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	user, ok := githubConsentUser(c)
	if !ok {
		return
	}
	owner, repoName, ok := bindGitHubRepoConsent(c)
	if !ok {
		return
	}
	consent, err := h.deps.repository.GrantGitHubRepoConsent(c.Request.Context(), user.ID, c.Param("id"), owner, repoName)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, consent)
}

// revokeGitHubRepoConsent godoc
// @Summary Withdraw your approval for one repository
// @Description Idempotent: withdrawing an approval that is not recorded also succeeds.
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param owner path string true "Repository owner"
// @Param repo path string true "Repository name"
// @Success 204 "No Content"
// @Router /v1/documents/{id}/github/consents/{owner}/{repo} [delete]
func (h handlers) revokeGitHubRepoConsent(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	user, ok := githubConsentUser(c)
	if !ok {
		return
	}
	owner, repoName, ok := normalizeGitHubRepoRef(c.Param("owner"), c.Param("repo"))
	if !ok {
		badRequest(c, "invalid repository owner or name")
		return
	}
	if err := h.deps.repository.RevokeGitHubRepoConsent(c.Request.Context(), user.ID, c.Param("id"), owner, repoName); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// githubTokenFor opens the caller's stored GitHub token, answering the client
// when GitHub is not connected or the stored token cannot be decrypted.
func (h handlers) githubTokenFor(c *gin.Context) (string, bool) {
	conn, err := h.deps.repository.GetGitHubConnection(c.Request.Context(), current(c).ID)
	if errors.Is(err, repo.ErrNotFound) {
		c.JSON(http.StatusPreconditionRequired, errorResponse("github_not_connected", "Connect GitHub before loading repository content", nil))
		return "", false
	}
	if err != nil {
		internalErr(c, err)
		return "", false
	}
	token, err := openGitHubToken(h.deps.sessionSecret, conn.AccessTokenCiphertext)
	if err != nil {
		internalErr(c, err)
		return "", false
	}
	return token, true
}

func (h handlers) githubRawContent(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	in, ok := bindGitHubRawRequest(c)
	if !ok {
		return
	}
	// Consent is checked before the connection is opened: document JavaScript
	// picks these coordinates, so an unapproved repository must not even
	// reveal whether the reader has connected GitHub.
	if !h.needGitHubRepoConsent(c, c.Param("id"), in.Owner, in.Repo) {
		return
	}
	token, ok := h.githubTokenFor(c)
	if !ok {
		return
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", githubAPIBase, url.PathEscape(in.Owner), url.PathEscape(in.Repo), escapeGitHubPath(in.Path), url.QueryEscape(in.Ref))
	req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := h.deps.githubHTTPClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, errorResponse("github_unavailable", "GitHub request failed", nil))
		return
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, res.Body)
		c.JSON(res.StatusCode, errorResponse("github_error", "GitHub could not return that file", nil))
		return
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxHTMLBytes+1))
	if err != nil {
		internalErr(c, err)
		return
	}
	if len(body) > maxHTMLBytes {
		c.JSON(http.StatusRequestEntityTooLarge, errorResponse("github_file_too_large", "GitHub files must be 50 MiB or smaller", nil))
		return
	}
	c.JSON(http.StatusOK, gin.H{"owner": in.Owner, "repo": in.Repo, "path": in.Path, "content": string(body), "content_type": res.Header.Get("Content-Type"), "etag": res.Header.Get("ETag"), "ref": in.Ref})
}

func (h handlers) githubJSON(ctx context.Context, token, path string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := h.deps.githubHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("github status %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}

func sealGitHubToken(secret, token string) ([]byte, error) {
	block, err := aes.NewCipher(tokenKey(secret))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(token), nil), nil
}
func openGitHubToken(secret string, sealed []byte) (string, error) {
	block, err := aes.NewCipher(tokenKey(secret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("invalid token ciphertext")
	}
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	return string(plain), err
}
func tokenKey(secret string) []byte {
	sum := sha256.Sum256([]byte("aidocs/github-token/v1\x00" + secret))
	return sum[:]
}
func escapeGitHubPath(path string) string {
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
