package avatar

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/razorpay/aidocs/api/internal/repo"
)

// Mock is the built-in avatar-service stand-in, enabled with
// AIDOCS_AVATAR_SERVICE_MOCK=true. It walks each review through
// queued → running → terminal on timers so the UI's convening states render
// for real, fabricates anchored comments through the aidocs comment
// pipeline, and remembers feedback in memory.
//
// TODO(avatar-service): mocked — replace with a real avatar-service API
// client once the service is live; delete nothing else.
type Mock struct {
	cfg MockConfig

	mu         sync.Mutex
	seq        int
	requests   map[string]*mockRequest      // request id → request
	idempotent map[string]string            // doc|version|avatars → request id
	votes      map[string]map[string]string // comment ref → voter → signal
}

type MockConfig struct {
	// QueueDelay is how long a review sits queued; RunDelay how long it
	// "reads". Production-ish defaults land in the spec's 5–15s window.
	QueueDelay time.Duration
	RunDelay   time.Duration
	Store      DocumentStore
}

type mockRequest struct {
	id           string
	aidocID      string
	aidocVersion string
	reviews      []*mockReview
}

type mockReview struct {
	id           string
	avatar       Avatar
	outcome      string // terminal state this review will reach
	state        string
	commentCount int
	errText      string
}

// TODO(avatar-service): mocked fixtures — the real avatar roster (consent,
// approved personas, relevance) is served by the avatar service.
var mockAvatars = []struct {
	avatar  Avatar
	outcome string
}{
	{Avatar{ID: "av_priya", DisplayName: "Priya's avatar", PersonEmail: "priya@example.com", FocusAreas: []string{"architecture", "api-design"}, RelevanceHint: "Reviewed 12 similar design docs"}, StateCommented},
	{Avatar{ID: "av_arjun", DisplayName: "Arjun's avatar", PersonEmail: "arjun@example.com", FocusAreas: []string{"security", "privacy"}, RelevanceHint: "Owns the last two threat models in this area"}, StateCommented},
	{Avatar{ID: "av_meera", DisplayName: "Meera's avatar", PersonEmail: "meera@example.com", FocusAreas: []string{"docs", "clarity"}, RelevanceHint: "Edits most specs on this team"}, StateNoComments},
	{Avatar{ID: "av_dev", DisplayName: "Dev's avatar", PersonEmail: "dev@example.com", FocusAreas: []string{"performance"}, RelevanceHint: "Profiled the adjacent hot paths"}, StateFailed},
}

// benchAvatar is the non-selectable teaser for the panel/bench phase.
var benchAvatar = Avatar{
	ID:            BenchAvatarID,
	DisplayName:   "The Bench",
	FocusAreas:    []string{"everything"},
	RelevanceHint: "Reviews every doc, no persona needed",
	ComingSoon:    true,
}

// TODO(avatar-service): mocked comments — real ones arrive via aidocs MCP
// from the service. One body per severity, cycled over the extracted quotes.
var mockFindings = []struct {
	severity string
	body     string
}{
	{repo.SeverityBlocking, "This needs a failure-mode answer before it ships: what happens when this path is unavailable?"},
	{repo.SeverityShouldFix, "The numbers here are asserted without a source — link the measurement or soften the claim."},
	{repo.SeverityConsider, "Worth a line on how this behaves at 10x the stated load; the current wording reads as unbounded."},
}

func NewMock(cfg MockConfig) *Mock {
	if cfg.QueueDelay <= 0 {
		cfg.QueueDelay = 3 * time.Second
	}
	if cfg.RunDelay <= 0 {
		cfg.RunDelay = 7 * time.Second
	}
	return &Mock{
		cfg:        cfg,
		requests:   map[string]*mockRequest{},
		idempotent: map[string]string{},
		votes:      map[string]map[string]string{},
	}
}

func (m *Mock) ListAvatars(ctx context.Context, aidocID string) (AvatarList, error) {
	items := make([]Avatar, 0, len(mockAvatars)+1)
	for _, fixture := range mockAvatars {
		items = append(items, fixture.avatar)
	}
	// The Bench renders last on every picker, always coming-soon.
	items = append(items, benchAvatar)
	return AvatarList{Items: items, MaxSelectable: 1}, nil
}

