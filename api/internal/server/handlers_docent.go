package server

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/repo"
)

// Docent turns an existing document into a narrated, progressively-drawn
// board: one LLM call scripts the source content into a short sequence of
// steps, and the result is rendered through docentPlayerTemplate and saved
// as a new, separate, private document (never a new version of the source,
// and never automatically shared beyond the person who generated it).
//
// This is an internal-only add-on (see docentAgentBaseURL in server.go) —
// deliberately not part of the core feature set, so it has its own env
// namespace (DOCENT_*) and ships fully disabled by default.
//
// Why the generated board only has cards/arrows/frames/notes, not custom
// illustrations: the LLM's output here is untrusted input that ends up
// embedded in a new document. A fixed schema rendered by a fixed, reviewed
// template is safe; having a model emit arbitrary code to render is not.
//
// Why generation is a background job, not a synchronous request: a real
// gateway model can take well over a minute (observed: ~106s for a tiny
// source, 120s+ for a few KB) — long enough that a bare request/response
// cycle risks timing out at the client, at this server's own HTTP timeout,
// or at any reverse proxy sitting in front of it. The client starts a job,
// gets an id back immediately, and polls for the result.
const (
	docentMaxPlaintextBytes  = 50_000
	docentMinPlaintextBytes  = 500
	docentMaxSteps           = 16
	docentMaxElementsPerStep = 12
	docentMinSteps           = 3
	docentCanvasW            = 2000
	docentCanvasH            = 1000
	docentJobTimeout         = 5 * time.Minute
	docentJobRetention       = 15 * time.Minute
)

//go:embed docentassets/player-template.html
var docentPlayerTemplate string

var docentElementTypes = map[string]bool{
	"title-block": true,
	"card":        true,
	"note":        true,
	"frame":       true,
	"arrow":       true,
}

var docentTones = map[string]bool{
	"start": true, "process": true, "gate": true, "end": true, "info": true,
}

// docentIDPattern bounds element ids to a safe charset: they are used as
// object keys and as arrow from/to references inside the generated board's
// inline script.
var docentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type docentGenerateRequest struct {
	VersionID string `json:"version_id,omitempty"`
}

// docentStartResponse is returned immediately on POST .../docent/generate:
// either an existing board for this exact source version ("exists"), or a
// job to poll ("pending").
type docentStartResponse struct {
	Status           string `json:"status"`
	JobID            string `json:"job_id,omitempty"`
	ID               string `json:"id,omitempty"`
	CurrentVersionID string `json:"current_version_id,omitempty"`
	Truncated        bool   `json:"truncated,omitempty"`
}

// docentJobResponse is returned on GET .../docent/jobs/:job_id.
type docentJobResponse struct {
	Status           string     `json:"status"`
	ID               string     `json:"id,omitempty"`
	CurrentVersionID string     `json:"current_version_id,omitempty"`
	Error            *errorBody `json:"error,omitempty"`
}

// docentStory is the schema the LLM is asked to produce. It intentionally
// mirrors the narrated-board Claude Code skill's story schema, so the two
// authoring paths (a Claude Code session, or this one LLM call) render
// through the exact same player.
type docentStory struct {
	Title string       `json:"title"`
	Date  string       `json:"date"`
	Steps []docentStep `json:"steps"`
}

type docentStep struct {
	Narration string          `json:"narration"`
	Elements  []docentElement `json:"elements"`
}

