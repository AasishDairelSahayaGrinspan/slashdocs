package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func connectedKnowledgeBot() auth.Principal {
	return auth.Principal{Type: auth.PrincipalServiceAccount, ID: "sa_connected_knowledge", Name: "Connected Knowledge"}
}

func connectedKnowledgePRMarker(sourceID string) string {
	return "<!-- aidocs-connected-source:" + sourceID + " -->"
}

// upsertConnectedKnowledgePullRequest replaces the latest still-open pull
// request for a source with a new patchset. Merged and closed pull requests are
// immutable history, so a later revision creates a new pull request.
func upsertConnectedKnowledgePullRequest(ctx context.Context, deps dependencies, source knowledge.Source, document repo.Document, base repo.Version, summary string, generated []byte) (repo.PullRequest, bool, error) {
	changes := []repo.PullRequestChangeInput{{Operation: repo.PullRequestChangeUpdate, DocumentID: document.ID, BaseVersionID: base.ID, HTML: generated}}
	marker := connectedKnowledgePRMarker(source.ID)
	items, err := deps.repository.ListPullRequests(ctx, source.RepositoryID)
	if err != nil {
		return repo.PullRequest{}, false, err
	}
	var latest *repo.PullRequest
	for index := range items {
		item := &items[index]
		if item.Status != repo.PullRequestStatusOpen || item.Author.ID != connectedKnowledgeBot().ID || !strings.Contains(item.Body, marker) {
			continue
		}
		if latest == nil || item.Number > latest.Number {
			latest = item
		}
	}
	if latest != nil {
		updated, updateErr := deps.repository.CreatePullRequestPatchset(ctx, source.RepositoryID, latest.Number, summary, changes, connectedKnowledgeBot())
		return updated, true, updateErr
	}
	title := strings.TrimSpace(source.ExternalRepository)
	if title == "" {
		title = source.Provider + " source"
	}
	created, createErr := deps.repository.CreatePullRequestChangeSet(ctx, source.RepositoryID, "Sync "+title, marker+"\n"+summary, summary, changes, connectedKnowledgeBot())
	return created, false, createErr
}

func startKnowledgeProcessor(ctx context.Context, deps dependencies) <-chan struct{} {
	if err := deps.knowledge.RecoverSyncRuns(ctx); err != nil {
		slog.ErrorContext(ctx, "connected knowledge sync recovery failed", slog.Any("error", err))
	}
	interval := 2 * time.Second
	switch deps.environment {
	case envTest:
		interval = 10 * time.Millisecond
	case EnvDevelopment:
		interval = 200 * time.Millisecond
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			processKnowledgeQueue(ctx, deps)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

func processKnowledgeQueue(ctx context.Context, deps dependencies) {
	runs, err := deps.knowledge.ClaimQueuedSyncRuns(ctx, 10)
	if err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "connected knowledge queue claim failed", slog.Any("error", err))
		}
		return
	}
	for index, run := range runs {
		if ctx.Err() != nil {
			requeueClaimedKnowledgeRuns(ctx, deps, runs[index:])
			return
		}
		processKnowledgeRun(ctx, deps, run)
		if ctx.Err() != nil {
			requeueClaimedKnowledgeRuns(ctx, deps, runs[index:])
			return
		}
	}
}

// requeueClaimedKnowledgeRuns releases work claimed by this processor when its
// lifecycle is cancelled. The cleanup context intentionally outlives the
// cancelled worker context; Server.Close waits for this function before callers
// close the store.
func requeueClaimedKnowledgeRuns(ctx context.Context, deps dependencies, claimed []knowledge.SyncRun) {
	const cleanupTimeout = 5 * time.Second
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()

	claimedByRepository := map[string]map[string]struct{}{}
	for _, run := range claimed {
		if claimedByRepository[run.RepositoryID] == nil {
			claimedByRepository[run.RepositoryID] = map[string]struct{}{}
		}
		claimedByRepository[run.RepositoryID][run.ID] = struct{}{}
	}
	for repositoryID, ids := range claimedByRepository {
		runs, err := deps.knowledge.ListSyncRuns(cleanupCtx, repositoryID)
		if err != nil {
			slog.ErrorContext(cleanupCtx, "connected knowledge shutdown release failed", slog.String("repository_id", repositoryID), slog.Any("error", err))
			continue
		}
		for _, run := range runs {
			if _, ok := ids[run.ID]; !ok || run.Status != "processing" {
				continue
			}
			run.Status = "queued"
			run.Outcome = ""
			run.Message = ""
			run.PullRequestNumber = 0
			run.FinishedAt = ""
			if _, err := deps.knowledge.AddSyncRun(cleanupCtx, run); err != nil {
				slog.ErrorContext(cleanupCtx, "connected knowledge shutdown requeue failed", slog.String("repository_id", repositoryID), slog.String("sync_run_id", run.ID), slog.Any("error", err))
			}
		}
	}
}

