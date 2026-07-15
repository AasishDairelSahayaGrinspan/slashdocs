package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/knowledge"
)

const (
	maxSourceWebhookBody = 1 << 20
	webhookReplayWindow  = 5 * time.Minute
)

type sourceDeliveryResponse struct {
	Accepted  bool `json:"accepted"`
	Duplicate bool `json:"duplicate"`
	Ignored   bool `json:"ignored"`
	Queued    int  `json:"queued"`
}

type ignoredSourceDeliveryResponse struct {
	Accepted bool `json:"accepted"`
	Ignored  bool `json:"ignored"`
	Queued   int  `json:"queued"`
}

type webhookChallengeResponse struct {
	Challenge string `json:"challenge"`
}

func readSourceWebhookBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxSourceWebhookBody+1))
	if err != nil {
		badRequest(c, "could not read webhook")
		return nil, false
	}
	if len(body) > maxSourceWebhookBody {
		c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "webhook payload exceeds 1 MiB", nil))
		return nil, false
	}
	return body, true
}

func hmacSHA256Hex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func timestampIsFresh(timestamp, now time.Time) bool {
	delta := now.Sub(timestamp)
	if delta < 0 {
		delta = -delta
	}
	return delta <= webhookReplayWindow
}

func safeSourcePathToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, value)
}

func queueSourceDelivery(c *gin.Context, h handlers, provider, deliveryID string, runs []knowledge.SyncRun, ignored bool) {
	accepted, err := h.deps.knowledge.QueueDelivery(c.Request.Context(), provider, deliveryID, runs)
	if err != nil {
		internalErr(c, err)
		return
	}
	if !accepted {
		c.JSON(http.StatusOK, sourceDeliveryResponse{Accepted: false, Duplicate: true, Queued: 0})
		return
	}
	c.JSON(http.StatusAccepted, sourceDeliveryResponse{Accepted: true, Duplicate: false, Ignored: ignored, Queued: len(runs)})
}

