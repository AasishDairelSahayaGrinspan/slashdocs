package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/repo"
)

// docentSystemPrincipal attributes Docent's own automated actions — an
// auto-generated board version, a reply to a comment — to Docent itself
// rather than to whichever person happened to trigger them. Comment and
// version authorship is an audit-trail field the repository stores as given,
// not a foreign key into a registered-users table, so this needs no setup
// step (no account to create) to use.
func docentSystemPrincipal() auth.Principal {
	return auth.Principal{
		Type:  auth.PrincipalServiceAccount,
		ID:    "docent",
		Name:  "Docent",
		Email: "docent@slashdocs.internal",
	}
}

// maybeRespondToDocentComment is called after a top-level comment is created
// successfully. It is a no-op for every document that isn't a Docent board —
// gated on the <meta name="docent-source-doc-id"> marker renderDocentBoard
// embeds — so it has zero effect on ordinary documents and ordinary comments.
// Everything past that check runs in a detached goroutine that must never
// slow down or fail the comment-creation response: every error is logged and
// swallowed, and a panic is recovered rather than propagated.
//
// Only hooked from createComment, never createCommentReply: Docent's own
// response is posted as a reply (see runDocentCommentResponse below), so a
// reply can never re-enter this path and there is no feedback loop to guard
// against explicitly.
func (h handlers) maybeRespondToDocentComment(docID string, cm repo.Comment) {
	if h.deps.docentAgentBaseURL == "" {
		return
	}
	if cm.Author.Type == auth.PrincipalServiceAccount && cm.Author.ID == "docent" {
		return
	}

	params := docentCommentResponseParams{
		docID:        docID,
		comment:      cm,
		repository:   h.deps.repository,
		agentBaseURL: h.deps.docentAgentBaseURL,
		agentAPIKey:  h.deps.docentAgentAPIKey,
		agentModel:   h.deps.docentAgentModel,
		httpClient:   h.deps.docentHTTPClient,
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("docent comment auto-response panicked",
					slog.String("doc_id", docID), slog.String("comment_id", cm.ID), slog.Any("panic", r))
			}
		}()
		runDocentCommentResponse(params)
	}()
}

type docentCommentResponseParams struct {
	docID        string
	comment      repo.Comment
	repository   repo.Repository
	agentBaseURL string
	agentAPIKey  string
	agentModel   string
	httpClient   *http.Client
}

// runDocentCommentResponse decides what, if anything, to do about a comment
// on a Docent board, then always leaves a reply explaining the outcome — a
// fix and why it was made, or no change and why not. It must not use the
// originating request's context, which is cancelled once that request's
// response is written.
func runDocentCommentResponse(p docentCommentResponseParams) {
	ctx, cancel := context.WithTimeout(context.Background(), docentJobTimeout)
	defer cancel()

	doc, err := p.repository.GetDocument(ctx, p.docID)
	if err != nil {
		slog.Error("docent comment response: get document failed", slog.String("doc_id", p.docID), slog.Any("error", err))
		return
	}
	if doc.CurrentVersionID == "" {
		return
	}
	raw, err := p.repository.GetVersionHTML(ctx, doc.CurrentVersionID)
	if err != nil {
		slog.Error("docent comment response: get version html failed", slog.String("doc_id", p.docID), slog.Any("error", err))
		return
	}
	sourceDocID, ok := docentSourceDocID(raw)
	if !ok {
		return // not a Docent board — nothing to do
	}
	story, err := docentStoryFromHTML(raw)
	if err != nil {
		slog.Error("docent comment response: could not recover board story", slog.String("doc_id", p.docID), slog.Any("error", err))
		return
	}

	var sourceContext string
	if sourceDoc, err := p.repository.GetDocument(ctx, sourceDocID); err == nil && sourceDoc.CurrentVersionID != "" {
		if sourceRaw, err := p.repository.GetVersionHTML(ctx, sourceDoc.CurrentVersionID); err == nil {
			sourceContext = truncateUTF8(docentHTMLToText(sourceRaw), docentMaxPlaintextBytes)
		}
	}
	if sourceContext == "" {
		// The source document moved or is no longer readable; fall back to
		// the board's own narration so a decision is still possible, but the
		// model can only check requests against what the board already says.
		sourceContext = "(the original source document is no longer available; judge only against the board content above)"
	}

	reply, newStory, err := callDocentCommentLLM(ctx, p.httpClient, p.agentBaseURL, p.agentAPIKey, p.agentModel, story, sourceContext, p.comment.Body)
	if err != nil {
		slog.Error("docent comment response: llm call failed", slog.String("doc_id", p.docID), slog.Any("error", err))
		reply = "Docent couldn't process this comment automatically — the configured backend did not return a usable response. A person should take a look."
		newStory = nil
	}

	if newStory != nil {
		pageHTML, err := renderDocentBoard(newStory, doc.Title, sourceDocID)
		if err != nil {
			slog.Error("docent comment response: render failed", slog.String("doc_id", p.docID), slog.Any("error", err))
			reply = reply + " (Docent drafted a fix but could not render it, so the board is unchanged.)"
		} else if _, err := p.repository.CreateVersion(ctx, p.docID, doc.CurrentVersionID, "Updated by Docent in response to a comment", []byte(pageHTML), docentSystemPrincipal()); err != nil {
			slog.Error("docent comment response: create version failed", slog.String("doc_id", p.docID), slog.Any("error", err))
			reply = reply + " (Docent drafted a fix but could not save it, so the board is unchanged.)"
		} else {
			incVersion("created_comment_fix", "docent")
		}
	}

	if _, err := p.repository.CreateCommentReply(ctx, p.docID, p.comment.ID, reply, docentSystemPrincipal(), nil); err != nil {
		slog.Error("docent comment response: reply failed", slog.String("doc_id", p.docID), slog.String("comment_id", p.comment.ID), slog.Any("error", err))
	}
}

