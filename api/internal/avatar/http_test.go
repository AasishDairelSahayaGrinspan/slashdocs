package avatar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeState struct {
	logins    int
	lastBody  map[string]any
	expireOne bool
}

// fakeAva imitates the ava service surface the HTTPClient consumes.
func fakeAva(t *testing.T) (*httptest.Server, *fakeState) {
	t.Helper()
	state := &fakeState{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		state.logins++
		if err := r.ParseForm(); err != nil || r.PostForm.Get("username") != "svc" || r.PostForm.Get("password") != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("GET /api/v1/bench", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"avatar_id": "av1", "display_name": "Arnav Gaur Avatar", "person_email": "arnav.gaur@razorpay.com", "tags": []string{"security", "infra"}},
		})
	})
	mux.HandleFunc("POST /api/v1/reviews", func(w http.ResponseWriter, r *http.Request) {
		if state.expireOne {
			state.expireOne = false
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&state.lastBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"request_id": "req1",
			"reviews": []map[string]any{
				{"id": "rev1", "request_id": "req1", "avatar_id": "av1", "aidoc_id": "doc_x", "aidoc_version": "ver_1", "status": "queued", "stats": map[string]any{}},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/reviews/requests/req1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "rev1", "request_id": "req1", "avatar_id": "av1", "aidoc_id": "doc_x", "aidoc_version": "ver_1", "status": "notified", "stats": map[string]any{"comments_posted": 2}},
		})
	})
	mux.HandleFunc("GET /api/v1/reviews/requests/silent", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "rev2", "request_id": "silent", "avatar_id": "av1", "aidoc_id": "doc_x", "aidoc_version": "ver_1", "status": "notified", "stats": map[string]any{"comments_posted": 0}},
		})
	})
	mux.HandleFunc("GET /api/v1/reviews/requests/gone", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"unknown request_id"}`))
	})
	mux.HandleFunc("PUT /api/v1/comments/aidoc/cmt_1/feedback", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state.lastBody = body
		_ = json.NewEncoder(w).Encode(map[string]any{"signal": body["verdict"], "up_count": 1, "down_count": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

func newTestClient(srv *httptest.Server) *HTTPClient {
	return NewHTTPClient(srv.URL, "svc", "pw", "https://aidocs.example.com")
}

func TestRequestReviewMapsTheTriggerAndAttribution(t *testing.T) {
	srv, state := fakeAva(t)
	c := newTestClient(srv)

	status, err := c.RequestReview(context.Background(), ReviewRequest{
		AidocID: "doc_x", AidocVersion: "ver_1", AvatarIDs: []string{"av1"}, TriggeredByEmail: "author@razorpay.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.lastBody["aidoc_url"] != "https://aidocs.example.com/app/d/doc_x" {
		t.Fatalf("aidoc_url = %v", state.lastBody["aidoc_url"])
	}
	if state.lastBody["triggered_by_email"] != "author@razorpay.com" {
		t.Fatalf("attribution lost: %v", state.lastBody["triggered_by_email"])
	}
	if status.RequestID != "req1" || len(status.Reviews) != 1 {
		t.Fatalf("status = %+v", status)
	}
	r := status.Reviews[0]
	if r.State != StateQueued || r.DisplayName != "Arnav Gaur Avatar" {
		t.Fatalf("review = %+v", r)
	}
}

func TestGetReviewMapsTerminalStates(t *testing.T) {
	srv, _ := fakeAva(t)
	c := newTestClient(srv)

	done, err := c.GetReview(context.Background(), "req1")
	if err != nil {
		t.Fatal(err)
	}
	if got := done.Reviews[0]; got.State != StateCommented || got.CommentCount != 2 {
		t.Fatalf("notified with comments should be commented: %+v", got)
	}

	silent, err := c.GetReview(context.Background(), "silent")
	if err != nil {
		t.Fatal(err)
	}
	if got := silent.Reviews[0]; got.State != StateNoComments {
		t.Fatalf("notified without comments should be no_comments: %+v", got)
	}

	if _, err := c.GetReview(context.Background(), "gone"); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("missing request should be ErrUnknownRequest, got %v", err)
	}
}

func TestExpiredTokenIsMintedOnceAndRetried(t *testing.T) {
	srv, state := fakeAva(t)
	c := newTestClient(srv)
	state.expireOne = true // first authed call sees a 401

	_, err := c.RequestReview(context.Background(), ReviewRequest{
		AidocID: "doc_x", AidocVersion: "ver_1", AvatarIDs: []string{"av1"}, TriggeredByEmail: "a@razorpay.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.logins != 2 {
		t.Fatalf("expected re-login after 401, logins = %d", state.logins)
	}
}

func TestSubmitFeedbackSpeaksAvaVerdicts(t *testing.T) {
	srv, state := fakeAva(t)
	c := newTestClient(srv)

	got, err := c.SubmitFeedback(context.Background(), "cmt_1", Feedback{Signal: SignalUp, Persona: PersonaIC, Surface: "aidoc", GivenBy: "v@razorpay.com"})
	if err != nil {
		t.Fatal(err)
	}
	if state.lastBody["verdict"] != "up" || state.lastBody["persona"] != "ic" || state.lastBody["given_by"] != "v@razorpay.com" {
		t.Fatalf("feedback body = %v", state.lastBody)
	}
	if got.Signal != "up" || got.UpCount != 1 {
		t.Fatalf("state = %+v", got)
	}
}

func TestListAvatarsMapsTheBench(t *testing.T) {
	srv, _ := fakeAva(t)
	c := newTestClient(srv)

	list, err := c.ListAvatars(context.Background(), "doc_x")
	if err != nil {
		t.Fatal(err)
	}
	if list.MaxSelectable != 3 || len(list.Items) != 1 {
		t.Fatalf("list = %+v", list)
	}
	a := list.Items[0]
	if a.ID != "av1" || a.PersonEmail != "arnav.gaur@razorpay.com" || len(a.FocusAreas) != 2 {
		t.Fatalf("avatar = %+v", a)
	}
}