type slackMessage struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	Channel  string `json:"channel"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	Text     string `json:"text"`
	User     string `json:"user"`
	BotID    string `json:"bot_id"`
}

type slackEventEnvelope struct {
	Type          string `json:"type"`
	Challenge     string `json:"challenge"`
	EventID       string `json:"event_id"`
	TeamID        string `json:"team_id"`
	ContextTeamID string `json:"context_team_id"`
	Event         struct {
		slackMessage
		DeletedTS       string       `json:"deleted_ts"`
		Message         slackMessage `json:"message"`
		PreviousMessage slackMessage `json:"previous_message"`
	} `json:"event"`
}

func normalizeSlackMessage(envelope slackEventEnvelope) (slackMessage, bool) {
	event := envelope.Event.slackMessage
	if event.Type != "message" {
		return slackMessage{}, false
	}
	switch event.Subtype {
	case "message_changed":
		message := envelope.Event.Message
		message.Type = "message"
		if message.Channel == "" {
			message.Channel = event.Channel
		}
		return message, message.BotID == ""
	case "message_deleted":
		message := envelope.Event.PreviousMessage
		message.Type = "message"
		if message.Channel == "" {
			message.Channel = event.Channel
		}
		if message.TS == "" {
			message.TS = envelope.Event.DeletedTS
		}
		if strings.TrimSpace(message.Text) == "" {
			message.Text = "Message deleted from this Slack thread."
		} else {
			message.Text = "Deleted message: " + message.Text
		}
		return message, true
	case "bot_message":
		return slackMessage{}, false
	default:
		return event, event.BotID == ""
	}
}

func slackSourceMatches(source knowledge.Source, teamID, channelID, threadTS string) bool {
	if source.Provider != knowledge.ProviderSlack || source.Status != "active" || source.TriggerMode != "event" || source.Direction == "export_only" {
		return false
	}
	parts := strings.Split(source.ExternalRepository, "/")
	if (len(parts) != 2 && len(parts) != 3) || !strings.EqualFold(parts[0], teamID) || !strings.EqualFold(parts[1], channelID) {
		return false
	}
	return len(parts) == 2 || parts[2] == threadTS
}

// slackWebhook godoc
// @Summary Receive Slack thread events
// @Description Verify Slack's v0 HMAC and replay timestamp, answer URL verification, deduplicate event IDs, and queue matching message or thread updates.
// @Tags connected-source-webhooks
// @Accept json
// @Produce json
// @Param X-Slack-Signature header string true "Slack v0 HMAC signature"
// @Param X-Slack-Request-Timestamp header string true "Unix request timestamp"
// @Param body body object true "Slack Events API envelope"
// @Success 200 {string} string "URL verification challenge or duplicate delivery"
// @Success 202 {object} sourceDeliveryResponse "Accepted event"
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /v1/webhooks/slack [post]
//
// slackWebhook receives Slack Events API callbacks. Slack signs the exact raw
// body using v0:{timestamp}:{body}; the five-minute timestamp check prevents a
// captured valid request from being replayed indefinitely.
//
//nolint:gocyclo // Slack verification, replay protection, parsing, and source dispatch are one ordered pipeline.
func (h handlers) slackWebhook(c *gin.Context) {
	secret := h.deps.slackSigningSecret
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, errorResponse("webhook_not_configured", "Slack signing secret is not configured", nil))
		return
	}
	body, ok := readSourceWebhookBody(c)
	if !ok {
		return
	}
	timestampHeader := strings.TrimSpace(c.GetHeader("X-Slack-Request-Timestamp"))
	unixTimestamp, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil || !timestampIsFresh(time.Unix(unixTimestamp, 0), time.Now()) {
		c.JSON(http.StatusUnauthorized, errorResponse("stale_request", "Slack request timestamp is invalid or outside the five-minute replay window", nil))
		return
	}
	expected := "v0=" + hmacSHA256Hex(secret, []byte("v0:"+timestampHeader+":"+string(body)))
	if !hmac.Equal([]byte(strings.TrimSpace(c.GetHeader("X-Slack-Signature"))), []byte(expected)) {
		c.JSON(http.StatusUnauthorized, errorResponse("invalid_signature", "invalid Slack webhook signature", nil))
		return
	}
	var payload slackEventEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		badRequest(c, "invalid Slack payload")
		return
	}
	if payload.Type == "url_verification" {
		if strings.TrimSpace(payload.Challenge) == "" {
			badRequest(c, "Slack URL verification challenge is required")
			return
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(payload.Challenge))
		return
	}
	if payload.Type != "event_callback" {
		c.JSON(http.StatusAccepted, ignoredSourceDeliveryResponse{Accepted: true, Ignored: true, Queued: 0})
		return
	}
	if strings.TrimSpace(payload.EventID) == "" {
		badRequest(c, "Slack event_id is required")
		return
	}
	message, relevant := normalizeSlackMessage(payload)
	if !relevant {
		queueSourceDelivery(c, h, knowledge.ProviderSlack, payload.EventID, nil, true)
		return
	}
	teamID := strings.TrimSpace(payload.TeamID)
	if teamID == "" {
		teamID = strings.TrimSpace(payload.ContextTeamID)
	}
	message.Channel = strings.TrimSpace(message.Channel)
	message.TS = strings.TrimSpace(message.TS)
	if teamID == "" || message.Channel == "" || message.TS == "" {
		badRequest(c, "Slack message event requires team_id, channel, and ts")
		return
	}
	threadTS := strings.TrimSpace(message.ThreadTS)
	if threadTS == "" {
		threadTS = message.TS
	}
	sources, err := h.deps.knowledge.ListAllSources(c.Request.Context())
	if err != nil {
		internalErr(c, err)
		return
	}
	path := fmt.Sprintf("slack/%s/%s/threads/%s.txt", safeSourcePathToken(teamID), safeSourcePathToken(message.Channel), safeSourcePathToken(threadTS))
	runs := []knowledge.SyncRun{}
	for _, source := range sources {
		if !slackSourceMatches(source, teamID, message.Channel, threadTS) {
			continue
		}
		runs = append(runs, knowledge.SyncRun{
			ID: knowledgeID("sync"), RepositoryID: source.RepositoryID, SourceID: source.ID,
			DeliveryID: payload.EventID, AfterSHA: payload.EventID, ChangedFiles: []string{path},
			Status: "queued", Outcome: "pending", Message: "Slack thread event accepted for asynchronous documentation review.",
			ContextItems: []knowledge.ContextItem{{
				Kind: "slack_thread_message", ExternalID: teamID + "/" + message.Channel + "/" + threadTS,
				Title: "Slack thread update", Body: message.Text,
				Metadata: map[string]string{"author": message.User, "channel": message.Channel, "thread": threadTS},
			}},
			Provenance: []knowledge.Citation{{SourceID: source.ID, Provider: knowledge.ProviderSlack}}, StartedAt: knowledge.Now(),
		})
	}
	queueSourceDelivery(c, h, knowledge.ProviderSlack, payload.EventID, runs, len(runs) == 0)
}

type devRevWork struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	DisplayID    string `json:"display_id"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	State        string `json:"state"`
	ModifiedDate string `json:"modified_date"`
}

