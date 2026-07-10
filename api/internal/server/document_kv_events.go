package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/repo"
)

const (
	documentKVReplayLimit = 100
	documentKVPollEvery   = 2 * time.Second
	documentKVKeepAlive   = 25 * time.Second
)

type documentKVEventHub struct {
	mu   sync.Mutex
	next uint64
	subs map[string]map[uint64]chan repo.DocumentKVEvent
}

func newDocumentKVEventHub() *documentKVEventHub {
	return &documentKVEventHub{subs: map[string]map[uint64]chan repo.DocumentKVEvent{}}
}

func (h *documentKVEventHub) subscribe(documentID string) (<-chan repo.DocumentKVEvent, func()) {
	ch := make(chan repo.DocumentKVEvent, 32)
	h.mu.Lock()
	h.next++
	id := h.next
	if h.subs[documentID] == nil {
		h.subs[documentID] = map[uint64]chan repo.DocumentKVEvent{}
	}
	h.subs[documentID][id] = ch
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			if subs := h.subs[documentID]; subs != nil {
				if _, ok := subs[id]; ok {
					delete(subs, id)
					close(ch)
				}
				if len(subs) == 0 {
					delete(h.subs, documentID)
				}
			}
			h.mu.Unlock()
		})
	}
	return ch, unsubscribe
}

func (h *documentKVEventHub) publish(event repo.DocumentKVEvent) {
	if event.DocumentID == "" || len(event.Changes) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.subs[event.DocumentID] {
		select {
		case ch <- event:
		default:
			delete(h.subs[event.DocumentID], id)
			close(ch)
		}
	}
	if len(h.subs[event.DocumentID]) == 0 {
		delete(h.subs, event.DocumentID)
	}
}

func (h handlers) streamDocumentKVEvents(c *gin.Context) {
	docID := c.Param("id")
	if !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}
	afterRevision, err := parseKVEventRevision(c.Query("after_revision"))
	if err != nil {
		badRequest(c, "invalid after_revision")
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		internalErr(c, errors.New("streaming unsupported"))
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	liveEvents, unsubscribe := h.deps.documentKVEvents.subscribe(docID)
	defer unsubscribe()

	lastSent := afterRevision
	sendBacklog := func() error {
		for {
			events, err := h.deps.repository.ListDocumentKVEvents(c.Request.Context(), docID, lastSent, documentKVReplayLimit)
			if err != nil {
				return err
			}
			for _, event := range events {
				if event.Revision <= lastSent {
					continue
				}
				if err := writeDocumentKVSSE(c.Writer, event); err != nil {
					return err
				}
				lastSent = event.Revision
			}
			flusher.Flush()
			if len(events) < documentKVReplayLimit {
				return nil
			}
		}
	}
	if err := sendBacklog(); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	if _, err := fmt.Fprint(c.Writer, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	poll := time.NewTicker(documentKVPollEvery)
	defer poll.Stop()
	keepAlive := time.NewTicker(documentKVKeepAlive)
	defer keepAlive.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-liveEvents:
			if !ok {
				return
			}
			if event.Revision <= lastSent {
				continue
			}
			if event.Revision > lastSent+1 {
				if err := sendBacklog(); err != nil {
					return
				}
				if event.Revision <= lastSent {
					continue
				}
			}
			if err := writeDocumentKVSSE(c.Writer, event); err != nil {
				return
			}
			lastSent = event.Revision
			flusher.Flush()
		case <-poll.C:
			if err := sendBacklog(); err != nil {
				return
			}
		case <-keepAlive.C:
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
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
	_, err = fmt.Fprintf(w, "data: %s\n\n", raw)
	return err
}