// docentStoryFromHTML recovers the story a Docent board was rendered from by
// decoding the `const STORY = {...};` literal renderDocentBoard embeds. Using
// a JSON decoder (rather than splitting on the trailing ";") means trailing
// script bytes after the value — the ";" itself, the rest of the <script>
// block — are simply left unread instead of needing to be located, which
// matters because a semicolon can legitimately appear inside a narration or
// text string.
func docentStoryFromHTML(raw []byte) (*docentStory, error) {
	marker := []byte("const STORY = ")
	idx := bytes.Index(raw, marker)
	if idx < 0 {
		return nil, fmt.Errorf("docent story marker not found")
	}
	var story docentStory
	dec := json.NewDecoder(bytes.NewReader(raw[idx+len(marker):]))
	if err := dec.Decode(&story); err != nil {
		return nil, fmt.Errorf("docent story decode: %w", err)
	}
	return &story, nil
}

const docentCommentSystemPromptTemplate = `A reader left a comment on a narrated whiteboard board that Docent generated from a source document. Decide whether to act on the comment or explain why not, then reply.

The board's current content, in the step/element JSON schema Docent boards are authored in:
<board>
%s
</board>

The original source document this board was generated from — use it to check facts, and never invent something that isn't in here:
<source>
%s
</source>

The reader's comment:
<comment>
%s
</comment>

Decide one of two actions:
- "fix": the comment points out something the board should change (wrong or misleading content, a missing step, a confusing layout, content that is stale relative to the source) and the fix is supported by the source document above. Produce a complete, corrected version of the ENTIRE board — not a diff — in the same step/element schema, folding in the requested change.
- "explain": the comment is a question, the requested change is not supported by the source document, the request is out of scope for a whiteboard board, or no change is actually needed. Say why, concretely — reference what the comment asked and, where relevant, what the source document does or doesn't say.

Respond with ONLY a JSON object, no markdown fences, no commentary, matching exactly this shape:

{
  "action": "fix" or "explain",
  "reply": "a short reply to post on the comment thread — plain language, 1-4 sentences, say what changed and why, or why nothing changed",
  "story": { ...only when action is "fix": the complete updated board, same shape as <board> above... }
}

When action is "fix", "story" must be a complete, valid board (title, date, at least 3 steps, each with narration and elements) — not just the changed part. Omit "story" entirely when action is "explain".`

// callDocentCommentLLM asks the configured backend to triage one comment
// against a board's current story and its source document, and returns the
// reply text plus, only when the model decided to fix something, the
// corrected story. The corrected story is run back through parseDocentStory
// (the same bounding/clamping used for a fresh board) before it's trusted,
// since this is still unreviewed model output.
func callDocentCommentLLM(ctx context.Context, httpClient *http.Client, agentBaseURL, agentAPIKey, agentModel string, story *docentStory, sourceContext, commentBody string) (reply string, newStory *docentStory, err error) {
	storyJSON, err := json.Marshal(story)
	if err != nil {
		return "", nil, err
	}
	systemPrompt := fmt.Sprintf(docentCommentSystemPromptTemplate, string(storyJSON), sourceContext, commentBody)

	payload, err := json.Marshal(map[string]any{
		"model": agentModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Decide and respond now."},
		},
		"stream": false,
	})
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentBaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if agentAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+agentAPIKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", errDocentBackend, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("%w: status %s", errDocentBackend, resp.Status)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", nil, fmt.Errorf("%w: %v", errDocentBackend, err)
	}
	if len(chatResp.Choices) == 0 || strings.TrimSpace(chatResp.Choices[0].Message.Content) == "" {
		return "", nil, fmt.Errorf("%w: empty response", errDocentBackend)
	}

	content := stripDocentCodeFence(chatResp.Choices[0].Message.Content)

	var decoded struct {
		Action string          `json:"action"`
		Reply  string          `json:"reply"`
		Story  json.RawMessage `json:"story"`
	}
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return "", nil, fmt.Errorf("%w: %v", errDocentUnparseable, err)
	}

	reply = truncateRunes(strings.TrimSpace(decoded.Reply), 2000)
	if reply == "" {
		reply = "Docent reviewed this comment but didn't have anything further to add."
	}
	if decoded.Action == "fix" && len(decoded.Story) > 0 {
		fixed, err := parseDocentStory(string(decoded.Story))
		if err != nil {
			return "", nil, err
		}
		newStory = fixed
	}
	return reply, newStory, nil
}

// stripDocentCodeFence removes an optional ```json / ``` wrapper a model
// sometimes adds despite being asked not to. Shared by parseDocentStory and
// callDocentCommentLLM, the two places that unmarshal raw model output.
func stripDocentCodeFence(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}
