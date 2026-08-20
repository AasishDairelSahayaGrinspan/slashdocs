package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPClient is the real avatar-service client (the ava service). It signs in
// with a service credential (a superuser account on ava), caches the short-
// lived JWT, and re-authenticates once on a 401 before failing a call.
//
// Endpoint mapping (ava's surface, not the drafted /v1/* one — ava shipped
// first and this client adapts):
//
//	ListAvatars     GET  /api/v1/bench                         (public)
//	RequestReview   POST /api/v1/reviews                       (service auth;
//	                     triggered_by_email is honored for superusers)
//	GetReview       GET  /api/v1/reviews/requests/{request_id} (public)
//	SubmitFeedback  PUT  /api/v1/comments/aidoc/{ref}/feedback (service auth)
type HTTPClient struct {
	// BaseURL of the avatar service, e.g. https://ava.dev.razorpay.in.
	BaseURL string
	// Username/Password of ava's service (superuser) account.
	Username string
	Password string
	// DocOrigin is this aidocs deployment's public origin; ava's trigger
	// takes a document URL and extracts the doc_… id from it.
	DocOrigin string

	HTTP *http.Client

	mu    sync.Mutex
	token string
}

func NewHTTPClient(baseURL, username, password, docOrigin string) *HTTPClient {
	return &HTTPClient{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Username:  username,
		Password:  password,
		DocOrigin: strings.TrimRight(docOrigin, "/"),
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

// ---- ava's wire shapes (only the fields this client reads) ----

type avaBenchAvatar struct {
	AvatarID    string   `json:"avatar_id"`
	DisplayName string   `json:"display_name"`
	PersonEmail string   `json:"person_email"`
	Tags        []string `json:"tags"`
}

type avaReview struct {
	ID        string         `json:"id"`
	RequestID string         `json:"request_id"`
	AvatarID  string         `json:"avatar_id"`
	AidocID   string         `json:"aidoc_id"`
	AidocVer  string         `json:"aidoc_version"`
	Status    string         `json:"status"`
	Error     *string        `json:"error"`
	Stats     map[string]any `json:"stats"`
}

type avaTriggerResponse struct {
	RequestID string      `json:"request_id"`
	Reviews   []avaReview `json:"reviews"`
}

type avaFeedbackState struct {
	Signal    string `json:"signal"`
	UpCount   int    `json:"up_count"`
	DownCount int    `json:"down_count"`
}

// maxSelectable mirrors ava's AVATAR_REVIEW_MAX_PANEL.
const maxSelectable = 3

func (c *HTTPClient) ListAvatars(ctx context.Context, _ string) (AvatarList, error) {
	var rows []avaBenchAvatar
	if err := c.call(ctx, http.MethodGet, "/api/v1/bench", nil, &rows, false); err != nil {
		return AvatarList{}, err
	}
	items := make([]Avatar, 0, len(rows))
	for _, r := range rows {
		items = append(items, Avatar{
			ID:            r.AvatarID,
			DisplayName:   r.DisplayName,
			PersonEmail:   r.PersonEmail,
			FocusAreas:    r.Tags,
			RelevanceHint: relevanceHint(r.Tags),
		})
	}
	return AvatarList{Items: items, MaxSelectable: maxSelectable}, nil
}

func relevanceHint(tags []string) string {
	if len(tags) == 0 {
		return "Claimed and live on the bench"
	}
	return "Picked for " + strings.Join(tags, ", ")
}

func (c *HTTPClient) RequestReview(ctx context.Context, req ReviewRequest) (ReviewRequestStatus, error) {
	if len(req.AvatarIDs) == 0 || req.AidocID == "" {
		return ReviewRequestStatus{}, ErrInvalidRequest
	}
	body := map[string]any{
		// ava's trigger takes the document URL and extracts the doc_… id.
		"aidoc_url":          fmt.Sprintf("%s/app/d/%s", c.DocOrigin, req.AidocID),
		"aidoc_version":      req.AidocVersion,
		"avatar_ids":         req.AvatarIDs,
		"triggered_by_email": req.TriggeredByEmail,
	}
	var out avaTriggerResponse
	if err := c.call(ctx, http.MethodPost, "/api/v1/reviews", body, &out, true); err != nil {
		return ReviewRequestStatus{}, err
	}
	return c.toStatus(ctx, out.RequestID, out.Reviews), nil
}

func (c *HTTPClient) GetReview(ctx context.Context, requestID string) (ReviewRequestStatus, error) {
	var rows []avaReview
	err := c.call(ctx, http.MethodGet, "/api/v1/reviews/requests/"+url.PathEscape(requestID), nil, &rows, false)
	if err != nil {
		return ReviewRequestStatus{}, err
	}
	if len(rows) == 0 {
		return ReviewRequestStatus{}, ErrUnknownRequest
	}
	return c.toStatus(ctx, requestID, rows), nil
}

func (c *HTTPClient) SubmitFeedback(ctx context.Context, commentRef string, fb Feedback) (FeedbackState, error) {
	body := map[string]any{
		"verdict":  fb.Signal,
		"persona":  fb.Persona,
		"surface":  fb.Surface,
		"given_by": fb.GivenBy,
	}
	var out avaFeedbackState
	path := "/api/v1/comments/aidoc/" + url.PathEscape(commentRef) + "/feedback"
	if err := c.callMethod(ctx, http.MethodPut, path, body, &out, true); err != nil {
		return FeedbackState{}, err
	}
	return FeedbackState{Signal: out.Signal, UpCount: out.UpCount, DownCount: out.DownCount}, nil
}

// toStatus maps ava reviews onto the aidocs shape, resolving display names
// from the bench (best-effort — an unknown name never fails a poll).
func (c *HTTPClient) toStatus(ctx context.Context, requestID string, rows []avaReview) ReviewRequestStatus {
	names := map[string]string{}
	var bench []avaBenchAvatar
	if err := c.call(ctx, http.MethodGet, "/api/v1/bench", nil, &bench, false); err == nil {
		for _, b := range bench {
			names[b.AvatarID] = b.DisplayName
		}
	}
	status := ReviewRequestStatus{RequestID: requestID}
	for _, r := range rows {
		if status.AidocID == "" {
			status.AidocID, status.AidocVersion = r.AidocID, r.AidocVer
		}
		display := names[r.AvatarID]
		if display == "" {
			display = "Avatar"
		}
		count := 0
		if v, ok := r.Stats["comments_posted"].(float64); ok {
			count = int(v)
		}
		review := AvatarReview{ReviewID: r.ID, AvatarID: r.AvatarID, DisplayName: display, State: mapState(r.Status, count), CommentCount: count}
		if r.Error != nil {
			review.Error = *r.Error
		}
		status.Reviews = append(status.Reviews, review)
	}
	return status
}

// mapState translates ava review statuses to the aidocs state set. ava's
// "commented" (posted, Slack pending) and "notified" both mean the review is
// done from the reader's perspective; zero posted comments is the distinct
// "reviewed — nothing to say" outcome.
func mapState(avaStatus string, commentCount int) string {
	switch avaStatus {
	case "queued":
		return StateQueued
	case "running":
		return StateRunning
	case "commented", "notified":
		if commentCount == 0 {
			return StateNoComments
		}
		return StateCommented
	case "failed":
		return StateFailed
	default:
		return StateRunning
	}
}

// ---- transport ----

// call handles GETs/POSTs; callMethod is the general form.
func (c *HTTPClient) call(ctx context.Context, method, path string, body, out any, authed bool) error {
	return c.callMethod(ctx, method, path, body, out, authed)
}

func (c *HTTPClient) callMethod(ctx context.Context, method, path string, body, out any, authed bool) error {
	do := func(token string) (*http.Response, error) {
		var reader io.Reader
		if body != nil {
			payload, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return c.HTTP.Do(req)
	}

	token := ""
	if authed {
		var err error
		if token, err = c.getToken(ctx, false); err != nil {
			return err
		}
	}
	resp, err := do(token)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized && authed {
		// The cached JWT expired mid-flight — mint a fresh one, retry once.
		_ = resp.Body.Close()
		if token, err = c.getToken(ctx, true); err != nil {
			return err
		}
		if resp, err = do(token); err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out == nil {
			return nil
		}
		return json.Unmarshal(payload, out)
	case resp.StatusCode == http.StatusNotFound:
		if strings.Contains(string(payload), "avatar") {
			return fmt.Errorf("%w: %s", ErrUnknownAvatar, detail(payload))
		}
		return ErrUnknownRequest
	case resp.StatusCode == http.StatusConflict, resp.StatusCode == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %s", ErrInvalidRequest, detail(payload))
	default:
		return fmt.Errorf("avatar service %s %s: status %d: %s", method, path, resp.StatusCode, detail(payload))
	}
}

func detail(payload []byte) string {
	var body struct {
		Detail any `json:"detail"`
	}
	if err := json.Unmarshal(payload, &body); err == nil && body.Detail != nil {
		return fmt.Sprintf("%v", body.Detail)
	}
	s := string(payload)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// getToken returns the cached service JWT, minting one when absent or forced.
func (c *HTTPClient) getToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && !force {
		return c.token, nil
	}
	form := url.Values{"username": {c.Username}, "password": {c.Password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/login", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("avatar service login failed: status %d: %s", resp.StatusCode, detail(payload))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(payload, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("avatar service login returned no access_token")
	}
	c.token = out.AccessToken
	return c.token, nil
}
