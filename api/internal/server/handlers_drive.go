package server

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// resolveSession is the non-aborting counterpart of requireAuth. It attempts to
// authenticate the request and, on success, sets the principal so current(c)
// works downstream. Unlike requireAuth it never rejects: a route mounted with
// it stays public, so an unauthenticated caller proceeds and the handler can
// bounce them to login. This lets public entry points (e.g. /drive/open) still
// recognise a valid session cookie / credential without enforcing one.
func resolveSession(a auth.Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p, err := a.Authenticate(c.Request); err == nil {
			c.Set("principal", p)
		}
		c.Next()
	}
}

// driveOpen is the Google Drive "Open with" entry point. When a user picks
// "Open with -> aidocs" on a Drive HTML file, Google redirects here with a
// URL-encoded JSON `state` param: {"ids":[fileId],"action":"open",...}.
//
// The handler parses the state, ensures the caller has an aidocs session
// (bouncing to Google login with a return-to that replays this exact URL if
// not), upserts the Drive-backed document for the file, and redirects to the
// document in the app. /drive/open is public so a logged-out user can reach it
// from Drive; resolveSession still populates current(c) when a session exists.
func (h handlers) driveOpen(c *gin.Context) {
	st, err := parseDriveState(c.Query("state"))
	if err != nil || st.Action != "open" {
		badRequest(c, "invalid drive open request")
		return
	}

	// current(c) is populated by resolveSession (public, non-aborting). No
	// session => bounce to Google login with a return-to that replays this
	// exact URL after a successful web login.
	p := current(c)
	if p == nil {
		// nosemgrep: go.gin.ssrf.gin-tainted-url-host.gin-tainted-url-host - driveLoginURL builds a fixed path-relative target ("/v1/auth/google/start?..."); the state is only a query value and googleStart re-validates the nested redirect via safeWebRedirect.
		c.Redirect(http.StatusFound, driveLoginURL(c.Query("state")))
		return
	}

	fileID := st.IDs[0]
	rk := st.ResourceKeys[fileID]
	doc, err := h.deps.repository.UpsertDriveDocument(c.Request.Context(), *p, fileID, rk, "Drive document")
	if err != nil {
		internalErr(c, err)
		return
	}
	// nosemgrep: go.gin.ssrf.gin-tainted-url-host.gin-tainted-url-host - doc.ID is a server-generated id; the target is a fixed path-relative app URL with no attacker-controlled host.
	c.Redirect(http.StatusFound, "/app/d/"+doc.ID)
}

// driveLoginURL builds the Google login start URL whose post-login redirect
// returns the user to /drive/open?state=<rawState>. The nested `state` value is
// escaped via url.Values so the handler never hand-concatenates it. googleStart
// validates the web redirect with safeWebRedirect, which requires a
// path-relative target (no scheme/host), so we build it as an absolute path.
func driveLoginURL(rawState string) string {
	target := url.Values{}
	target.Set("state", rawState)
	q := url.Values{}
	q.Set("mode", authModeWeb)
	q.Set("redirect", "/drive/open?"+target.Encode())
	return "/v1/auth/google/start?" + q.Encode()
}

// driveRender wraps client-fetched Google Drive HTML in aidocs' sandbox
// renderer. The browser fetches the file bytes directly from Drive (no server
// token is ever involved) and POSTs them here; this endpoint merely confirms
// the document id refers to a Drive-backed doc and returns the wrapper page.
//
// The posted HTML is never persisted. Access is intentionally NOT gated by
// RoleForDocument: Drive ACLs gate the fetch (a user who can retrieve the bytes
// from Drive is authorised to view them), and aidocs grants are inert for Drive
// docs (Global Constraint). Requiring a valid session (the route is on the
// authed group) prevents this from being an open HTML-wrapping proxy.
func (h handlers) driveRender(c *gin.Context) {
	// Mounted on the authed group, so requireAuth has already rejected any
	// unauthenticated request; no need to re-check the session here.
	var in struct {
		DocumentID string `json:"document_id"`
		HTML       string `json:"html"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.DocumentID == "" {
		badRequest(c, "invalid render request")
		return
	}
	doc, err := h.deps.repository.GetDocument(c.Request.Context(), in.DocumentID)
	if err != nil || doc.Source != repo.SourceDrive {
		notFound(c)
		return
	}
	// Same sandbox CSP / nosniff headers as the native render path.
	appOrigin := renderSecurityHeaders(c, h.deps.appOrigin)
	observeHTML("drive_render", len(in.HTML))
	c.Data(http.StatusOK, "text/html; charset=utf-8", renderWrapperHTML([]byte(in.HTML), appOrigin))
}
