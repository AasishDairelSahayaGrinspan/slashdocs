package server

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func commentsJSON(items []repo.Comment, placementVersionID string, placementHTML []byte) []gin.H {
	// Stringify the version body once for all placement checks below.
	haystack := string(placementHTML)
	roots := make([]repo.Comment, 0, len(items))
	byID := map[string]int{}
	for _, cm := range items {
		if cm.ParentCommentID == "" {
			byID[cm.ID] = len(roots)
			if cm.Replies == nil {
				cm.Replies = []repo.Comment{}
			}
			roots = append(roots, cm)
		}
	}
	for _, cm := range items {
		if cm.ParentCommentID == "" {
			continue
		}
		rootID := cm.RootCommentID
		if rootID == "" {
			rootID = cm.ParentCommentID
		}
		if i, ok := byID[rootID]; ok {
			roots[i].Replies = append(roots[i].Replies, cm)
		}
	}
	out := make([]gin.H, 0, len(roots))
	for _, cm := range roots {
		out = append(out, commentJSON(cm, placementVersionID, haystack))
	}
	return out
}

// commentPlacement reports whether a comment's selected text still appears in
// the given version, returning its placement status, a confidence score, and
// the matched text. placementHTML is the already-stringified version body.
func commentPlacement(cm repo.Comment, placementVersionID, placementHTML string) (status string, confidence float64, matched string) {
	status = placementAttached
	confidence = 1.0
	matched = cm.SelectedText
	if placementVersionID != cm.VersionID && len(placementHTML) > 0 && !strings.Contains(placementHTML, cm.SelectedText) {
		status = placementOrphaned
		confidence = 0
		matched = ""
	}
	return
}

// userJSON renders a user principal without the "type" field, for endpoints
// that already convey the principal type separately.
func userJSON(p auth.Principal) gin.H {
	out := principalJSON(p)
	delete(out, "type")
	return out
}

func principalJSON(p auth.Principal) gin.H {
	out := gin.H{"type": p.Type, "id": p.ID}
	if p.Email != "" {
		out["email"] = p.Email
	}
	if p.Name != "" {
		out["name"] = p.Name
	}
	if p.PictureURL != "" {
		out["picture_url"] = p.PictureURL
	}
	return out
}

func commentJSON(cm repo.Comment, placementVersionID, placementHTML string) gin.H {
	out := commentBaseJSON(cm)
	if cm.ParentCommentID == "" {
		if placementVersionID == "" {
			placementVersionID = cm.VersionID
		}
		status, confidence, matched := commentPlacement(cm, placementVersionID, placementHTML)
		out["current_placement"] = gin.H{"version_id": placementVersionID, "status": status, "anchor": cm.Anchor, "matched_text": matched, "confidence": confidence}
		if cm.Replies != nil {
			out["replies"] = commentsReplyJSON(cm.Replies)
		}
	}
	return out
}

func commentBaseJSON(cm repo.Comment) gin.H {
	out := gin.H{"id": cm.ID, "author": principalJSON(cm.Author), "body": cm.Body, "status": cm.Status, "created_on_version_id": cm.VersionID}
	if cm.ParentCommentID != "" {
		out["parent_comment_id"] = cm.ParentCommentID
	}
	if cm.SelectedText != "" {
		out["selected_text"] = cm.SelectedText
		out["anchor"] = cm.Anchor
	}
	return out
}

func commentsReplyJSON(items []repo.Comment) []gin.H {
	out := make([]gin.H, 0, len(items))
	for _, cm := range items {
		out = append(out, commentBaseJSON(cm))
	}
	return out
}
