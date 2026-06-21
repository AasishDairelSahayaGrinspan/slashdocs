package server

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
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
