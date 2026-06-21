// api/internal/server/drive_url.go
package server

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// bareIDPattern matches a plausible Drive file id (alphanumeric, dash, underscore).
var bareIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// fileDPathPattern extracts the id from a /file/d/<ID>/... path.
var fileDPathPattern = regexp.MustCompile(`/file/d/([A-Za-z0-9_-]+)`)

// parseDriveFileRef extracts a Drive file id (and optional resource key) from a
// Drive share URL or a bare id. It rejects native Google Docs URLs
// (docs.google.com/document|spreadsheets|presentation), which are not raw HTML
// files, and anything it cannot recognise.
func parseDriveFileRef(raw string) (string, string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", errors.New("empty drive link")
	}
	// Bare id (no scheme, no slash): accept as-is.
	if !strings.Contains(s, "/") && bareIDPattern.MatchString(s) {
		return s, "", nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", "", errors.New("invalid drive link")
	}
	host := strings.ToLower(u.Hostname())
	if host == "docs.google.com" {
		return "", "", errors.New("this is a Google Docs/Sheets/Slides link, not an HTML file in Drive")
	}
	if host != "drive.google.com" {
		return "", "", errors.New("not a Google Drive link")
	}
	rk := u.Query().Get("resourcekey")
	// drive.google.com/file/d/<ID>/view
	if m := fileDPathPattern.FindStringSubmatch(u.Path); m != nil {
		return m[1], rk, nil
	}
	// drive.google.com/open?id=<ID> or /uc?id=<ID>
	if id := u.Query().Get("id"); id != "" && bareIDPattern.MatchString(id) {
		return id, rk, nil
	}
	return "", "", errors.New("could not find a file id in the drive link")
}
