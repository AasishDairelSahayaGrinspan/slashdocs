package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

const (
	documentKVReplayLimit = 100
	documentKVKeepAlive   = 25 * time.Second
)

type documentKVStream struct {
	ctx          context.Context
	writer       http.ResponseWriter
	flusher      http.Flusher
	repository   documentKVStreamRepository
	authenticate func(context.Context) (*auth.Principal, error)
	principal    auth.Principal
	documentID   string
	lastSent     int64
	keepAlive    <-chan time.Time
	started      bool
}

type documentKVStreamRepository interface {
	ListDocumentKVEvents(context.Context, string, int64, int32) ([]repo.DocumentKVEvent, error)
	RoleForDocument(context.Context, auth.Principal, string) (repo.Role, error)
}

func (s *documentKVStream) sendBacklog() error {
	for {
		events, err := s.repository.ListDocumentKVEvents(s.ctx, s.documentID, s.lastSent, documentKVReplayLimit)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Revision <= s.lastSent {
				continue
			}
			s.started = true
			if err := writeDocumentKVSSE(s.writer, event); err != nil {
				return err
			}
			s.lastSent = event.Revision
		}
		s.flusher.Flush()
		if len(events) < documentKVReplayLimit {
			return nil
		}
	}
}

func (s *documentKVStream) sendLive(event repo.DocumentKVEvent) error {
	if event.Revision <= s.lastSent {
		return nil
	}
	if event.Revision > s.lastSent+1 {
		if err := s.sendBacklog(); err != nil {
			return err
		}
		if event.Revision <= s.lastSent {
			return nil
		}
	}
	s.started = true
	if err := writeDocumentKVSSE(s.writer, event); err != nil {
		return err
	}
	s.lastSent = event.Revision
	s.flusher.Flush()
	return nil
}

func (s *documentKVStream) reauthorize() bool {
	if s.authenticate == nil {
		return false
	}
	principal, err := s.authenticate(s.ctx)
	if err != nil {
		if s.ctx.Err() == nil && !errors.Is(err, auth.ErrUnauthorized) {
			slog.ErrorContext(s.ctx, "document KV stream credential check failed",
				slog.String("document_id", s.documentID),
				slog.Any("error", err))
		}
		return false
	}
	if principal == nil || principal.Type != s.principal.Type || principal.ID != s.principal.ID {
		return false
	}
	role, err := s.repository.RoleForDocument(s.ctx, s.principal, s.documentID)
	if err != nil {
		if s.ctx.Err() == nil {
			slog.ErrorContext(s.ctx, "document KV stream authorization check failed",
				slog.String("document_id", s.documentID),
				slog.Any("error", err))
		}
		return false
	}
	return atLeast(role, repo.RoleViewer)
}

func (s *documentKVStream) run(liveEvents <-chan repo.DocumentKVEvent) {
	keepAlive := s.keepAlive
	var ticker *time.Ticker
	if keepAlive == nil {
		ticker = time.NewTicker(documentKVKeepAlive)
		defer ticker.Stop()
		keepAlive = ticker.C
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-liveEvents:
			if !ok {
				return
			}
			if err := s.sendLive(event); err != nil {
				return
			}
		case <-keepAlive:
			if !s.reauthorize() {
				return
			}
			if _, err := fmt.Fprint(s.writer, ": keepalive\n\n"); err != nil {
				return
			}
			s.flusher.Flush()
		}
	}
}

func (h handlers) streamDocumentKVEvents(c *gin.Context) {
	docID := c.Param("id")
	if !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}
	afterRevision, err := parseKVEventRevision(c.Query("after_revision"))
	if err != nil {
		badRequest(c, "invalid event revision")
		return
	}
	lastEventRevision, err := parseKVEventRevision(c.GetHeader("Last-Event-ID"))
	if err != nil {
		badRequest(c, "invalid event revision")
		return
	}
	afterRevision = max(afterRevision, lastEventRevision)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		internalErr(c, errors.New("streaming unsupported"))
		return
	}
	liveEvents, unsubscribe, err := h.deps.documentKVEvents.Subscribe(c.Request.Context(), docID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	defer unsubscribe()

	// This stream outlives the server-wide read timeout; clear the connection
	// deadlines so an expiring deadline doesn't sever the SSE response. The
	// errors are ignored: recorders and exotic writers that don't support
	// deadlines never had them to begin with.
	rc := http.NewResponseController(c.Writer)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	authRequest := c.Request.Clone(c.Request.Context())
	stream := &documentKVStream{
		ctx:        c.Request.Context(),
		writer:     c.Writer,
		flusher:    flusher,
		repository: h.deps.repository,
		authenticate: func(ctx context.Context) (*auth.Principal, error) {
			return h.deps.authenticator.Authenticate(authRequest.Clone(ctx))
		},
		principal:  *current(c),
		documentID: docID,
		lastSent:   afterRevision,
	}
	if err := stream.sendBacklog(); err != nil {
		if stream.started {
			slog.ErrorContext(c.Request.Context(), "document KV stream backlog failed",
				slog.String("document_id", docID),
				slog.Any("error", err))
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	stream.started = true
	if _, err := fmt.Fprint(c.Writer, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()
	stream.run(liveEvents)
}

func parseKVEventRevision(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 0 {
		return 0, errors.New("invalid revision")
	}
	return revision, nil
}

func writeDocumentKVSSE(w http.ResponseWriter, event repo.DocumentKVEvent) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Revision, raw)
	return err
}
