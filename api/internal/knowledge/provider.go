package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type CompareResult struct {
	ChangedFiles []string `json:"changed_files"`
	WebURL       string   `json:"web_url,omitempty"`
}

// ProviderClient isolates remote provider access from webhook ingestion. A
// webhook only records queued work; explicit checks may use this interface.
type ProviderClient interface {
	Compare(context.Context, Source, string, string) (CompareResult, error)
}

type HTTPProviderClient struct {
	Client      *http.Client
	GitHubToken string
}

func (p HTTPProviderClient) Compare(ctx context.Context, source Source, before, after string) (CompareResult, error) {
	if source.Provider != ProviderGitHub {
		return CompareResult{}, fmt.Errorf("provider %q does not have an HTTP compare client", source.Provider)
	}
	if source.ExternalRepository == "" || before == "" || after == "" {
		return CompareResult{}, fmt.Errorf("external_repository, before_sha, and after_sha are required")
	}
	endpoint := "https://api.github.com/repos/" + strings.Trim(source.ExternalRepository, "/") + "/compare/" + url.PathEscape(before) + "..." + url.PathEscape(after)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CompareResult{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if p.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.GitHubToken)
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return CompareResult{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CompareResult{}, fmt.Errorf("github compare returned %s", resp.Status)
	}
	var payload struct {
		HTMLURL string `json:"html_url"`
		Files   []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CompareResult{}, err
	}
	out := CompareResult{WebURL: payload.HTMLURL, ChangedFiles: []string{}}
	for _, file := range payload.Files {
		out.ChangedFiles = append(out.ChangedFiles, file.Filename)
	}
	return out, nil
}
