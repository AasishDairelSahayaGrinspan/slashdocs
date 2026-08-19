package avatar

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/repo"
)

// fakeStore records fabricated avatar comments and serves fixture HTML.
type fakeStore struct {
	mu       sync.Mutex
	html     string
	comments []fakeComment
}

type fakeComment struct {
	DocumentID    string
	VersionID     string
	Quote         string
	Body          string
	AuthorDisplay string
	Severity      string
}

func (f *fakeStore) VersionHTML(ctx context.Context, documentID, versionID string) ([]byte, error) {
	return []byte(f.html), nil
}

func (f *fakeStore) CreateAvatarComment(ctx context.Context, documentID, versionID, quote, body, authorDisplay, severity string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments = append(f.comments, fakeComment{documentID, versionID, quote, body, authorDisplay, severity})
	return "cmt_fake", nil
}

func (f *fakeStore) recorded() []fakeComment {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeComment(nil), f.comments...)
}

const fixtureHTML = `<html><body><h1>Payment retries design</h1>
<p>The retry queue drains at a fixed rate of one hundred jobs per second.</p>
<p>Failures push the job back with exponential backoff and a maximum of five attempts.</p>
<p>Dead-lettered jobs alert the on-call engineer through the paging pipeline.</p>
</body></html>`

func newTestMock(store DocumentStore) *Mock {
	return NewMock(MockConfig{QueueDelay: 5 * time.Millisecond, RunDelay: 10 * time.Millisecond, Store: store})
}

func waitTerminal(t *testing.T, m *Mock, requestID string) ReviewRequestStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := m.GetReview(context.Background(), requestID)
		if err != nil {
			t.Fatalf("get review: %v", err)
		}
		done := true
		for _, r := range status.Reviews {
			if r.State == StateQueued || r.State == StateRunning {
				done = false
			}
		}
		if done {
			return status
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("review never reached a terminal state")
	return ReviewRequestStatus{}
}

func TestMockListAvatarsShape(t *testing.T) {
	m := newTestMock(&fakeStore{html: fixtureHTML})
	list, err := m.ListAvatars(context.Background(), "doc_1")
	if err != nil {
		t.Fatal(err)
	}
	if list.MaxSelectable != 1 {
		t.Fatalf("max_selectable = %d, want 1", list.MaxSelectable)
	}
	if len(list.Items) < 4 {
		t.Fatalf("want at least 3 avatars + bench, got %d", len(list.Items))
	}
	last := list.Items[len(list.Items)-1]
	if !last.ComingSoon || last.DisplayName != "The Bench" {
		t.Fatalf("last item = %+v, want The Bench with coming_soon", last)
	}
	for _, a := range list.Items[:len(list.Items)-1] {
		if a.ComingSoon {
			t.Fatalf("selectable avatar %s flagged coming_soon", a.ID)
		}
		if a.DisplayName == "" || a.PersonEmail == "" || len(a.FocusAreas) == 0 {
			t.Fatalf("avatar card incomplete: %+v", a)
		}
	}
}

func TestMockReviewLifecyclePostsAnchoredComments(t *testing.T) {
	store := &fakeStore{html: fixtureHTML}
	m := newTestMock(store)

	req := ReviewRequest{AidocID: "doc_1", AidocVersion: "ver_1", AvatarIDs: []string{"av_priya"}, TriggeredByEmail: "someone@example.com"}
	created, err := m.RequestReview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if created.RequestID == "" || len(created.Reviews) != 1 {
		t.Fatalf("unexpected create response: %+v", created)
	}
	if created.Reviews[0].State != StateQueued {
		t.Fatalf("initial state = %s, want queued", created.Reviews[0].State)
	}

	status := waitTerminal(t, m, created.RequestID)
	review := status.Reviews[0]
	if review.State != StateCommented {
		t.Fatalf("state = %s, want commented", review.State)
	}
	comments := store.recorded()
	if len(comments) < 2 || len(comments) > 3 {
		t.Fatalf("posted %d comments, want 2-3", len(comments))
	}
	if review.CommentCount != len(comments) {
		t.Fatalf("comment_count = %d, want %d", review.CommentCount, len(comments))
	}
	for _, cm := range comments {
		if cm.AuthorDisplay != "Priya's avatar" {
			t.Fatalf("author_display = %q", cm.AuthorDisplay)
		}
		if !repo.ValidSeverity(cm.Severity) || cm.Severity == "" {
			t.Fatalf("severity = %q, want a non-empty valid severity", cm.Severity)
		}
		if cm.Quote == "" || !strings.Contains(fixtureHTML, cm.Quote) {
			t.Fatalf("quote %q is not verbatim document text", cm.Quote)
		}
		if cm.Body == "" {
			t.Fatal("empty comment body")
		}
	}
}

