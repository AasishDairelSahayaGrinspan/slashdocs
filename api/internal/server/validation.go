package server

import (
	"net/url"
	"strings"

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
