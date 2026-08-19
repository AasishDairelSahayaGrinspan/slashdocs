// Package avatar is aidocs' single seam to the avatar service: the picker
// data, review requests, review status, and comment feedback all go through
// the Client interface. The rest of aidocs never knows whether the client is
// the real service or the built-in mock.
//
// TODO(avatar-service): mocked — replace with a real avatar-service HTTP
// client once the service is live. The API shapes below mirror the agreed
// avatar-service surface: GET /v1/avatars, POST /v1/avatar-reviews,
// GET /v1/avatar-reviews?request_id=..., POST /v1/comments/{ref}/feedback.
package avatar

import (
	"context"
	"errors"
)

// Review states. Queued and Running are transient; the other three are
// terminal. NoComments ("reviewed — no comments") and Failed ("couldn't
// finish") are deliberately distinct outcomes: a silent avatar and a broken
// avatar must never look the same.
const (
	StateQueued     = "queued"
	StateRunning    = "running"
	StateCommented  = "commented"
	StateNoComments = "no_comments"
	StateFailed     = "failed"
)

// Feedback signals and personas, mirroring the avatar-service feedback API.
const (
	SignalUp   = "up"
	SignalDown = "down"

	PersonaAuthor = "author"
	PersonaIC     = "ic"
)

// BenchAvatarID identifies "The Bench", the reviews-everything default
// avatar teased in the picker. It is not selectable in v1.
const BenchAvatarID = "av_bench"

var (
	ErrUnknownAvatar  = errors.New("unknown avatar")
	ErrUnknownRequest = errors.New("unknown review request")
	ErrInvalidRequest = errors.New("invalid avatar request")
)

// Avatar is the card data the picker renders: the minimum the UI needs, all
// fields straight from the avatar service.
type Avatar struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	PersonEmail   string   `json:"person_email"`
	FocusAreas    []string `json:"focus_areas"`
	RelevanceHint string   `json:"relevance_hint"`
	ComingSoon    bool     `json:"coming_soon,omitempty"`
}

// AvatarList is the GET /v1/avatars response. MaxSelectable caps picker
// selection (1 today; multi-avatar panels are upcoming).
type AvatarList struct {
	Items         []Avatar `json:"items"`
	MaxSelectable int      `json:"max_selectable"`
}

// ReviewRequest is the POST /v1/avatar-reviews payload.
type ReviewRequest struct {
	AidocID          string   `json:"aidoc_id"`
	AidocVersion     string   `json:"aidoc_version"`
	AvatarIDs        []string `json:"avatar_ids"`
	TriggeredByEmail string   `json:"triggered_by_email"`
}

// AvatarReview is one avatar's progress within a request. Avatars complete
// independently; the UI must never gate on the slowest.
type AvatarReview struct {
	ReviewID     string `json:"review_id"`
	AvatarID     string `json:"avatar_id"`
	DisplayName  string `json:"display_name"`
	State        string `json:"state"`
	CommentCount int    `json:"comment_count"`
	Error        string `json:"error,omitempty"`
}

// ReviewRequestStatus is the state of a whole request: the create response
// and the GET /v1/avatar-reviews?request_id=... polling shape.
type ReviewRequestStatus struct {
	RequestID    string         `json:"request_id"`
	AidocID      string         `json:"aidoc_id"`
	AidocVersion string         `json:"aidoc_version"`
	Reviews      []AvatarReview `json:"reviews"`
}

// Feedback is one viewer's vote on an avatar comment.
type Feedback struct {
	Signal  string `json:"signal"`
	Persona string `json:"persona"`
	Surface string `json:"surface"`
	GivenBy string `json:"given_by"`
}

// FeedbackState is the vote state the service returns after an upsert:
// the caller's live signal ("" when cleared) and the current tallies.
type FeedbackState struct {
	Signal    string `json:"signal"`
	UpCount   int    `json:"up_count"`
	DownCount int    `json:"down_count"`
}

// Client is the avatar-service surface aidocs consumes. Aidocs stores none
// of this state; every call is answered by the service (or its mock).
type Client interface {
	ListAvatars(ctx context.Context, aidocID string) (AvatarList, error)
	RequestReview(ctx context.Context, req ReviewRequest) (ReviewRequestStatus, error)
	GetReview(ctx context.Context, requestID string) (ReviewRequestStatus, error)
	SubmitFeedback(ctx context.Context, commentRef string, fb Feedback) (FeedbackState, error)
}

// DocumentStore is the minimal aidocs surface the mock needs to fabricate
// realistically anchored comments. The real avatar service posts its
// comments through aidocs MCP instead; this interface exists only so mock
// reviews exercise the genuine comment pipeline.
type DocumentStore interface {
	VersionHTML(ctx context.Context, documentID, versionID string) ([]byte, error)
	CreateAvatarComment(ctx context.Context, documentID, versionID, quote, body, authorDisplay, severity string) (string, error)
}
