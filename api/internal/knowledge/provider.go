package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultGitHubAPIBaseURL      = "https://api.github.com"
	defaultSlackAPIBaseURL       = "https://slack.com/api"
	defaultGoogleDriveAPIBaseURL = "https://www.googleapis.com/drive/v3"
	defaultDevRevAPIBaseURL      = "https://api.devrev.ai"
	maxProviderResponseBytes     = 10 << 20
	maxSlackPages                = 100
	maxContextPaths              = 200
)

var (
	ErrProviderCredentialUnavailable = errors.New("provider credential unavailable")
	devRevDisplayIDPattern           = regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9_]*-[0-9]+\b`)
)

type CompareResult struct {
	ChangedFiles []string `json:"changed_files"`
	WebURL       string   `json:"web_url,omitempty"`
}

// InspectResult is the provider-neutral material returned by an explicit
// source check. Revision is an opaque cursor: callers persist it and pass it
// back to the next inspection.
type InspectResult struct {
	Revision      string        `json:"revision"`
	ChangedFiles  []string      `json:"changed_files"`
	ContextItems  []ContextItem `json:"context_items,omitempty"`
	ProvenanceURL string        `json:"provenance_url,omitempty"`
	Unchanged     bool          `json:"unchanged"`
}

// ProviderClient isolates remote provider access from webhook ingestion. A
// webhook only records queued work; explicit checks may use this interface.
// Compare remains intentionally small for compatibility with webhook clients.
type ProviderClient interface {
	Compare(context.Context, Source, string, string) (CompareResult, error)
}

// ProviderInspector is implemented by clients that can perform a complete,
// provider-specific manual source check.
type ProviderInspector interface {
	Inspect(context.Context, Source, string) (InspectResult, error)
}

// AccessTokenResolver returns the current access token for a user-owned
// connection. Implementations may refresh an expired token before returning.
type AccessTokenResolver func(context.Context, string) (string, error)

// ProviderHTTPError deliberately excludes response bodies and request URLs so
// provider credentials can never be reflected into user-visible errors.
type ProviderHTTPError struct {
	Provider   string
	StatusCode int
}

func (e *ProviderHTTPError) Error() string {
	return fmt.Sprintf("%s request returned HTTP %d", e.Provider, e.StatusCode)
}

// RateLimitError exposes only the provider and safe Retry-After value.
type RateLimitError struct {
	Provider   string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter <= 0 {
		return fmt.Sprintf("%s request was rate limited", e.Provider)
	}
	return fmt.Sprintf("%s request was rate limited; retry after %s", e.Provider, e.RetryAfter)
}

type HTTPProviderClient struct {
	Client              *http.Client
	GitHubToken         string
	AccessTokenResolver AccessTokenResolver

	// API base URLs are configurable for private proxies and deterministic
	// integration tests. Empty values use the public provider endpoints.
	GitHubAPIBaseURL      string
	SlackAPIBaseURL       string
	GoogleDriveAPIBaseURL string
	DevRevAPIBaseURL      string
}

func (p HTTPProviderClient) Compare(ctx context.Context, source Source, before, after string) (CompareResult, error) {
	if source.Provider != ProviderGitHub {
		return CompareResult{}, fmt.Errorf("provider %q does not have an HTTP compare client", source.Provider)
	}
	if before == "" || after == "" {
		return CompareResult{}, errors.New("before_sha and after_sha are required")
	}
	owner, repository, err := githubRepositoryParts(source.ExternalRepository)
	if err != nil {
		return CompareResult{}, err
	}
	token, err := p.accessToken(ctx, source, false)
	if err != nil {
		return CompareResult{}, err
	}
	return p.githubCompare(ctx, owner, repository, before, after, token)
}

func (p HTTPProviderClient) Inspect(ctx context.Context, source Source, previousRevision string) (InspectResult, error) {
	switch source.Provider {
	case ProviderGitHub:
		return p.inspectGitHub(ctx, source, previousRevision)
	case ProviderSlack:
		return p.inspectSlack(ctx, source, previousRevision)
	case ProviderGoogleDrive:
		return p.inspectGoogleDrive(ctx, source, previousRevision)
	case ProviderDevRev:
		return p.inspectDevRev(ctx, source, previousRevision)
	default:
		return InspectResult{}, fmt.Errorf("provider %q does not support source inspection", source.Provider)
	}
}

func (p HTTPProviderClient) inspectGitHub(ctx context.Context, source Source, previousRevision string) (InspectResult, error) {
	owner, repository, err := githubRepositoryParts(source.ExternalRepository)
	if err != nil {
		return InspectResult{}, err
	}
	token, err := p.accessToken(ctx, source, false)
	if err != nil {
		return InspectResult{}, err
	}
	branch := strings.TrimSpace(source.Branch)
	if branch == "" {
		branch = "main"
	}
	prefix := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository)
	endpoint := joinBaseURL(p.githubBaseURL(), prefix+"/branches/"+url.PathEscape(branch))
	var branchPayload struct {
		Commit struct {
			SHA     string `json:"sha"`
			HTMLURL string `json:"html_url"`
		} `json:"commit"`
	}
	if err := p.getJSON(ctx, ProviderGitHub, endpoint, token, githubHeaders(), &branchPayload); err != nil {
		return InspectResult{}, err
	}
	head := strings.TrimSpace(branchPayload.Commit.SHA)
	if head == "" {
		return InspectResult{}, errors.New("github returned an invalid branch response")
	}
	provenance := strings.TrimSpace(branchPayload.Commit.HTMLURL)
	if provenance == "" {
		provenance = "https://github.com/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/commit/" + url.PathEscape(head)
	}
	if previousRevision != "" && previousRevision == head {
		return InspectResult{Revision: head, ChangedFiles: []string{}, ProvenanceURL: provenance, Unchanged: true}, nil
	}

	if previousRevision != "" {
		comparison, compareErr := p.githubCompare(ctx, owner, repository, previousRevision, head, token)
		if compareErr != nil {
			return InspectResult{}, compareErr
		}
		changed := normalizedPaths(comparison.ChangedFiles)
		if comparison.WebURL != "" {
			provenance = comparison.WebURL
		}
		return InspectResult{
			Revision:      head,
			ChangedFiles:  changed,
			ContextItems:  []ContextItem{githubContextItem(owner, repository, branch, previousRevision, head, changed, provenance, false)},
			ProvenanceURL: provenance,
		}, nil
	}

	treeEndpoint := joinBaseURL(p.githubBaseURL(), prefix+"/git/trees/"+url.PathEscape(head)) + "?recursive=1"
	var treePayload struct {
		Truncated bool `json:"truncated"`
		Tree      []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := p.getJSON(ctx, ProviderGitHub, treeEndpoint, token, githubHeaders(), &treePayload); err != nil {
		return InspectResult{}, err
	}
	paths := make([]string, 0, len(treePayload.Tree))
	for _, entry := range treePayload.Tree {
		if entry.Type == "blob" && strings.TrimSpace(entry.Path) != "" {
			paths = append(paths, entry.Path)
		}
	}
	paths = normalizedPaths(paths)
	item := githubContextItem(owner, repository, branch, "", head, paths, provenance, true)
	item.Metadata["tree_truncated"] = strconv.FormatBool(treePayload.Truncated)
	return InspectResult{
		Revision:      head,
		ChangedFiles:  paths,
		ContextItems:  []ContextItem{item},
		ProvenanceURL: provenance,
	}, nil
}

func (p HTTPProviderClient) githubCompare(ctx context.Context, owner, repository, before, after, token string) (CompareResult, error) {
	endpoint := joinBaseURL(p.githubBaseURL(), "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repository)+"/compare/"+url.PathEscape(before)+"..."+url.PathEscape(after))
	var payload struct {
		HTMLURL string `json:"html_url"`
		Files   []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}
	if err := p.getJSON(ctx, ProviderGitHub, endpoint, token, githubHeaders(), &payload); err != nil {
		return CompareResult{}, err
	}
	changed := make([]string, 0, len(payload.Files))
	for _, file := range payload.Files {
		changed = append(changed, file.Filename)
	}
	return CompareResult{WebURL: payload.HTMLURL, ChangedFiles: normalizedPaths(changed)}, nil
}

func (p HTTPProviderClient) inspectSlack(ctx context.Context, source Source, previousRevision string) (InspectResult, error) {
	teamID, channelID, threadTS, err := slackThreadParts(source.ExternalRepository)
	if err != nil {
		return InspectResult{}, err
	}
	token, err := p.accessToken(ctx, source, true)
	if err != nil {
		return InspectResult{}, err
	}

	type slackMessage struct {
		Type     string `json:"type,omitempty"`
		Subtype  string `json:"subtype,omitempty"`
		Channel  string `json:"channel,omitempty"`
		TS       string `json:"ts,omitempty"`
		ThreadTS string `json:"thread_ts,omitempty"`
		Text     string `json:"text,omitempty"`
		User     string `json:"user,omitempty"`
		BotID    string `json:"bot_id,omitempty"`
	}
	messages := make([]slackMessage, 0)
	cursor := ""
	seenCursors := map[string]bool{}
	for page := 0; page < maxSlackPages; page++ {
		query := url.Values{"channel": {channelID}, "ts": {threadTS}, "limit": {"15"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		endpoint := joinBaseURL(p.slackBaseURL(), "/conversations.replies") + "?" + query.Encode()
		var payload struct {
			OK       bool           `json:"ok"`
			Messages []slackMessage `json:"messages"`
			Metadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := p.getJSON(ctx, ProviderSlack, endpoint, token, nil, &payload); err != nil {
			return InspectResult{}, err
		}
		if !payload.OK {
			return InspectResult{}, errors.New("slack API request failed")
		}
		messages = append(messages, payload.Messages...)
		next := strings.TrimSpace(payload.Metadata.NextCursor)
		if next == "" {
			break
		}
		if seenCursors[next] {
			return InspectResult{}, errors.New("slack returned an invalid pagination cursor")
		}
		seenCursors[next] = true
		cursor = next
		if page == maxSlackPages-1 {
			return InspectResult{}, errors.New("slack thread exceeded the inspection page limit")
		}
	}

	canonical, err := json.Marshal(struct {
		TeamID    string         `json:"team_id"`
		ChannelID string         `json:"channel_id"`
		ThreadTS  string         `json:"thread_ts"`
		Messages  []slackMessage `json:"messages"`
	}{teamID, channelID, threadTS, messages})
	if err != nil {
		return InspectResult{}, errors.New("slack response could not be inspected")
	}
	revision := contentRevision(canonical)
	provenance := slackThreadURL(teamID, channelID, threadTS)
	unchanged := previousRevision != "" && previousRevision == revision
	changed := []string{}
	if !unchanged {
		changed = []string{"slack/" + safePathComponent(teamID) + "/" + safePathComponent(channelID) + "/threads/" + safePathComponent(threadTS) + ".txt"}
	}
	items := make([]ContextItem, 0, len(messages))
	for _, message := range messages {
		author := message.User
		if author == "" {
			author = message.BotID
		}
		items = append(items, ContextItem{
			Kind:       "slack_message",
			ExternalID: message.TS,
			Title:      "Slack thread message",
			Body:       message.Text,
			URL:        provenance,
			Metadata: nonEmptyMetadata(map[string]string{
				"team_id": teamID, "channel_id": channelID, "thread_ts": threadTS,
				"author_id": author, "message_ts": message.TS, "subtype": message.Subtype,
			}),
		})
	}
	return InspectResult{Revision: revision, ChangedFiles: changed, ContextItems: items, ProvenanceURL: provenance, Unchanged: unchanged}, nil
}

func (p HTTPProviderClient) inspectGoogleDrive(ctx context.Context, source Source, previousRevision string) (InspectResult, error) {
	fileID, err := googleDriveFileID(source.ExternalRepository)
	if err != nil {
		return InspectResult{}, err
	}
	token, err := p.accessToken(ctx, source, true)
	if err != nil {
		return InspectResult{}, err
	}
	fields := "id,name,mimeType,modifiedTime,version,trashed,webViewLink,capabilities(canDownload)"
	query := url.Values{"fields": {fields}, "supportsAllDrives": {"true"}}
	metadataEndpoint := joinBaseURL(p.googleDriveBaseURL(), "/files/"+url.PathEscape(fileID)) + "?" + query.Encode()
	var metadata struct {
		ID           string          `json:"id"`
		Name         string          `json:"name"`
		MIMEType     string          `json:"mimeType"`
		ModifiedTime string          `json:"modifiedTime"`
		Version      json.RawMessage `json:"version"`
		Trashed      bool            `json:"trashed"`
		WebViewLink  string          `json:"webViewLink"`
		Capabilities struct {
			CanDownload *bool `json:"canDownload"`
		} `json:"capabilities"`
	}
	if err := p.getJSON(ctx, ProviderGoogleDrive, metadataEndpoint, token, nil, &metadata); err != nil {
		return InspectResult{}, err
	}
	if metadata.Trashed {
		return InspectResult{}, errors.New("google drive file is in the trash")
	}
	if metadata.Capabilities.CanDownload != nil && !*metadata.Capabilities.CanDownload {
		return InspectResult{}, errors.New("google drive file cannot be downloaded")
	}
	if metadata.ID == "" {
		metadata.ID = fileID
	}
	content, err := p.downloadGoogleDriveText(ctx, fileID, metadata.MIMEType, token)
	if err != nil {
		return InspectResult{}, err
	}
	version := rawJSONScalar(metadata.Version)
	canonical, err := json.Marshal(struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		MIMEType     string `json:"mime_type"`
		ModifiedTime string `json:"modified_time"`
		Version      string `json:"version"`
		Content      string `json:"content"`
	}{metadata.ID, metadata.Name, metadata.MIMEType, metadata.ModifiedTime, version, string(content)})
	if err != nil {
		return InspectResult{}, errors.New("google drive response could not be inspected")
	}
	revision := contentRevision(canonical)
	unchanged := previousRevision != "" && previousRevision == revision
	changed := []string{}
	name := strings.TrimSpace(metadata.Name)
	if name == "" {
		name = fileID
	}
	if !unchanged {
		changed = []string{"drive/" + safePathComponent(name)}
	}
	provenance := strings.TrimSpace(metadata.WebViewLink)
	if provenance == "" {
		provenance = "https://drive.google.com/open?id=" + url.QueryEscape(fileID)
	}
	item := ContextItem{
		Kind:       "google_drive_file",
		ExternalID: metadata.ID,
		Title:      name,
		Body:       string(content),
		URL:        provenance,
		Metadata: nonEmptyMetadata(map[string]string{
			"mime_type": metadata.MIMEType, "modified_time": metadata.ModifiedTime, "version": version,
		}),
	}
	return InspectResult{Revision: revision, ChangedFiles: changed, ContextItems: []ContextItem{item}, ProvenanceURL: provenance, Unchanged: unchanged}, nil
}

func (p HTTPProviderClient) downloadGoogleDriveText(ctx context.Context, fileID, mimeType, token string) ([]byte, error) {
	base := joinBaseURL(p.googleDriveBaseURL(), "/files/"+url.PathEscape(fileID))
	var endpoint string
	switch mimeType {
	case "application/vnd.google-apps.document", "application/vnd.google-apps.presentation":
		endpoint = base + "/export?" + url.Values{"mimeType": {"text/plain"}}.Encode()
	case "application/vnd.google-apps.spreadsheet":
		endpoint = base + "/export?" + url.Values{"mimeType": {"text/csv"}}.Encode()
	default:
		if !isTextMIMEType(mimeType) {
			return nil, errors.New("google drive file is not a supported text document")
		}
		endpoint = base + "?" + url.Values{"alt": {"media"}, "supportsAllDrives": {"true"}}.Encode()
	}
	return p.getText(ctx, ProviderGoogleDrive, endpoint, token)
}

func (p HTTPProviderClient) inspectDevRev(ctx context.Context, source Source, previousRevision string) (InspectResult, error) {
	workID, err := devRevWorkID(source.ExternalRepository)
	if err != nil {
		return InspectResult{}, err
	}
	token, err := p.accessToken(ctx, source, true)
	if err != nil {
		return InspectResult{}, err
	}
	body, err := json.Marshal(map[string]string{"id": workID})
	if err != nil {
		return InspectResult{}, errors.New("devrev request could not be created")
	}
	endpoint := joinBaseURL(p.devRevBaseURL(), "/works.get")
	var payload struct {
		Work json.RawMessage `json:"work"`
	}
	if err := p.postJSON(ctx, ProviderDevRev, endpoint, token, body, &payload); err != nil {
		return InspectResult{}, err
	}
	if len(payload.Work) == 0 || string(payload.Work) == "null" {
		return InspectResult{}, errors.New("devrev returned an invalid work response")
	}
	var work map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload.Work))
	decoder.UseNumber()
	if err := decoder.Decode(&work); err != nil || len(work) == 0 {
		return InspectResult{}, errors.New("devrev returned an invalid work response")
	}
	canonical, err := json.Marshal(work)
	if err != nil {
		return InspectResult{}, errors.New("devrev response could not be inspected")
	}
	revision := contentRevision(canonical)
	unchanged := previousRevision != "" && previousRevision == revision
	displayID := firstNonEmpty(stringMapValue(work, "display_id"), stringMapValue(work, "id"), workID)
	changed := []string{}
	if !unchanged {
		changed = []string{"devrev/works/" + safePathComponent(displayID) + ".txt"}
	}
	provenance := firstNonEmpty(stringMapValue(work, "web_url"), sourceURL(source.ExternalRepository))
	state := firstNonEmpty(stringMapValue(work, "state"), nestedStringMapValue(work, "stage", "name"))
	item := ContextItem{
		Kind:       "devrev_work",
		ExternalID: firstNonEmpty(stringMapValue(work, "id"), displayID),
		Title:      firstNonEmpty(stringMapValue(work, "title"), displayID),
		Body:       firstNonEmpty(stringMapValue(work, "body"), stringMapValue(work, "description")),
		URL:        provenance,
		Metadata: nonEmptyMetadata(map[string]string{
			"display_id": displayID, "type": stringMapValue(work, "type"), "state": state,
			"modified_date": firstNonEmpty(stringMapValue(work, "modified_date"), stringMapValue(work, "updated_at")),
		}),
	}
	return InspectResult{Revision: revision, ChangedFiles: changed, ContextItems: []ContextItem{item}, ProvenanceURL: provenance, Unchanged: unchanged}, nil
}

func (p HTTPProviderClient) accessToken(ctx context.Context, source Source, required bool) (string, error) {
	if source.ConnectionID != "" {
		if p.AccessTokenResolver == nil {
			return "", fmt.Errorf("%w for %s", ErrProviderCredentialUnavailable, source.Provider)
		}
		token, err := p.AccessTokenResolver(ctx, source.ConnectionID)
		if err != nil || strings.TrimSpace(token) == "" {
			// Resolver errors are deliberately discarded: they may contain raw
			// credential-manager values or provider response bodies.
			return "", fmt.Errorf("%w for %s", ErrProviderCredentialUnavailable, source.Provider)
		}
		return strings.TrimSpace(token), nil
	}
	if source.Provider == ProviderGitHub {
		return strings.TrimSpace(p.GitHubToken), nil
	}
	if required {
		return "", fmt.Errorf("%w for %s", ErrProviderCredentialUnavailable, source.Provider)
	}
	return "", nil
}

func (p HTTPProviderClient) getJSON(ctx context.Context, provider, endpoint, token string, headers map[string]string, target any) error {
	resp, err := p.do(ctx, provider, http.MethodGet, endpoint, token, headers, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkedProviderResponse(provider, resp); err != nil {
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s returned an invalid JSON response", provider)
	}
	return nil
}

func (p HTTPProviderClient) postJSON(ctx context.Context, provider, endpoint, token string, body []byte, target any) error {
	resp, err := p.do(ctx, provider, http.MethodPost, endpoint, token, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkedProviderResponse(provider, resp); err != nil {
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s returned an invalid JSON response", provider)
	}
	return nil
}

func (p HTTPProviderClient) getText(ctx context.Context, provider, endpoint, token string) ([]byte, error) {
	resp, err := p.do(ctx, provider, http.MethodGet, endpoint, token, nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkedProviderResponse(provider, resp); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s response could not be read", provider)
	}
	if len(content) > maxProviderResponseBytes {
		return nil, fmt.Errorf("%s response exceeds the inspection size limit", provider)
	}
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("%s response is not UTF-8 text", provider)
	}
	return content, nil
}

func (p HTTPProviderClient) do(ctx context.Context, provider, method, endpoint, token string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("%s request could not be created", provider)
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%s request failed", provider)
	}
	return resp, nil
}

func checkedProviderResponse(provider string, resp *http.Response) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		return &RateLimitError{Provider: provider, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ProviderHTTPError{Provider: provider, StatusCode: resp.StatusCode}
	}
	return nil
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now).Round(time.Second)
	}
	return 0
}

func githubRepositoryParts(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		value = strings.Trim(parsed.Path, "/")
	}
	value = strings.TrimSuffix(strings.Trim(value, "/"), ".git")
	parts := strings.Split(value, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", errors.New("github source must identify an owner/repository")
	}
	return parts[0], parts[1], nil
}

func slackThreadParts(value string) (string, string, string, error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(value), "/"), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", errors.New("slack source must identify team/channel/thread_ts")
	}
	return parts[0], parts[1], parts[2], nil
}

func googleDriveFileID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("google drive source must identify a file")
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		if id := strings.TrimSpace(parsed.Query().Get("id")); id != "" {
			return id, nil
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for index := 0; index+1 < len(parts); index++ {
			if parts[index] == "d" && parts[index+1] != "" {
				return parts[index+1], nil
			}
		}
		return "", errors.New("google drive URL does not identify a file")
	}
	if strings.ContainsAny(value, "/?#& ") {
		return "", errors.New("google drive source must identify a file")
	}
	return value, nil
}

func devRevWorkID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("devrev source must identify a work item")
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		if id := strings.TrimSpace(parsed.Query().Get("id")); id != "" {
			return id, nil
		}
	}
	if id := devRevDisplayIDPattern.FindString(value); id != "" {
		return strings.ToUpper(id), nil
	}
	if !strings.ContainsAny(value, " /?#&") {
		return value, nil
	}
	return "", errors.New("devrev source must identify a work item")
}

func sourceURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" {
		return parsed.String()
	}
	return ""
}

func githubHeaders() map[string]string {
	return map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}
}

func githubContextItem(owner, repository, branch, before, after string, paths []string, provenance string, firstSync bool) ContextItem {
	kind := "github_compare"
	title := "GitHub changes on " + branch
	if firstSync {
		kind = "github_repository_snapshot"
		title = "GitHub repository snapshot on " + branch
	}
	return ContextItem{
		Kind:       kind,
		ExternalID: after,
		Title:      title,
		Body:       summarizePaths(paths),
		URL:        provenance,
		Metadata: nonEmptyMetadata(map[string]string{
			"repository": owner + "/" + repository, "branch": branch, "before_revision": before,
			"after_revision": after, "path_count": strconv.Itoa(len(paths)),
		}),
	}
}

func summarizePaths(paths []string) string {
	if len(paths) == 0 {
		return "No changed document paths were returned by the provider."
	}
	limit := len(paths)
	if limit > maxContextPaths {
		limit = maxContextPaths
	}
	body := strings.Join(paths[:limit], "\n")
	if limit < len(paths) {
		body += fmt.Sprintf("\n… and %d more paths", len(paths)-limit)
	}
	return body
}

func slackThreadURL(teamID, channelID, threadTS string) string {
	return "https://app.slack.com/client/" + url.PathEscape(teamID) + "/" + url.PathEscape(channelID) + "/thread/" + url.PathEscape(channelID+"-"+threadTS)
}

func isTextMIMEType(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	return strings.HasPrefix(mimeType, "text/") || mimeType == "application/json" || mimeType == "application/xml" || mimeType == "application/xhtml+xml" || mimeType == "application/yaml" || mimeType == "application/x-yaml"
}

func rawJSONScalar(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return ""
	}
	switch typed := decoded.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func stringMapValue(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func nestedStringMapValue(values map[string]any, key, nested string) string {
	child, ok := values[key].(map[string]any)
	if !ok {
		return ""
	}
	return stringMapValue(child, nested)
}

func nonEmptyMetadata(values map[string]string) map[string]string {
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			delete(values, key)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func normalizedPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		normalized = append(normalized, path)
	}
	sort.Strings(normalized)
	return normalized
}

func safePathComponent(value string) string {
	value = strings.TrimSpace(value)
	var safe strings.Builder
	lastDash := false
	for _, character := range value {
		allowed := (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-'
		if allowed {
			safe.WriteRune(character)
			lastDash = false
			continue
		}
		if safe.Len() > 0 && !lastDash {
			safe.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(safe.String(), "-.")
	if result == "" || result == "." || result == ".." {
		return "source"
	}
	return result
}

func contentRevision(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func joinBaseURL(base, suffix string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
}

func (p HTTPProviderClient) githubBaseURL() string {
	if strings.TrimSpace(p.GitHubAPIBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(p.GitHubAPIBaseURL), "/")
	}
	return defaultGitHubAPIBaseURL
}

func (p HTTPProviderClient) slackBaseURL() string {
	if strings.TrimSpace(p.SlackAPIBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(p.SlackAPIBaseURL), "/")
	}
	return defaultSlackAPIBaseURL
}

func (p HTTPProviderClient) googleDriveBaseURL() string {
	if strings.TrimSpace(p.GoogleDriveAPIBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(p.GoogleDriveAPIBaseURL), "/")
	}
	return defaultGoogleDriveAPIBaseURL
}

func (p HTTPProviderClient) devRevBaseURL() string {
	if strings.TrimSpace(p.DevRevAPIBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(p.DevRevAPIBaseURL), "/")
	}
	return defaultDevRevAPIBaseURL
}