type devRevWebhookEnvelope struct {
	ID        string `json:"id"`
	WebhookID string `json:"webhook_id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Verify    struct {
		Challenge string `json:"challenge"`
	} `json:"verify"`
	Events map[string]json.RawMessage `json:"-"`
}

func decodeDevRevEnvelope(body []byte) (devRevWebhookEnvelope, error) {
	var payload devRevWebhookEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, err
	}
	if err := json.Unmarshal(body, &payload.Events); err != nil {
		return payload, err
	}
	return payload, nil
}

func devRevEventWork(payload devRevWebhookEnvelope) (devRevWork, error) {
	raw := payload.Events[payload.Type]
	if len(raw) == 0 {
		return devRevWork{}, fmt.Errorf("%s payload is required", payload.Type)
	}
	var wrapped struct {
		Work json.RawMessage `json:"work"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return devRevWork{}, err
	}
	if len(wrapped.Work) > 0 && string(wrapped.Work) != "null" {
		raw = wrapped.Work
	}
	var work devRevWork
	if err := json.Unmarshal(raw, &work); err != nil {
		return devRevWork{}, err
	}
	if work.ID == "" && work.DisplayID == "" {
		return devRevWork{}, fmt.Errorf("DevRev work id or display_id is required")
	}
	return work, nil
}

func devRevSourceMatches(source knowledge.Source, webhookID string, work devRevWork) bool {
	if source.Provider != knowledge.ProviderDevRev || source.Status != "active" || source.TriggerMode != "event" || source.Direction == "export_only" {
		return false
	}
	selector := strings.TrimSpace(source.ExternalRepository)
	return strings.EqualFold(selector, webhookID) || (work.ID != "" && strings.EqualFold(selector, work.ID)) || (work.DisplayID != "" && strings.EqualFold(selector, work.DisplayID))
}

func devRevDeletedWorkType(id string) string {
	value := strings.ToLower(strings.TrimSpace(id))
	for _, candidate := range []string{"ticket", "issue", "task", "opportunity", "incident"} {
		if strings.Contains(value, ":"+candidate+"/") {
			return candidate
		}
	}
	switch {
	case strings.HasPrefix(value, "tkt-"):
		return "ticket"
	case strings.HasPrefix(value, "iss-"):
		return "issue"
	}
	// A custom display ID does not necessarily encode its work type. DevRev
	// ticket sources are expected to use a ticket-filtered webhook, so retain
	// unknown IDs while rejecting IDs that are provably another work type.
	return "ticket"
}

