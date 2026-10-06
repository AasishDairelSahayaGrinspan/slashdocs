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
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/repo"
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
	token, err := h.deps.githubOAuth.Exchange(c.Request.Context(), c.Query("code"))
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

// bindGitHubRawRequest reads and validates the repository coordinates,
// answering the client itself when they are unusable.
func bindGitHubRawRequest(c *gin.Context) (githubRawRequest, bool) {
	var in githubRawRequest
	if err := c.ShouldBindJSON(&in); err != nil || in.Owner == "" || in.Repo == "" || in.Path == "" || in.Ref == "" {
		badRequest(c, "owner, repo, path, and ref are required")
		return githubRawRequest{}, false
	}
	if strings.Contains(in.Owner, "/") || strings.Contains(in.Repo, "/") || strings.HasPrefix(in.Path, "/") || strings.Contains(in.Path, "..") {
		badRequest(c, "invalid repository path")
		return githubRawRequest{}, false
	}
	return in, true
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

// tokenKey keeps the pre-rename "aidocs" domain separator: changing it would
// make every stored GitHub token undecryptable.
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