type docentElement struct {
	ID       string  `json:"id,omitempty"`
	Type     string  `json:"type"`
	Tone     string  `json:"tone,omitempty"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	W        float64 `json:"w,omitempty"`
	H        float64 `json:"h,omitempty"`
	Text     string  `json:"text,omitempty"`
	Subtitle string  `json:"subtitle,omitempty"`
	From     string  `json:"from,omitempty"`
	To       string  `json:"to,omitempty"`
	Label    string  `json:"label,omitempty"`
}

// docentJobStatus is the lifecycle of one generation job.
type docentJobStatus string

const (
	docentJobPending docentJobStatus = "pending"
	docentJobDone    docentJobStatus = "done"
	docentJobError   docentJobStatus = "error"
)

type docentJob struct {
	status     docentJobStatus
	boardID    string
	versionID  string
	errCode    string
	errMessage string
	createdAt  time.Time
}

// docentJobStore is process-local, in-memory state: generation jobs, a
// source-version -> board index (so a repeat request reopens the existing
// board instead of generating another one), and a one-job-at-a-time budget
// per principal. It does not survive a restart and is not shared across
// replicas — an accepted scope limit for an internal add-on; see the PR
// description.
type docentJobStore struct {
	mu       sync.Mutex
	jobs     map[string]*docentJob
	bySource map[string]string // "<docID>@<versionID>" -> board document id
	inFlight map[string]bool   // "<principalType>:<principalID>" -> true while a job runs
}

func newDocentJobStore() *docentJobStore {
	return &docentJobStore{
		jobs:     map[string]*docentJob{},
		bySource: map[string]string{},
		inFlight: map[string]bool{},
	}
}

func principalKey(p auth.Principal) string {
	return string(p.Type) + ":" + p.ID
}

func sourceKey(docID, versionID string) string {
	return docID + "@" + versionID
}

func (s *docentJobStore) existingBoard(docID, versionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.bySource[sourceKey(docID, versionID)]
	return id, ok
}

// tryStart claims the one-job-per-principal slot. It reports false if the
// principal already has a job in flight.
func (s *docentJobStore) tryStart(p auth.Principal) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := principalKey(p)
	if s.inFlight[key] {
		return false
	}
	s.inFlight[key] = true
	return true
}

func (s *docentJobStore) finish(p auth.Principal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, principalKey(p))
}

// create registers a new pending job and returns its id.
func (s *docentJobStore) create() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	id := newDocentJobID()
	s.jobs[id] = &docentJob{status: docentJobPending, createdAt: time.Now()}
	return id
}

// gcLocked drops jobs older than docentJobRetention. Callers must hold mu.
func (s *docentJobStore) gcLocked() {
	cutoff := time.Now().Add(-docentJobRetention)
	for id, j := range s.jobs {
		if j.createdAt.Before(cutoff) {
			delete(s.jobs, id)
		}
	}
}

func (s *docentJobStore) get(id string) (docentJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return docentJob{}, false
	}
	return *j, true
}

func (s *docentJobStore) succeed(jobID, docID, versionID, boardID, boardVersionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[jobID]; ok {
		j.status = docentJobDone
		j.boardID = boardID
		j.versionID = boardVersionID
	}
	s.bySource[sourceKey(docID, versionID)] = boardID
}

func (s *docentJobStore) fail(jobID, code, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[jobID]; ok {
		j.status = docentJobError
		j.errCode = code
		j.errMessage = message
	}
}

func newDocentJobID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error; it panics instead.
	return "djob_" + hex.EncodeToString(b[:])
}

func (h handlers) generateDocent(c *gin.Context) {
	if h.deps.docentAgentBaseURL == "" {
		featureDisabled(c, "docent is not enabled on this deployment")
		return
	}

	docID := c.Param("id")
	if !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}

	var in docentGenerateRequest
	// An empty body is the common case (no explicit version pin); only
	// reject bodies that are present but malformed.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&in); err != nil {
			badRequest(c, "invalid request body")
			return
		}
	}

	doc, err := h.deps.repository.GetDocument(c.Request.Context(), docID)
	if err != nil {
		internalErr(c, err)
		return
	}

	versionID := in.VersionID
	if versionID == "" {
		versionID = doc.CurrentVersionID
	} else {
		v, err := h.deps.repository.GetVersion(c.Request.Context(), versionID)
		if err != nil || v.DocumentID != docID {
			notFound(c)
			return
		}
	}
	if versionID == "" {
		c.JSON(http.StatusUnprocessableEntity, errorResponse("no_version", "document has no version to narrate", nil))
		return
	}

	// A repeat request for the exact same source version reopens the board
	// already generated from it, rather than creating another one. Scoped to
	// this server process's uptime — see docentJobStore's doc comment.
	if boardID, ok := h.deps.docentJobs.existingBoard(docID, versionID); ok {
		if board, err := h.deps.repository.GetDocument(c.Request.Context(), boardID); err == nil {
			c.JSON(http.StatusOK, docentStartResponse{Status: "exists", ID: board.ID, CurrentVersionID: board.CurrentVersionID})
			return
		}
		// The indexed board no longer resolves (e.g. deleted since); fall
		// through and generate a fresh one.
	}

	raw, err := h.deps.repository.GetVersionHTML(c.Request.Context(), versionID)
	if err != nil {
		internalErr(c, err)
		return
	}

	fullPlaintext := docentHTMLToText(raw)
	if len(strings.TrimSpace(fullPlaintext)) < docentMinPlaintextBytes {
		c.JSON(http.StatusUnprocessableEntity, errorResponse(
			"source_too_short",
			"this document is too short to narrate reliably — a model asked to script a near-empty source tends to invent content instead of reporting that there isn't enough to work with",
			nil,
		))
		return
	}
	truncated := len(fullPlaintext) > docentMaxPlaintextBytes
	plaintext := truncateUTF8(fullPlaintext, docentMaxPlaintextBytes)

	principal := *current(c)
	if !h.deps.docentJobs.tryStart(principal) {
		c.JSON(http.StatusTooManyRequests, errorResponse(
			"docent_busy",
			"you already have a Docent board generating — wait for it to finish before starting another",
			nil,
		))
		return
	}

	jobID := h.deps.docentJobs.create()
	sourceTitle := doc.Title
	jobs := h.deps.docentJobs
	agentBaseURL, agentAPIKey, agentModel, httpClient := h.deps.docentAgentBaseURL, h.deps.docentAgentAPIKey, h.deps.docentAgentModel, h.deps.docentHTTPClient
	repository := h.deps.repository

	go runDocentJob(docentJobParams{
		jobID:        jobID,
		docID:        docID,
		versionID:    versionID,
		sourceTitle:  sourceTitle,
		plaintext:    plaintext,
		principal:    principal,
		jobs:         jobs,
		repository:   repository,
		agentBaseURL: agentBaseURL,
		agentAPIKey:  agentAPIKey,
		agentModel:   agentModel,
		httpClient:   httpClient,
	})

	c.JSON(http.StatusAccepted, docentStartResponse{Status: "pending", JobID: jobID, Truncated: truncated})
}

func (h handlers) getDocentJob(c *gin.Context) {
	if h.deps.docentAgentBaseURL == "" {
		featureDisabled(c, "docent is not enabled on this deployment")
		return
	}
	docID := c.Param("id")
	if !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}
	job, ok := h.deps.docentJobs.get(c.Param("job_id"))
	if !ok {
		notFound(c)
		return
	}
	switch job.status {
	case docentJobDone:
		c.JSON(http.StatusOK, docentJobResponse{Status: "done", ID: job.boardID, CurrentVersionID: job.versionID})
	case docentJobError:
		c.JSON(http.StatusOK, docentJobResponse{Status: "error", Error: &errorBody{Code: job.errCode, Message: job.errMessage}})
	default:
		c.JSON(http.StatusOK, docentJobResponse{Status: "pending"})
	}
}

var (
	errDocentBackend     = errors.New("docent backend returned an error")
	errDocentUnparseable = errors.New("docent could not parse the generated board; try again")
)

const docentSystemPromptTemplate = `You turn a document into a short narrated, progressively-drawn whiteboard board.

Break the document below into %d to %d narrative steps — one idea per step, in the order a reader would need them. For each step, decide which small set of visual elements (cards, arrows, a grouping frame, a short note) should appear.

Only use facts, numbers, names, and claims that actually appear in the document below. Do not invent specifics — dates, amounts, incident details, people, or outcomes — that are not present in the source. If the document does not say something, the board should not say it either.

Respond with ONLY a JSON object, no markdown fences, no commentary, matching exactly this shape:

{
  "title": "a short title for this board",
  "date": "a short one-line context string, e.g. a date or source note",
  "steps": [
    {
      "narration": "one or two spoken sentences for this step",
      "elements": [
        {"id": "unique-id", "type": "card", "tone": "start", "x": 80, "y": 260, "w": 260, "h": 90, "text": "label for this card"},
        {"type": "arrow", "from": "unique-id", "to": "other-id", "tone": "process"}
      ]
    }
  ]
}

Element "type" is one of: "title-block" (text + optional subtitle, used once as the first step), "card" (needs id, x, y, w, h, text), "note" (small caption, same geometry, no tone), "frame" (dashed grouping box spanning the cards it groups, x/y/w/h, optional label in the "label" field), "arrow" (needs from, to — no geometry, it anchors to the cards itself).

"tone" is one of: start, process, gate, end, info — use "start" for the trigger/actor, "process" for things doing work, "gate" for a decision point, "end" for the terminal outcome, "info" for neutral captions. Omit tone for note, frame, and arrow unless it's an arrow you want colored.

Canvas is %d x %d. Lay elements out left to right in the order the narration introduces them, leaving at least 150px between card columns and 200px between rows. Keep narration short and conversational — it is both spoken and displayed as text.

Document:
<document>
%s
</document>`

type docentJobParams struct {
	jobID        string
	docID        string
	versionID    string
	sourceTitle  string
	plaintext    string
	principal    auth.Principal
	jobs         *docentJobStore
	repository   repo.Repository
	agentBaseURL string
	agentAPIKey  string
	agentModel   string
	httpClient   *http.Client
}

// runDocentJob does the actual work in a background goroutine: it must not
// use the originating request's context, which is cancelled the moment the
// 202 response is written.
func runDocentJob(p docentJobParams) {
	defer p.jobs.finish(p.principal)
	ctx, cancel := context.WithTimeout(context.Background(), docentJobTimeout)
	defer cancel()

	story, err := callDocentLLM(ctx, p.httpClient, p.agentBaseURL, p.agentAPIKey, p.agentModel, p.sourceTitle, p.plaintext)
	if err != nil {
		code, message := docentPublicError(err)
		slog.Error("docent generation failed", slog.String("job_id", p.jobID), slog.String("doc_id", p.docID), slog.Any("error", err))
		p.jobs.fail(p.jobID, code, message)
		return
	}

	pageHTML, err := renderDocentBoard(story, p.sourceTitle, p.docID)
	if err != nil {
		slog.Error("docent render failed", slog.String("job_id", p.jobID), slog.Any("error", err))
		p.jobs.fail(p.jobID, "render_failed", "could not render the generated board")
		return
	}

	title := strings.TrimSpace(story.Title)
	if title == "" {
		title = p.sourceTitle + " — Docent board"
	}
	title = truncateRunes(title, 200)

	observeHTML("docent_generate", len(pageHTML))
	// Always private: the generating principal may only have viewer access
	// to the source, so the board must not inherit the server's default
	// "anyone" sharing grant — that would let any viewer re-publish a
	// restricted document's content org-wide. Share the board explicitly,
	// same as any other document, if that's wanted.
	created, _, err := p.repository.CreateDocument(ctx, p.principal, title, []byte(pageHTML))
	if err != nil {
		slog.Error("docent create document failed", slog.String("job_id", p.jobID), slog.Any("error", err))
		p.jobs.fail(p.jobID, "create_failed", "could not save the generated board")
		return
	}
	incDocument("created", "docent")
	incVersion("created_initial", "docent")

	p.jobs.succeed(p.jobID, p.docID, p.versionID, created.ID, created.CurrentVersionID)
}

// docentPublicError maps an internal error to a stable code and a message
// safe to show a reader. Gateway URLs, Go transport errors, and JSON decoder
// detail stay in the server log only.
func docentPublicError(err error) (code, message string) {
	switch {
	case errors.Is(err, errDocentUnparseable):
		return "docent_unparseable", "docent could not parse the generated board; try again"
	case errors.Is(err, errDocentBackend):
		return "docent_backend_error", "the configured Docent backend did not respond successfully; try again"
	default:
		return "docent_failed", "docent generation failed"
	}
}

func callDocentLLM(ctx context.Context, httpClient *http.Client, agentBaseURL, agentAPIKey, agentModel, sourceTitle, plaintext string) (*docentStory, error) {
	systemPrompt := fmt.Sprintf(docentSystemPromptTemplate, docentMinSteps, docentMaxSteps, docentCanvasW, docentCanvasH, plaintext)

	payload, err := json.Marshal(map[string]any{
		"model": agentModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Generate the board for: " + sourceTitle},
		},
		"stream": false,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentBaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if agentAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+agentAPIKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errDocentBackend, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", errDocentBackend, resp.Status)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("%w: %v", errDocentBackend, err)
	}
	if len(chatResp.Choices) == 0 || strings.TrimSpace(chatResp.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("%w: empty response", errDocentBackend)
	}

	return parseDocentStory(chatResp.Choices[0].Message.Content)
}

// parseDocentStory parses and defensively bounds the model's JSON response.
// The result is embedded into a new document, so out-of-range or malformed
// input is clamped or dropped rather than trusted verbatim.
func parseDocentStory(content string) (*docentStory, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var story docentStory
	if err := json.Unmarshal([]byte(content), &story); err != nil {
		return nil, fmt.Errorf("%w: %v", errDocentUnparseable, err)
	}
	if len(story.Steps) == 0 {
		return nil, fmt.Errorf("%w: no steps", errDocentUnparseable)
	}

	story.Title = truncateRunes(strings.TrimSpace(story.Title), 200)
	story.Date = truncateRunes(strings.TrimSpace(story.Date), 200)

	if len(story.Steps) > docentMaxSteps {
		story.Steps = story.Steps[:docentMaxSteps]
	}

	knownIDs := map[string]bool{}
	for si := range story.Steps {
		step := &story.Steps[si]
		step.Narration = truncateRunes(strings.TrimSpace(step.Narration), 1000)
		if len(step.Elements) > docentMaxElementsPerStep {
			step.Elements = step.Elements[:docentMaxElementsPerStep]
		}
		kept := step.Elements[:0]
		for _, el := range step.Elements {
			if !docentElementTypes[el.Type] {
				continue
			}
			el.Text = truncateRunes(strings.TrimSpace(el.Text), 300)
			el.Subtitle = truncateRunes(strings.TrimSpace(el.Subtitle), 200)
			el.Label = truncateRunes(strings.TrimSpace(el.Label), 120)
			if !docentTones[el.Tone] {
				el.Tone = ""
			}
			el.X = clampCoord(el.X)
			el.Y = clampCoord(el.Y)
			// Zero means "the model left this geometry unset" (title-block
			// and note don't require it) — clamp only a value that was
			// actually supplied. Force-flooring an omitted 0 to the minimum
			// size used to render every such element as a near-invisible
			// 10px box (e.g. a title squeezed to one word per line). A card
			// that explicitly asks for 0x0 is indistinguishable from one
			// that omitted it entirely (both decode to the float64 zero
			// value) and is left at 0 the same way — an acceptable gap: the
			// prompt tells the model cards require geometry, so a card
			// genuinely sized 0x0 is already a model mistake either way.
			if el.W != 0 {
				el.W = clampSize(el.W)
			}
			if el.H != 0 {
				el.H = clampSize(el.H)
			}
			if el.Type == "arrow" {
				if !docentIDPattern.MatchString(el.From) || !docentIDPattern.MatchString(el.To) {
					continue
				}
			} else {
				if el.ID == "" || !docentIDPattern.MatchString(el.ID) {
					el.ID = fmt.Sprintf("el-%d-%d", si, len(kept))
				}
				knownIDs[el.ID] = true
			}
			kept = append(kept, el)
		}
		step.Elements = kept
	}
	// Drop arrows referencing an id no card/note/frame in this story defines —
	// the player anchors arrows to live DOM positions, so a dangling
	// reference would otherwise fail silently at playback time.
	for si := range story.Steps {
		step := &story.Steps[si]
		kept := step.Elements[:0]
		for _, el := range step.Elements {
			if el.Type == "arrow" && (!knownIDs[el.From] || !knownIDs[el.To]) {
				continue
			}
			kept = append(kept, el)
		}
		step.Elements = kept
	}

	if len(story.Steps) < docentMinSteps {
		return nil, fmt.Errorf("%w: only %d usable step(s) after validation, need at least %d", errDocentUnparseable, len(story.Steps), docentMinSteps)
	}
	return &story, nil
}

func clampCoord(v float64) float64 { return clampFloat(v, -200, docentCanvasW+200) }
func clampSize(v float64) float64  { return clampFloat(v, 10, docentCanvasW) }

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// renderDocentBoard substitutes the story into the shared player template.
// STORY_JSON is embedded inside a <script> block, so every "<" is escaped to
// its < form: that keeps the JSON semantically identical (JSON.parse
// decodes < back to "<") while making a "</script" sequence impossible
// to form in the page's raw source — the standard mitigation for embedding
// untrusted JSON inside an inline script. The player itself (player-template.html)
// is responsible for rendering every story-supplied string via textContent
// or an equivalent safe sink, never innerHTML — this function only handles
// the one script-breakout risk that's specific to how the JSON is embedded.
func renderDocentBoard(story *docentStory, sourceTitle, sourceDocID string) (string, error) {
	storyJSON, err := json.Marshal(story)
	if err != nil {
		return "", err
	}
	safeStoryJSON := strings.ReplaceAll(string(storyJSON), "<", "\\u003c")

	title := story.Title
	if title == "" {
		title = sourceTitle
	}
	dateLine := fmt.Sprintf("AI-generated by Docent from %q · %s · verify against the source before relying on it", sourceTitle, time.Now().UTC().Format("2 Jan 2006"))

	out := docentPlayerTemplate
	out = strings.ReplaceAll(out, "{{TITLE}}", html.EscapeString(title))
	out = strings.ReplaceAll(out, "{{DATE}}", html.EscapeString(dateLine))
	out = strings.ReplaceAll(out, "{{STORY_JSON}}", safeStoryJSON)
	return out, nil
}

// docentHTMLToText extracts visible text nodes from HTML, skipping
// script/style/head — same approach as any other "read this document's
// content" path in this server.
func docentHTMLToText(raw []byte) string {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return string(raw)
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "head":
				return
			}
		}
		if n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" {
				sb.WriteString(t)
				sb.WriteByte('\n')
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return sb.String()
}

func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for i := maxBytes; i > 0; i-- {
		if (s[i] & 0xc0) != 0x80 {
			return s[:i]
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