//nolint:gocyclo // A sync run keeps all terminal outcomes in one state machine to guarantee a single completion path.
func processKnowledgeRun(ctx context.Context, deps dependencies, run knowledge.SyncRun) {
	settings, _ := deps.knowledge.GetSettings(ctx, run.RepositoryID)
	if settings.Archived {
		finishKnowledgeRun(ctx, deps, run, "completed", "no_op", "Repository is archived; restore it to resume connected-source updates.", nil, 0)
		return
	}
	source, err := deps.knowledge.GetSource(ctx, run.RepositoryID, run.SourceID)
	if err != nil {
		finishKnowledgeRun(ctx, deps, run, "failed", "failure", "source no longer exists", nil, 0)
		return
	}
	if run.AfterSHA != "" {
		if prior, err := deps.knowledge.ListSyncRuns(ctx, run.RepositoryID); err == nil {
			for _, item := range prior {
				if item.ID != run.ID && item.SourceID == run.SourceID && item.AfterSHA == run.AfterSHA && item.Status == "completed" {
					finishKnowledgeRun(ctx, deps, run, "completed", "no_op", "This provider revision was already processed.", nil, 0)
					return
				}
			}
		}
	}
	files := run.ChangedFiles
	if len(files) == 0 && run.BeforeSHA != "" && run.AfterSHA != "" {
		comparison, compareErr := deps.providerClient.Compare(ctx, source, run.BeforeSHA, run.AfterSHA)
		if compareErr != nil {
			finishKnowledgeRun(ctx, deps, run, "failed", "failure", compareErr.Error(), nil, 0)
			return
		}
		files = comparison.ChangedFiles
	}
	files = filterSourceFiles(source, files)
	if len(files) == 0 {
		finishKnowledgeRun(ctx, deps, run, "completed", "no_op", "No included files changed.", files, 0)
		return
	}
	if source.TargetDocumentID == "" {
		finishKnowledgeRun(ctx, deps, run, "completed", "preview", "Meaningful source changes found; configure target_document_id to open an aidocs pull request.", files, 0)
		return
	}
	document, err := deps.repository.GetDocument(ctx, source.TargetDocumentID)
	if err != nil {
		finishKnowledgeRun(ctx, deps, run, "failed", "failure", "target document not found", files, 0)
		return
	}
	base, err := deps.repository.GetVersion(ctx, document.CurrentVersionID)
	if err != nil {
		finishKnowledgeRun(ctx, deps, run, "failed", "failure", "could not read target version", files, 0)
		return
	}
	body, err := deps.repository.GetVersionHTML(ctx, document.CurrentVersionID)
	if err != nil {
		finishKnowledgeRun(ctx, deps, run, "failed", "failure", "could not read target document", files, 0)
		return
	}
	summary := fmt.Sprintf("Sync %s/%s through %s", source.Provider, source.ExternalRepository, run.AfterSHA)
	pullRequest, updated, err := upsertConnectedKnowledgePullRequest(ctx, deps, source, document, base, summary, appendSourceSummary(body, source, run.AfterSHA, files, run.ContextItems...))
	if err != nil {
		finishKnowledgeRun(ctx, deps, run, "failed", "failure", err.Error(), files, 0)
		return
	}
	run.Provenance = []knowledge.Citation{{DocumentID: document.ID, Title: document.Title, VersionID: pullRequest.HeadVersionID, SourceID: source.ID, Provider: source.Provider, URL: providerRevisionURL(source, run.AfterSHA)}}
	outcome, message := "pull_request_opened", "Documentation changes are ready for review."
	if updated {
		outcome, message = "pull_request_updated", "The open documentation pull request has a new patchset ready for review."
	}
	finishKnowledgeRun(ctx, deps, run, "completed", outcome, message, files, pullRequest.Number)
	settings, _ = deps.knowledge.GetSettings(ctx, run.RepositoryID)
	recipients := append([]string{}, settings.RequiredReviewers...)
	requiredTeams := append([]string{}, settings.RequiredTeams...)
	for _, rule := range settings.PathRules {
		matched := false
		for _, file := range files {
			if globMatch(rule.Pattern, file) {
				matched = true
				break
			}
		}
		if matched {
			recipients = append(recipients, rule.RequiredReviewers...)
			requiredTeams = append(requiredTeams, rule.RequiredTeams...)
		}
	}
	teams, _ := deps.knowledge.ListTeams(ctx, run.RepositoryID)
	for _, team := range teams {
		for _, required := range requiredTeams {
			if required == team.ID || strings.EqualFold(required, team.Name) {
				recipients = append(recipients, team.MemberIDs...)
			}
		}
	}
	for _, userID := range uniqueStrings(recipients) {
		_ = deps.knowledge.AddNotification(ctx, knowledge.Notification{ID: knowledgeID("notification"), UserID: userID, RepositoryID: run.RepositoryID, Type: "review_requested", Title: "Documentation review requested", Body: "A connected source opened or updated a documentation pull request.", URL: fmt.Sprintf("/app/repos/%s/pulls/%d", run.RepositoryID, pullRequest.Number), CreatedAt: knowledge.Now()})
	}
}

func finishKnowledgeRun(ctx context.Context, deps dependencies, run knowledge.SyncRun, status, outcome, message string, files []string, pullRequestNumber int64) {
	run.Status = status
	run.Outcome = outcome
	run.Message = message
	run.ChangedFiles = files
	run.PullRequestNumber = pullRequestNumber
	run.FinishedAt = knowledge.Now()
	_, _ = deps.knowledge.AddSyncRun(ctx, run)
	if source, err := deps.knowledge.GetSource(ctx, run.RepositoryID, run.SourceID); err == nil {
		state := "fresh"
		if status == "failed" {
			state = "error"
		}
		source.Freshness.State = state
		source.Freshness.CheckedAt = knowledge.Now()
		source.LastSyncedAt = knowledge.Now()
		source.UpdatedAt = knowledge.Now()
		_, _ = deps.knowledge.PutSource(ctx, source)
	}
	_ = deps.knowledge.AddAudit(ctx, knowledge.AuditEvent{ID: knowledgeID("audit"), RepositoryID: run.RepositoryID, Action: "source.sync_" + outcome, Actor: connectedKnowledgeBot(), SubjectType: "source", SubjectID: run.SourceID, Details: map[string]any{"sync_run_id": run.ID, "pull_request_number": pullRequestNumber}, CreatedAt: knowledge.Now()})
}
