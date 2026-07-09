package server

import (
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/repo"
)

var roleRank = map[repo.Role]int{repo.RoleViewer: 1, repo.RoleCommenter: 2, repo.RoleEditor: 3, repo.RoleOwner: 4}

func atLeast(have repo.Role, need repo.Role) bool {
	return roleRank[have] >= roleRank[need]
}

func safeWebRedirect(redirect string) bool {
	if redirect == "" {
		return true
	}
	if strings.Contains(redirect, "\\") {
		return false
	}
	u, err := url.Parse(redirect)
	if err != nil || u.Scheme != "" || u.Host != "" || strings.Contains(u.Path, "\\") {
		return false
	}
	return strings.HasPrefix(u.EscapedPath(), "/") && !strings.HasPrefix(u.EscapedPath(), "//")
}

func validGrantRole(role repo.Role) bool {
	return role == repo.RoleViewer || role == repo.RoleCommenter || role == repo.RoleEditor
}

func requireJSON(c *gin.Context) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		c.JSON(http.StatusUnsupportedMediaType, errorResponse("unsupported_media_type", "content-type must be application/json", nil))
		return false
	}
	return true
}

func requestHasBody(c *gin.Context) bool {
	r := c.Request
	return r.Body != nil && r.Body != http.NoBody && (r.ContentLength != 0 || len(r.TransferEncoding) > 0)
}

func hostMatchesOrigin(host, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(sanitizeHost(host), u.Host)
}

func canonicalHTTPOrigin(origin string) (string, bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

func sameHTTPOrigin(a, b string) bool {
	ca, okA := canonicalHTTPOrigin(a)
	cb, okB := canonicalHTTPOrigin(b)
	return okA && okB && strings.EqualFold(ca, cb)
}

func sanitizeHost(host string) string {
	if host == "" || strings.ContainsAny(host, "/\\ \t\r\n@") {
		return "localhost"
	}
	return host
}

func isLocalHost(host string) bool {
	host = strings.ToLower(sanitizeHost(host))
	return host == "localhost" || strings.HasPrefix(host, "localhost:") || host == "127.0.0.1" || strings.HasPrefix(host, "127.0.0.1:") || host == "[::1]" || strings.HasPrefix(host, "[::1]:")
}

func emailDomainAllowed(email string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range allowed {
		if strings.EqualFold(domain, strings.TrimSpace(d)) {
			return true
		}
	}
	return false
}
