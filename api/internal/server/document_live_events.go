package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/repo"
)

// Live document events ride the document KV event stream. Writes announce
// "document" and "comments" events, and viewers publish "presence" events, so
// every open viewer on every server replica sees them without polling.

const (
	maxPresenceBytes       = 4 * 1024
	maxPresenceQuoteRunes  = 512
	maxPresenceContextLen  = 64
	maxPresenceVersionSize = 64
	liveEventWriteTimeout  = 5 * time.Second
)

var presenceSessionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// presence is one viewer session's place in a document: the version on screen
// and a short passage near the top of the viewport. The server adds identity.
type presence struct {
	Session    string       `json:"session"`
	Version    string       `json:"version,omitempty"`
	Anchor     *repo.Anchor `json:"anchor,omitempty"`
	Left       bool         `json:"left,omitempty"`
	PictureURL string       `json:"picture_url,omitempty"`
}

func (p presence) valid() bool {
	if !presenceSessionPattern.MatchString(p.Session) || len(p.Version) > maxPresenceVersionSize {
		return false
	}
	a := p.Anchor
	return a == nil || (len([]rune(a.Quote)) <= maxPresenceQuoteRunes &&
		len([]rune(a.Prefix)) <= maxPresenceContextLen &&
		len([]rune(a.Suffix)) <= maxPresenceContextLen &&
		a.StartOffset >= 0 && a.EndOffset >= a.StartOffset)
}

func (h handlers) postDocumentPresence(c *gin.Context) {
	docID := c.Param("id")
	if !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPresenceBytes)
	var in presence
	if err := c.ShouldBindJSON(&in); err != nil || !in.valid() {
		badRequest(c, "invalid presence")
		return
	}
	p := current(c)
	in.Left = false
	in.PictureURL = p.PictureURL
	if in.Anchor != nil {
		in.Anchor.DOMPath = ""
	}
	if err := h.appendPresence(c.Request.Context(), docID, in, *p); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h handlers) appendPresence(ctx context.Context, docID string, state presence, actor auth.Principal) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return h.deps.repository.AppendDocumentEvent(ctx, docID, repo.DocumentEventPresence, data, actor)
}

// leavePresence marks a viewer session as gone when its stream ends. The
// request context is already cancelled by then, so the write gets its own.
func (h handlers) leavePresence(c *gin.Context, docID, session string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), liveEventWriteTimeout)
	defer cancel()
	err := h.appendPresence(ctx, docID, presence{Session: session, Left: true}, *current(c))
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		slog.WarnContext(ctx, "document presence leave failed",
			slog.String("document_id", docID),
			slog.Any("error", err))
	}
}

// announce tells open viewers that a write changed the document or its
// comments. The write already succeeded, so a failure here is only logged.
func (h handlers) announce(c *gin.Context, kind string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), liveEventWriteTimeout)
	defer cancel()
	docID := c.Param("id")
	if err := h.deps.repository.AppendDocumentEvent(ctx, docID, kind, nil, *current(c)); err != nil {
		slog.WarnContext(ctx, "document event announcement failed",
			slog.String("document_id", docID),
			slog.String("kind", kind),
			slog.Any("error", err))
	}
}