func TestMockReviewIsIdempotentPerAvatarDocVersion(t *testing.T) {
	store := &fakeStore{html: fixtureHTML}
	m := newTestMock(store)
	req := ReviewRequest{AidocID: "doc_1", AidocVersion: "ver_1", AvatarIDs: []string{"av_priya"}, TriggeredByEmail: "someone@example.com"}
	first, err := m.RequestReview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.RequestReview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestID != second.RequestID {
		t.Fatalf("repeat request minted new request id %q != %q", second.RequestID, first.RequestID)
	}
	waitTerminal(t, m, first.RequestID)
	if n := len(store.recorded()); n > 3 {
		t.Fatalf("idempotent request posted duplicate comments: %d", n)
	}
}

func TestMockSilentAndFailedPathsAreDistinct(t *testing.T) {
	store := &fakeStore{html: fixtureHTML}
	m := newTestMock(store)

	quiet, err := m.RequestReview(context.Background(), ReviewRequest{AidocID: "doc_1", AidocVersion: "ver_1", AvatarIDs: []string{"av_meera"}, TriggeredByEmail: "x@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitTerminal(t, m, quiet.RequestID).Reviews[0].State; got != StateNoComments {
		t.Fatalf("quiet avatar state = %s, want %s", got, StateNoComments)
	}

	flaky, err := m.RequestReview(context.Background(), ReviewRequest{AidocID: "doc_1", AidocVersion: "ver_1", AvatarIDs: []string{"av_dev"}, TriggeredByEmail: "x@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitTerminal(t, m, flaky.RequestID).Reviews[0].State; got != StateFailed {
		t.Fatalf("flaky avatar state = %s, want %s", got, StateFailed)
	}
	if len(store.recorded()) != 0 {
		t.Fatal("silent/failed reviews must not post comments")
	}
}

func TestMockRejectsUnknownAndComingSoonAvatars(t *testing.T) {
	m := newTestMock(&fakeStore{html: fixtureHTML})
	if _, err := m.RequestReview(context.Background(), ReviewRequest{AidocID: "d", AidocVersion: "v", AvatarIDs: []string{"nope"}, TriggeredByEmail: "x@example.com"}); err == nil {
		t.Fatal("unknown avatar accepted")
	}
	if _, err := m.RequestReview(context.Background(), ReviewRequest{AidocID: "d", AidocVersion: "v", AvatarIDs: []string{BenchAvatarID}, TriggeredByEmail: "x@example.com"}); err == nil {
		t.Fatal("bench avatar accepted")
	}
}

func TestMockFeedbackUpsertAndToggle(t *testing.T) {
	m := newTestMock(&fakeStore{html: fixtureHTML})
	ctx := context.Background()

	state, err := m.SubmitFeedback(ctx, "cmt_1", Feedback{Signal: "up", Persona: "author", Surface: "aidoc", GivenBy: "a@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Signal != "up" || state.UpCount != 1 || state.DownCount != 0 {
		t.Fatalf("state = %+v", state)
	}

	// Changing your mind replaces the vote.
	state, err = m.SubmitFeedback(ctx, "cmt_1", Feedback{Signal: "down", Persona: "author", Surface: "aidoc", GivenBy: "a@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Signal != "down" || state.UpCount != 0 || state.DownCount != 1 {
		t.Fatalf("state = %+v", state)
	}

	// Repeating the same signal toggles the vote off.
	state, err = m.SubmitFeedback(ctx, "cmt_1", Feedback{Signal: "down", Persona: "author", Surface: "aidoc", GivenBy: "a@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Signal != "" || state.DownCount != 0 {
		t.Fatalf("state = %+v", state)
	}

	// A second voter's count is independent.
	if _, err := m.SubmitFeedback(ctx, "cmt_1", Feedback{Signal: "up", Persona: "ic", Surface: "aidoc", GivenBy: "b@example.com"}); err != nil {
		t.Fatal(err)
	}
	state, err = m.SubmitFeedback(ctx, "cmt_1", Feedback{Signal: "up", Persona: "author", Surface: "aidoc", GivenBy: "a@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if state.UpCount != 2 {
		t.Fatalf("up_count = %d, want 2", state.UpCount)
	}

	if _, err := m.SubmitFeedback(ctx, "cmt_1", Feedback{Signal: "sideways", GivenBy: "a@example.com"}); err == nil {
		t.Fatal("invalid signal accepted")
	}
}
