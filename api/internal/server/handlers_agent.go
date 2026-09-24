package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"

	"github.com/razorpay/aidocs/api/internal/repo"
)

const agentMaxPlaintextBytes = 60_000

type agentMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type agentChatRequest struct {
	Messages  []agentMessage `json:"messages"`
	VersionID string         `json:"version_id,omitempty"`
}

func (h handlers) agentChat(c *gin.Context) {
	if h.deps.agentBaseURL == "" {
		notFound(c)
		return
	}

	docID := c.Param("id")
	if !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}

	var in agentChatRequest
	if err := c.ShouldBindJSON(&in); err != nil || len(in.Messages) == 0 {
		badRequest(c, "messages must not be empty")
		return
	}

	versionID := in.VersionID
	if versionID == "" {
		doc, err := h.deps.repository.GetDocument(c.Request.Context(), docID)
		if err != nil {
			internalErr(c, err)
			return
		}
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
			"error": gin.H{"message": "document has no version to analyze"},
		})
		return
	}

	raw, err := h.deps.repository.GetVersionHTML(c.Request.Context(), versionID)
	if err != nil {
		internalErr(c, err)
		return
	}

	plaintext := htmlToText(raw)
	if len(plaintext) > agentMaxPlaintextBytes {
		// Truncate to a valid UTF-8 boundary.
		for i := agentMaxPlaintextBytes; i > 0; i-- {
			if (plaintext[i] & 0xc0) != 0x80 {
				plaintext = plaintext[:i]
				break
			}
		}
	}

	systemPrompt := "You are a helpful assistant. The document the user is asking about follows.\n\n<document>\n" +
		plaintext + "\n</document>"

	messages := make([]agentMessage, 0, len(in.Messages)+1)
	messages = append(messages, agentMessage{Role: "system", Content: systemPrompt})
	messages = append(messages, in.Messages...)

	payload, err := json.Marshal(map[string]any{
		"model":    h.deps.agentModel,
		"messages": messages,
		"stream":   true,
	})
	if err != nil {
		internalErr(c, err)
		return
	}

	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		http.MethodPost,
		h.deps.agentBaseURL+"/chat/completions",
		bytes.NewReader(payload),
	)
	if err != nil {
		internalErr(c, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if h.deps.agentAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.deps.agentAPIKey)
	}

	resp, err := h.deps.agentHTTPClient.Do(req)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{"message": "agent backend returned " + resp.Status},
		})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")

	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		after, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		if after == "[DONE]" {
			fmt.Fprint(c.Writer, "data: [DONE]\n\n")
			c.Writer.(http.Flusher).Flush()
			return
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(after), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		encoded, err := json.Marshal(chunk.Choices[0].Delta.Content)
		if err != nil {
			continue
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", encoded)
		c.Writer.(http.Flusher).Flush()
	}
}

// htmlToText extracts visible text nodes from HTML, skipping script/style/head.
func htmlToText(raw []byte) string {
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