// devRevWebhook godoc
// @Summary Receive DevRev ticket events
// @Description Verify DevRev's raw-body HMAC and event replay timestamp, answer endpoint verification, deduplicate event IDs, and queue matching ticket updates.
// @Tags connected-source-webhooks
// @Accept json
// @Produce json
// @Param X-DevRev-Signature header string true "Bare hex HMAC-SHA256 signature"
// @Param body body object true "DevRev webhook envelope"
// @Success 200 {object} webhookChallengeResponse "Verification challenge or duplicate delivery"
// @Success 202 {object} sourceDeliveryResponse "Accepted event"
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /v1/webhooks/devrev [post]
//
// devRevWebhook receives DevRev work events. DevRev's X-DevRev-Signature is a
// bare hex HMAC-SHA256 digest of the exact raw body. The payload timestamp is
// also checked because DevRev explicitly warns that signatures alone permit
// replay attacks.
//
//nolint:gocyclo // DevRev verification, delivery deduplication, parsing, and source dispatch are one ordered pipeline.
func (h handlers) devRevWebhook(c *gin.Context) {
	secret := h.deps.devRevWebhookSecret
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, errorResponse("webhook_not_configured", "DevRev webhook secret is not configured", nil))
		return
	}
	body, ok := readSourceWebhookBody(c)
	if !ok {
		return
	}
	expected := hmacSHA256Hex(secret, body)
	if !hmac.Equal([]byte(strings.TrimSpace(c.GetHeader("X-DevRev-Signature"))), []byte(expected)) {
		c.JSON(http.StatusUnauthorized, errorResponse("invalid_signature", "invalid DevRev webhook signature", nil))
		return
	}
	payload, err := decodeDevRevEnvelope(body)
	if err != nil {
		badRequest(c, "invalid DevRev payload")
		return
	}
	eventTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.Timestamp))
	if err != nil || !timestampIsFresh(eventTime, time.Now()) {
		c.JSON(http.StatusUnauthorized, errorResponse("stale_request", "DevRev timestamp is invalid or outside the five-minute replay window", nil))
		return
	}
	if payload.Type == "verify" {
		challenge := strings.TrimSpace(payload.Verify.Challenge)
		if challenge == "" {
			challenge = strings.TrimSpace(payload.Challenge)
		}
		if challenge == "" {
			badRequest(c, "DevRev verification challenge is required")
			return
		}
		c.JSON(http.StatusOK, webhookChallengeResponse{Challenge: challenge})
		return
	}
	if strings.TrimSpace(payload.ID) == "" || strings.TrimSpace(payload.WebhookID) == "" {
		badRequest(c, "DevRev id and webhook_id are required")
		return
	}
	if payload.Type != "work_created" && payload.Type != "work_updated" && payload.Type != "work_deleted" {
		queueSourceDelivery(c, h, knowledge.ProviderDevRev, payload.ID, nil, true)
		return
	}
	work, err := devRevEventWork(payload)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	// DevRev's documented work_deleted envelope contains only {"id": ...};
	// unlike create/update it cannot carry a work.type. A DevRev source is a
	// ticket connector, so preserve the deletion as ticket context instead of
	// silently discarding every valid deletion event.
	if payload.Type == "work_deleted" && work.Type == "" {
		work.Type = devRevDeletedWorkType(work.ID)
	}
	if !strings.EqualFold(work.Type, "ticket") {
		queueSourceDelivery(c, h, knowledge.ProviderDevRev, payload.ID, nil, true)
		return
	}
	sources, err := h.deps.knowledge.ListAllSources(c.Request.Context())
	if err != nil {
		internalErr(c, err)
		return
	}
	externalID := strings.TrimSpace(work.DisplayID)
	if externalID == "" {
		externalID = strings.TrimSpace(work.ID)
	}
	state := strings.TrimSpace(work.State)
	if payload.Type == "work_deleted" {
		state = "deleted"
	}
	path := "devrev/tickets/" + safeSourcePathToken(externalID) + ".txt"
	runs := []knowledge.SyncRun{}
	for _, source := range sources {
		if !devRevSourceMatches(source, payload.WebhookID, work) {
			continue
		}
		runs = append(runs, knowledge.SyncRun{
			ID: knowledgeID("sync"), RepositoryID: source.RepositoryID, SourceID: source.ID,
			DeliveryID: payload.ID, AfterSHA: payload.ID, ChangedFiles: []string{path},
			Status: "queued", Outcome: "pending", Message: "DevRev ticket event accepted for asynchronous documentation review.",
			ContextItems: []knowledge.ContextItem{{
				Kind: "devrev_ticket", ExternalID: externalID, Title: work.Title, Body: work.Body,
				Metadata: map[string]string{"event": payload.Type, "state": state, "type": work.Type},
			}},
			Provenance: []knowledge.Citation{{SourceID: source.ID, Provider: knowledge.ProviderDevRev}}, StartedAt: knowledge.Now(),
		})
	}
	queueSourceDelivery(c, h, knowledge.ProviderDevRev, payload.ID, runs, len(runs) == 0)
}