func (m *Mock) RequestReview(ctx context.Context, req ReviewRequest) (ReviewRequestStatus, error) {
	if req.AidocID == "" || req.AidocVersion == "" || len(req.AvatarIDs) == 0 {
		return ReviewRequestStatus{}, ErrInvalidRequest
	}
	fixtures := make([]struct {
		avatar  Avatar
		outcome string
	}, 0, len(req.AvatarIDs))
	for _, id := range req.AvatarIDs {
		found := false
		for _, fixture := range mockAvatars {
			if fixture.avatar.ID == id {
				fixtures = append(fixtures, fixture)
				found = true
				break
			}
		}
		if !found {
			return ReviewRequestStatus{}, fmt.Errorf("%w: %s", ErrUnknownAvatar, id)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Idempotent on (doc, version, avatar set): a repeat click returns the
	// existing request instead of convening a second review.
	sorted := append([]string(nil), req.AvatarIDs...)
	sort.Strings(sorted)
	key := req.AidocID + "|" + req.AidocVersion + "|" + strings.Join(sorted, ",")
	if id, ok := m.idempotent[key]; ok {
		return m.snapshotLocked(id)
	}

	m.seq++
	r := &mockRequest{id: fmt.Sprintf("avreq_%d", m.seq), aidocID: req.AidocID, aidocVersion: req.AidocVersion}
	for _, fixture := range fixtures {
		m.seq++
		review := &mockReview{id: fmt.Sprintf("avrev_%d", m.seq), avatar: fixture.avatar, outcome: fixture.outcome, state: StateQueued}
		r.reviews = append(r.reviews, review)
		m.scheduleLocked(r, review)
	}
	m.requests[r.id] = r
	m.idempotent[key] = r.id
	return m.snapshotLocked(r.id)
}

// scheduleLocked walks one review through the state machine on timers. Each
// avatar completes independently — never gated on the slowest.
func (m *Mock) scheduleLocked(r *mockRequest, review *mockReview) {
	time.AfterFunc(m.cfg.QueueDelay, func() {
		m.mu.Lock()
		review.state = StateRunning
		m.mu.Unlock()

		time.AfterFunc(m.cfg.RunDelay, func() {
			m.finish(r, review)
		})
	})
}

func (m *Mock) finish(r *mockRequest, review *mockReview) {
	if review.outcome != StateCommented {
		m.mu.Lock()
		review.state = review.outcome
		if review.outcome == StateFailed {
			review.errText = "avatar could not finish reading this version"
		}
		m.mu.Unlock()
		return
	}

	// The triggering HTTP request is long gone; fabrication runs standalone.
	ctx := context.Background()
	count, err := m.postComments(ctx, r, review.avatar)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		review.state = StateFailed
		review.errText = "posting review comments failed"
		return
	}
	if count == 0 {
		review.state = StateNoComments
		return
	}
	review.state = StateCommented
	review.commentCount = count
}

func (m *Mock) postComments(ctx context.Context, r *mockRequest, av Avatar) (int, error) {
	raw, err := m.cfg.Store.VersionHTML(ctx, r.aidocID, r.aidocVersion)
	if err != nil {
		return 0, err
	}
	quotes := extractQuotes(raw, len(mockFindings))
	posted := 0
	for i, quote := range quotes {
		finding := mockFindings[i%len(mockFindings)]
		if _, err := m.cfg.Store.CreateAvatarComment(ctx, r.aidocID, r.aidocVersion, quote, finding.body, av.DisplayName, finding.severity); err != nil {
			if posted == 0 {
				return 0, err
			}
			break
		}
		posted++
	}
	return posted, nil
}

func (m *Mock) GetReview(ctx context.Context, requestID string) (ReviewRequestStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked(requestID)
}

func (m *Mock) snapshotLocked(requestID string) (ReviewRequestStatus, error) {
	r, ok := m.requests[requestID]
	if !ok {
		return ReviewRequestStatus{}, ErrUnknownRequest
	}
	out := ReviewRequestStatus{RequestID: r.id, AidocID: r.aidocID, AidocVersion: r.aidocVersion}
	for _, review := range r.reviews {
		out.Reviews = append(out.Reviews, AvatarReview{
			ReviewID:     review.id,
			AvatarID:     review.avatar.ID,
			DisplayName:  review.avatar.DisplayName,
			State:        review.state,
			CommentCount: review.commentCount,
			Error:        review.errText,
		})
	}
	return out, nil
}

func (m *Mock) SubmitFeedback(ctx context.Context, commentRef string, fb Feedback) (FeedbackState, error) {
	if fb.Signal != SignalUp && fb.Signal != SignalDown {
		return FeedbackState{}, ErrInvalidRequest
	}
	if commentRef == "" || fb.GivenBy == "" {
		return FeedbackState{}, ErrInvalidRequest
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	voters := m.votes[commentRef]
	if voters == nil {
		voters = map[string]string{}
		m.votes[commentRef] = voters
	}
	// Upsert on (comment, voter): a repeated signal toggles the vote off, a
	// different one replaces it.
	if voters[fb.GivenBy] == fb.Signal {
		delete(voters, fb.GivenBy)
	} else {
		voters[fb.GivenBy] = fb.Signal
	}
	state := FeedbackState{Signal: voters[fb.GivenBy]}
	for _, signal := range voters {
		switch signal {
		case SignalUp:
			state.UpCount++
		case SignalDown:
			state.DownCount++
		}
	}
	return state, nil
}

// extractQuotes pulls up to n verbatim text snippets from the version HTML so
// fabricated comments anchor exactly like real ones. Only text that survives
// entity decoding unchanged is used — the anchor must match both the raw HTML
// (server-side placement check) and the rendered text (bridge anchoring).
func extractQuotes(raw []byte, n int) []string {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return nil
	}
	haystack := string(raw)
	var quotes []string
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if len(quotes) >= n {
			return
		}
		if node.Type == html.TextNode {
			candidate := strings.TrimSpace(node.Data)
			if parent := node.Parent; parent != nil && (parent.Data == "script" || parent.Data == "style") {
				candidate = ""
			}
			if len(candidate) >= 20 && len(candidate) <= 160 && !seen[candidate] && strings.Contains(haystack, candidate) {
				seen[candidate] = true
				quotes = append(quotes, candidate)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return quotes
}
