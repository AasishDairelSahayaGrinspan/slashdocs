package server

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/blob"
	"github.com/razorpay/slashdocs/api/internal/repo"
)

// Docent turns an existing document into a narrated, progressively-drawn
// board: one LLM call scripts the source content into a short sequence of
// steps, and the result is rendered through docentPlayerTemplate and saved
// as a new, separate document (never a new version of the source).
//
// This is an internal-only add-on (see docentAgentBaseURL in server.go) —
// deliberately not part of the core feature set, so it has its own env
// namespace (DOCENT_*) and ships fully disabled by default.
//
// Why the generated board only has cards/arrows/frames/notes, not custom
// illustrations: the LLM's output here is untrusted input that ends up
// embedded in a new document. A fixed schema rendered by a fixed, reviewed
// template is safe; having a model emit arbitrary code to render is not.
const (
	docentMaxPlaintextBytes  = 50_000
	docentMaxSteps           = 16
	docentMaxElementsPerStep = 12
	docentMinSteps           = 3
	docentCanvasW            = 2000
	docentCanvasH            = 1000
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

type docentGenerateResponse struct {
	ID               string        `json:"id"`
	CurrentVersionID string        `json:"current_version_id"`
	DefaultGrant     *defaultGrant `json:"default_grant,omitempty"`
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
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": gin.H{"message": "document has no version to narrate"},
		})
		return
	}

	raw, err := h.deps.repository.GetVersionHTML(c.Request.Context(), versionID)
	if err != nil {
		internalErr(c, err)
		return
	}

	plaintext := docentHTMLToText(raw)
	plaintext = truncateUTF8(plaintext, docentMaxPlaintextBytes)
	if strings.TrimSpace(plaintext) == "" {
		badRequest(c, "document has no readable text content to narrate")
		return
	}

	story, err := h.callDocentLLM(c.Request.Context(), doc.Title, plaintext)
	if err != nil {
		var status int
		switch {
		case errors.Is(err, errDocentBackend):
			status = http.StatusBadGateway
		case errors.Is(err, errDocentUnparseable):
			status = http.StatusBadGateway
		default:
			internalErr(c, err)
			return
		}
		c.JSON(status, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	pageHTML, err := renderDocentBoard(story, doc.Title, docID)
	if err != nil {
		internalErr(c, err)
		return
	}

	title := strings.TrimSpace(story.Title)
	if title == "" {
		title = doc.Title + " — Docent board"
	}
	title = truncateRunes(title, 200)

	observeHTML("docent_generate", len(pageHTML))
	created, _, err := h.deps.repository.CreateDocument(c.Request.Context(), *current(c), title, []byte(pageHTML))
	if err != nil {
		if errors.Is(err, blob.ErrStorage) {
			c.JSON(http.StatusBadGateway, errorResponse("blob_storage_failed", "could not upload the generated board to blob storage", nil))
			return
		}
		internalErr(c, err)
		return
	}
	incDocument("created", actorType(c))
	incVersion("created_initial", actorType(c))

	var granted *defaultGrant
	if h.deps.defaultDocPermission != repo.RoleNone {
		g, err := h.deps.repository.CreateGrant(c.Request.Context(), "document", created.ID, auth.Principal{Type: auth.PrincipalAnyone}, h.deps.defaultDocPermission, *current(c))
		if err != nil {
			internalErr(c, fmt.Errorf("default grant for docent board %s: %w", created.ID, err))
			return
		}
		incGrant("created", string(g.Role), string(g.Principal.Type), actorType(c))
		granted = &defaultGrant{GrantID: g.ID, Principal: defaultGrantPrincipal{Type: auth.PrincipalAnyone}, Role: g.Role}
	}

	c.JSON(http.StatusCreated, docentGenerateResponse{
		ID:               created.ID,
		CurrentVersionID: created.CurrentVersionID,
		DefaultGrant:     granted,
	})
}

var (
	errDocentBackend     = errors.New("docent backend returned an error")
	errDocentUnparseable = errors.New("docent could not parse the generated board; try again")
)

const docentSystemPromptTemplate = `You turn a document into a short narrated, progressively-drawn whiteboard board.

Break the document below into %d to %d narrative steps — one idea per step, in the order a reader would need them. For each step, decide which small set of visual elements (cards, arrows, a grouping frame, a short note) should appear.

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

Element "type" is one of: "title-block" (text + optional subtitle, used once as the first step), "card" (needs id, x, y, w, h, text), "note" (small caption, same geometry, no tone), "frame" (dashed grouping box spanning the cards it groups, x/y/w/h, optional label as "text"), "arrow" (needs from, to — no geometry, it anchors to the cards itself).

"tone" is one of: start, process, gate, end, info — use "start" for the trigger/actor, "process" for things doing work, "gate" for a decision point, "end" for the terminal outcome, "info" for neutral captions. Omit tone for note, frame, and arrow unless it's an arrow you want colored.

Canvas is %d x %d. Lay elements out left to right in the order the narration introduces them, leaving at least 150px between card columns and 200px between rows. Keep narration short and conversational — it is both spoken and displayed as text.

Document:
<document>
%s
</document>`

func (h handlers) callDocentLLM(ctx context.Context, sourceTitle, plaintext string) (*docentStory, error) {
	systemPrompt := fmt.Sprintf(docentSystemPromptTemplate, docentMinSteps, docentMaxSteps, docentCanvasW, docentCanvasH, plaintext)

	payload, err := json.Marshal(map[string]any{
		"model": h.deps.docentAgentModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Generate the board for: " + sourceTitle},
		},
		"stream": false,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.deps.docentAgentBaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.deps.docentAgentAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.deps.docentAgentAPIKey)
	}

	resp, err := h.deps.docentHTTPClient.Do(req)
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

	story, err := parseDocentStory(chatResp.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	return story, nil
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
			el.W = clampSize(el.W)
			el.H = clampSize(el.H)
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

	if len(story.Steps) < docentMinSteps && len(story.Steps) == 0 {
		return nil, fmt.Errorf("%w: no usable steps after validation", errDocentUnparseable)
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
// untrusted JSON inside an inline script.
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
	dateLine := fmt.Sprintf("Generated by Docent from %q · %s", sourceTitle, time.Now().UTC().Format("2 Jan 2006"))

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
