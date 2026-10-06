package server

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"

	"github.com/razorpay/slashdocs/api/internal/notifications"
	"github.com/razorpay/slashdocs/api/internal/repo"
)

type headingResponse struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level int    `json:"level"`
	URL   string `json:"url"`
}
type documentLinkResponse struct {
	URL string `json:"url"`
}

var headingSlug = regexp.MustCompile(`[^a-z0-9]+`)

// extractHeadings mirrors bridge prepareHeadings: reserve every author ID first,
// then suffix generated IDs until unique. The source HTML is never changed.
func extractHeadings(raw []byte) ([]headingResponse, error) {
	d, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if id := htmlID(n); id != "" {
				used[id] = true
			}
			if isHeading(n.Data) {
				nodes = append(nodes, n)
			}
			if n.Data == "template" {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(d)
	var content func(*html.Node) string
	content = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		var b strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			b.WriteString(content(c))
		}
		return b.String()
	}
	result := make([]headingResponse, 0, len(nodes))
	for _, n := range nodes {
		text := strings.TrimSpace(content(n))
		id := htmlID(n)
		if id == "" {
			base := strings.Trim(headingSlug.ReplaceAllString(strings.ToLower(text), "-"), "-")
			if base == "" {
				base = "section"
			}
			id = base
			for suffix := 2; used[id]; suffix++ {
				id = base + "-" + strconv.Itoa(suffix)
			}
			used[id] = true
		}
		result = append(result, headingResponse{ID: id, Text: text, Level: int(n.Data[1] - '0')})
	}
	return result, nil
}

// documentHeadings godoc
// @Summary List document headings and shareable links
// @Description Lists headings in stored HTML. Explicit HTML IDs are preserved. JavaScript-only headings appear in the viewer when created.
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param version query string false "Version ID (pins links to this version)"
// @Success 200 {object} listResponse[headingResponse]
// @Router /v1/documents/{id}/headings [get]
func (h handlers) documentHeadings(c *gin.Context) {
	id := c.Param("id")
	if !h.needDocRole(c, id, repo.RoleViewer) {
		return
	}
	d, err := h.deps.repository.GetDocument(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	version := c.Query("version")
	if version == "" {
		version = d.CurrentVersionID
	}
	v, err := h.deps.repository.GetVersion(c.Request.Context(), version)
	if errors.Is(err, repo.ErrNotFound) || (err == nil && v.DocumentID != id) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	raw, err := h.deps.repository.GetVersionHTML(c.Request.Context(), version)
	if err != nil {
		internalErr(c, err)
		return
	}
	items, err := extractHeadings(raw)
	if err != nil {
		internalErr(c, err)
		return
	}
	for i := range items {
		items[i].URL = h.headingURL(id, items[i].ID, c.Query("version"))
	}
	c.JSON(http.StatusOK, listResponse[headingResponse]{Items: items})
}

func (h handlers) headingURL(docID, heading, version string) string {
	u := url.URL{Path: "/app/d/" + docID, Fragment: heading}
	if version != "" {
		u.RawQuery = url.Values{"version": {version}}.Encode()
	}
	return strings.TrimRight(h.deps.appOrigin, "/") + u.String()
}

// documentLink godoc
// @Summary Get a shareable link to a document, comment, or heading
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param comment query string false "Comment or reply ID"
// @Param heading query string false "Heading ID, including an ID created by document JavaScript"
// @Param version query string false "Version ID for a heading or document link"
// @Success 200 {object} documentLinkResponse
// @Router /v1/documents/{id}/link [get]
func (h handlers) documentLink(c *gin.Context) {
	id := c.Param("id")
	if !h.needDocRole(c, id, repo.RoleViewer) {
		return
	}
	comment, heading, version := c.Query("comment"), c.Query("heading"), c.Query("version")
	if len(heading) > 1024 || (comment != "" && (heading != "" || version != "")) {
		badRequest(c, "choose a comment, or a heading and optional version")
		return
	}
	link := h.headingURL(id, heading, version)
	if !h.linkVersionAllowed(c, id, version) {
		return
	}
	if comment != "" {
		cm, err := h.deps.repository.GetComment(c.Request.Context(), comment)
		if errors.Is(err, repo.ErrNotFound) || (err == nil && cm.DocumentID != id) {
			notFound(c)
			return
		}
		if err != nil {
			internalErr(c, err)
			return
		}
		link = notifications.CommentURL(h.deps.appOrigin, id, comment)
	}
	c.JSON(http.StatusOK, documentLinkResponse{URL: link})
}

func htmlID(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "id" {
			return a.Val
		}
	}
	return ""
}

func (h handlers) linkVersionAllowed(c *gin.Context, documentID, versionID string) bool {
	if versionID == "" {
		return true
	}
	v, err := h.deps.repository.GetVersion(c.Request.Context(), versionID)
	if errors.Is(err, repo.ErrNotFound) || (err == nil && v.DocumentID != documentID) {
		notFound(c)
		return false
	}
	if err != nil {
		internalErr(c, err)
		return false
	}
	return true
}

func isHeading(tag string) bool {
	return len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6'
}
