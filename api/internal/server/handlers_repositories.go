package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func (h handlers) needRepositoryRole(c *gin.Context, repositoryID string, need repo.Role) bool {
	role, _ := h.deps.repository.RoleForRepository(c.Request.Context(), *current(c), repositoryID)
	if !atLeast(role, need) {
		forbidden(c, string(need)+" role required")
		return false
	}
	return true
}

func slugifyRepository(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(strings.TrimSpace(b.String()), "-")
}

func repositoryBoundary(c *gin.Context) {
	c.JSON(http.StatusConflict, errorResponse("repository_boundary", "repository tree changes require pull-request support", nil))
}

type pullRequestRequiredDetails struct {
	RepositoryID string `json:"repository_id,omitempty"`
}

type repositoryDocumentResponse struct {
	Document repo.Document `json:"document"`
	Version  repo.Version  `json:"version"`
}

func respondPullRequestRequired(c *gin.Context, err error) bool {
	if !errors.Is(err, repo.ErrPullRequestRequired) {
		return false
	}
	var required *repo.PullRequestRequiredError
	details := pullRequestRequiredDetails{}
	if errors.As(err, &required) && required.RepositoryID != "" {
		details.RepositoryID = required.RepositoryID
	}
	c.JSON(http.StatusConflict, errorResponse("pull_request_required", "this protected repository requires a pull request", details))
	return true
}

// createRepository godoc
// @Summary Create repository
// @Description Create an empty repository or wrap one owned loose document or root folder in a personal or organization repository.
// @Tags repositories
// @Security bearerAuth
// @Security cookieAuth
// @Param body body object true "Repository settings and seed"
// @Success 201 {object} repo.RepositoryRecord
// @Router /v1/repositories [post]
//
//nolint:gocyclo // Repository creation keeps scope, ownership, and seed-content validation in one transaction boundary.
func (h handlers) createRepository(c *gin.Context) {
	var in struct {
		Name              string `json:"name"`
		Slug              string `json:"slug"`
		Description       string `json:"description"`
		Scope             string `json:"scope"`
		RootFolderID      string `json:"root_folder_id"`
		DocumentID        string `json:"document_id"`
		RequiredApprovals *int   `json:"required_approvals"`
		Protected         *bool  `json:"protected"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid repository")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || (in.RootFolderID != "" && in.DocumentID != "") {
		badRequest(c, "name is required and root_folder_id and document_id cannot both be supplied")
		return
	}
	scope := in.Scope
	if scope == "" {
		scope = repo.RepositoryScopePersonal
	}
	if scope != repo.RepositoryScopePersonal && scope != repo.RepositoryScopeOrganization {
		badRequest(c, "scope must be personal or organization")
		return
	}
	if scope == repo.RepositoryScopeOrganization && h.deps.deployment != DeploymentOrg {
		forbidden(c, "organization repositories require an org deployment")
		return
	}
	slug := slugifyRepository(in.Slug)
	if slug == "" {
		slug = slugifyRepository(in.Name)
	}
	if slug == "" || len(slug) > 100 {
		badRequest(c, "invalid repository slug")
		return
	}
	requiredApprovals := 1
	if in.RequiredApprovals != nil {
		requiredApprovals = *in.RequiredApprovals
	}
	if requiredApprovals < 0 || requiredApprovals > 100 {
		badRequest(c, "required_approvals must be between 0 and 100")
		return
	}
	protected := true
	if in.Protected != nil {
		protected = *in.Protected
	}
	p := *current(c)
	namespaceID, namespaceName := p.ID, p.Name
	grantOrgViewer := false
	if namespaceName == "" {
		namespaceName = p.Email
	}
	if namespaceName == "" {
		namespaceName = p.ID
	}
	if scope == repo.RepositoryScopeOrganization {
		namespaceName = h.deps.orgName
		if namespaceName == "" {
			namespaceName = "Organization"
		}
		namespaceID = slugifyRepository(namespaceName)
		if namespaceID == "" {
			namespaceID = "organization"
		}
		grantOrgViewer = true
	}
	repository, err := h.deps.repository.CreateRepository(c.Request.Context(), p, repo.CreateRepositoryParams{
		Name:              in.Name,
		Slug:              slug,
		Description:       strings.TrimSpace(in.Description),
		Scope:             scope,
		NamespaceID:       namespaceID,
		NamespaceName:     namespaceName,
		RootFolderID:      in.RootFolderID,
		DocumentID:        in.DocumentID,
		RequiredApprovals: requiredApprovals,
		Protected:         protected,
		GrantOrgViewer:    grantOrgViewer,
	})
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrForbidden):
			forbidden(c, "the seed folder or document must be a root-level resource owned by you")
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		case errors.Is(err, repo.ErrConflict):
			c.JSON(http.StatusConflict, errorResponse("conflict", "repository slug or root is already in use", nil))
		default:
			internalErr(c, err)
		}
		return
	}
	c.JSON(http.StatusCreated, repository)
}

const maxRepositoryImportRequestBytes = 64 * 1024 * 1024

func repositoryImportFormValue(values map[string][]string, key string) string {
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func repositoryImportErrorMessage(err error) string {
	message := err.Error()
	for _, prefix := range []string{repo.ErrInvalidRepositoryImport.Error() + ": ", repo.ErrRepositoryImportTooLarge.Error() + ": "} {
		message = strings.TrimPrefix(message, prefix)
	}
	return message
}

// importRepository godoc
// @Summary Create repository from a local folder or document
// @Description Atomically creates a repository, its nested folders, documents, and initial versions from self-contained HTML files.
// @Tags repositories
// @Security bearerAuth
// @Security cookieAuth
// @Accept multipart/form-data
// @Param name formData string true "Repository name"
// @Param slug formData string false "Repository slug"
// @Param description formData string false "Repository description"
// @Param scope formData string false "personal or organization"
// @Param import_kind formData string true "folder or document"
// @Param paths formData []string false "Normalized paths relative to the selected folder, paired with files"
// @Param files formData file true "One or more self-contained .html/.htm files"
// @Success 201 {object} repo.RepositoryImportResult
// @Router /v1/repositories/import [post]
//
//nolint:gocyclo // Multipart limits, archive validation, and atomic import dispatch must remain an ordered pipeline.
func (h handlers) importRepository(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRepositoryImportRequestBytes)
	if err := c.Request.ParseMultipartForm(8 * 1024 * 1024); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "repository import request exceeds 64 MiB", nil))
			return
		}
		badRequest(c, "invalid repository import form")
		return
	}
	form := c.Request.MultipartForm
	if form == nil {
		badRequest(c, "multipart form is required")
		return
	}
	defer func() {
		_ = form.RemoveAll()
	}()

	name := strings.TrimSpace(repositoryImportFormValue(form.Value, "name"))
	kind := strings.TrimSpace(repositoryImportFormValue(form.Value, "import_kind"))
	scope := strings.TrimSpace(repositoryImportFormValue(form.Value, "scope"))
	if scope == "" {
		scope = repo.RepositoryScopePersonal
	}
	if name == "" || (kind != repo.RepositoryImportKindFolder && kind != repo.RepositoryImportKindDocument) {
		badRequest(c, "name and import_kind=folder|document are required")
		return
	}
	if scope != repo.RepositoryScopePersonal && scope != repo.RepositoryScopeOrganization {
		badRequest(c, "scope must be personal or organization")
		return
	}
	if scope == repo.RepositoryScopeOrganization && h.deps.deployment != DeploymentOrg {
		forbidden(c, "organization repositories require an org deployment")
		return
	}

	slug := slugifyRepository(repositoryImportFormValue(form.Value, "slug"))
	if slug == "" {
		slug = slugifyRepository(name)
	}
	if slug == "" || len(slug) > 100 {
		badRequest(c, "invalid repository slug")
		return
	}
	requiredApprovals := 1
	if raw := strings.TrimSpace(repositoryImportFormValue(form.Value, "required_approvals")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 100 {
			badRequest(c, "required_approvals must be between 0 and 100")
			return
		}
		requiredApprovals = value
	}
	protected := true
	if raw := strings.TrimSpace(repositoryImportFormValue(form.Value, "protected")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			badRequest(c, "protected must be true or false")
			return
		}
		protected = value
	}

	fileHeaders := form.File["files"]
	paths := form.Value["paths"]
	if len(fileHeaders) == 0 {
		badRequest(c, "at least one files part is required")
		return
	}
	if len(fileHeaders) > repo.MaxRepositoryImportFiles {
		c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "repository imports are limited to 500 files", nil))
		return
	}
	if kind == repo.RepositoryImportKindFolder && len(paths) != len(fileHeaders) {
		badRequest(c, "folder imports require one paths value for every files part")
		return
	}
	if kind == repo.RepositoryImportKindDocument && len(fileHeaders) != 1 {
		badRequest(c, "document imports require exactly one files part")
		return
	}

	files := make([]repo.RepositoryImportFile, 0, len(fileHeaders))
	totalBytes := 0
	for index, header := range fileHeaders {
		relativePath := header.Filename
		if len(paths) > index && strings.TrimSpace(paths[index]) != "" {
			relativePath = paths[index]
		}
		reader, err := header.Open()
		if err != nil {
			badRequest(c, "could not read uploaded file")
			return
		}
		html, readErr := io.ReadAll(io.LimitReader(reader, repo.MaxRepositoryImportFileBytes+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			badRequest(c, "could not read uploaded file")
			return
		}
		if len(html) > repo.MaxRepositoryImportFileBytes {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "each HTML file is limited to 10 MiB", nil))
			return
		}
		totalBytes += len(html)
		if totalBytes > repo.MaxRepositoryImportTotalBytes {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "repository imports are limited to 50 MiB", nil))
			return
		}
		files = append(files, repo.RepositoryImportFile{Path: relativePath, HTML: html})
	}

	p := *current(c)
	namespaceID, namespaceName := p.ID, p.Name
	if namespaceName == "" {
		namespaceName = p.Email
	}
	if namespaceName == "" {
		namespaceName = p.ID
	}
	grantOrgViewer := false
	if scope == repo.RepositoryScopeOrganization {
		namespaceName = h.deps.orgName
		if namespaceName == "" {
			namespaceName = "Organization"
		}
		namespaceID = slugifyRepository(namespaceName)
		if namespaceID == "" {
			namespaceID = "organization"
		}
		grantOrgViewer = true
	}
	for _, file := range files {
		observeHTML("repository_import", len(file.HTML))
	}
	result, err := h.deps.repository.ImportRepository(c.Request.Context(), p, repo.CreateRepositoryParams{
		Name: name, Slug: slug, Description: strings.TrimSpace(repositoryImportFormValue(form.Value, "description")),
		Scope: scope, NamespaceID: namespaceID, NamespaceName: namespaceName,
		RequiredApprovals: requiredApprovals, Protected: protected, GrantOrgViewer: grantOrgViewer,
	}, kind, files)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrRepositoryImportTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", repositoryImportErrorMessage(err), nil))
		case errors.Is(err, repo.ErrInvalidRepositoryImport):
			c.JSON(http.StatusBadRequest, errorResponse("invalid_repository_import", repositoryImportErrorMessage(err), nil))
		case errors.Is(err, repo.ErrConflict):
			c.JSON(http.StatusConflict, errorResponse("conflict", "repository slug or root is already in use", nil))
		case errors.Is(err, blob.ErrStorage):
			c.JSON(http.StatusBadGateway, errorResponse("blob_storage_failed", "could not upload HTML to blob storage", nil))
		default:
			internalErr(c, err)
		}
		return
	}
	for range result.Documents {
		incDocument("created", actorType(c))
		incVersion("created_initial", actorType(c))
	}
	c.JSON(http.StatusCreated, result)
}

// listRepositories godoc
// @Summary List accessible repositories
// @Tags repositories
// @Security bearerAuth
// @Security cookieAuth
// @Success 200 {object} listResponse[repo.RepositoryRecord]
// @Router /v1/repositories [get]
func (h handlers) listRepositories(c *gin.Context) {
	items, err := h.deps.repository.ListRepositories(c.Request.Context(), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[repo.RepositoryRecord]{Items: items})
}

// getRepository godoc
// @Summary Get repository
// @Tags repositories
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} repo.RepositoryRecord
// @Router /v1/repositories/{id} [get]
func (h handlers) getRepository(c *gin.Context) {
	if !h.needRepositoryRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	repository, err := h.deps.repository.GetRepository(c.Request.Context(), c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	c.JSON(http.StatusOK, repository)
}

// deleteRepository godoc
// @Summary Delete repository
// @Description Permanently delete a repository and every document, folder, version, comment, grant, pin, and pull request inside its root boundary. Only the repository creator may delete it.
// @Tags repositories
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 204
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id} [delete]
func (h handlers) deleteRepository(c *gin.Context) {
	err := h.deps.repository.DeleteRepository(c.Request.Context(), c.Param("id"), *current(c))
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrForbidden):
			forbidden(c, "only the repository owner can delete it")
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		default:
			internalErr(c, err)
		}
		return
	}
	c.Status(http.StatusNoContent)
}

// listRepositoryDocuments godoc
// @Summary List repository documents
// @Tags repositories
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} listResponse[repo.Document]
// @Router /v1/repositories/{id}/documents [get]
func (h handlers) listRepositoryDocuments(c *gin.Context) {
	if !h.needRepositoryRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	items, err := h.deps.repository.ListRepositoryDocuments(c.Request.Context(), *current(c), c.Param("id"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[repo.Document]{Items: items})
}

// createRepositoryDocument godoc
// @Summary Bootstrap an empty repository with its first document
// @Description Creates the only direct initial document allowed in a repository. Once any document exists, further content changes must use pull requests.
// @Tags repositories
// @Security bearerAuth
// @Security cookieAuth
// @Accept multipart/form-data
// @Param id path string true "Repository ID"
// @Param title formData string true "Document title"
// @Param file formData file true "Self-contained HTML file"
// @Success 201 {object} repositoryDocumentResponse
// @Router /v1/repositories/{id}/documents [post]
func (h handlers) createRepositoryDocument(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleEditor) {
		return
	}
	if !h.requireActiveRepository(c, repositoryID) {
		return
	}
	title := strings.TrimSpace(c.PostForm("title"))
	html, err := readMultipartFile(c, "file")
	if errors.Is(err, errPayloadTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "HTML file exceeds 10 MiB", nil))
		return
	}
	if err != nil || title == "" {
		badRequest(c, "title and file are required")
		return
	}
	observeHTML("repository_document_create", len(html))
	document, version, err := h.deps.repository.CreateRepositoryDocument(c.Request.Context(), repositoryID, *current(c), title, html)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrRepositoryNotEmpty):
			c.JSON(http.StatusConflict, errorResponse("repository_not_empty", "only an empty repository can be bootstrapped directly", nil))
		case errors.Is(err, repo.ErrForbidden):
			forbidden(c, "editor role required")
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		case errors.Is(err, blob.ErrStorage):
			c.JSON(http.StatusBadGateway, errorResponse("blob_storage_failed", "could not upload HTML to blob storage", nil))
		default:
			internalErr(c, err)
		}
		return
	}
	incDocument("created", actorType(c))
	incVersion("created_initial", actorType(c))
	c.JSON(http.StatusCreated, repositoryDocumentResponse{Document: document, Version: version})
}

func pullRequestNumber(c *gin.Context) (int64, bool) {
	number, err := strconv.ParseInt(c.Param("number"), 10, 64)
	if err != nil || number < 1 {
		badRequest(c, "invalid pull request number")
		return 0, false
	}
	return number, true
}

// createPullRequest godoc
// @Summary Open pull request
// @Description Propose one immutable HTML version without advancing the published document head.
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Accept multipart/form-data
// @Param id path string true "Repository ID"
// @Param document_id formData string true "Document ID"
// @Param base_version_id formData string true "Current base version ID"
// @Param title formData string true "Pull request title"
// @Param body formData string false "Pull request description"
// @Param change_summary formData string false "Version history summary"
// @Param file formData file true "Self-contained HTML file"
// @Success 201 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests [post]
//
//nolint:gocyclo // Pull-request creation validates every change before committing the change set atomically.
func (h handlers) createPullRequest(c *gin.Context) {
	repositoryID := c.Param("id")
	documentID := strings.TrimSpace(c.PostForm("document_id"))
	repositoryRole, _ := h.deps.repository.RoleForRepository(c.Request.Context(), *current(c), repositoryID)
	authorized := atLeast(repositoryRole, repo.RoleEditor)
	if !authorized && documentID != "" {
		documentRole, _ := h.deps.repository.RoleForDocument(c.Request.Context(), *current(c), documentID)
		if atLeast(documentRole, repo.RoleEditor) {
			if repository, err := h.deps.repository.RepositoryForDocument(c.Request.Context(), documentID); err == nil && repository.ID == repositoryID {
				authorized = true
			}
		}
	}
	if !authorized {
		forbidden(c, "editor role required on the repository or target document")
		return
	}
	if !h.requireActiveRepository(c, repositoryID) {
		return
	}
	html, err := readMultipartFile(c, "file")
	if errors.Is(err, errPayloadTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "HTML file exceeds 10 MiB", nil))
		return
	}
	if err != nil {
		badRequest(c, "file is required")
		return
	}
	baseVersionID := strings.TrimSpace(c.PostForm("base_version_id"))
	title := strings.TrimSpace(c.PostForm("title"))
	if documentID == "" || baseVersionID == "" || title == "" {
		badRequest(c, "document_id, base_version_id, title, and file are required")
		return
	}
	observeHTML("pull_request_create", len(html))
	pullRequest, err := h.deps.repository.CreatePullRequest(c.Request.Context(), repositoryID, documentID, baseVersionID, title, c.PostForm("body"), c.PostForm("change_summary"), html, *current(c))
	if err != nil {
		var conflict *repo.VersionConflictError
		switch {
		case errors.As(err, &conflict):
			c.JSON(http.StatusConflict, errorResponse("version_conflict", "base_version_id is stale", versionConflictDetails{CurrentVersionID: conflict.LatestVersionID}))
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		case errors.Is(err, blob.ErrStorage):
			c.JSON(http.StatusBadGateway, errorResponse("blob_storage_failed", "could not upload HTML to blob storage", nil))
		default:
			internalErr(c, err)
		}
		return
	}
	c.JSON(http.StatusCreated, pullRequest)
}

type pullRequestChangeInput struct {
	Operation     string  `json:"operation"`
	DocumentID    string  `json:"document_id"`
	FolderID      string  `json:"folder_id"`
	BaseVersionID string  `json:"base_version_id"`
	Title         string  `json:"title"`
	Path          string  `json:"path"`
	ContentHTML   *string `json:"content_html"`
}

func parsePullRequestChanges(inputs []pullRequestChangeInput) ([]repo.PullRequestChangeInput, bool) {
	if len(inputs) == 0 {
		return nil, false
	}
	changes := make([]repo.PullRequestChangeInput, 0, len(inputs))
	for _, input := range inputs {
		change := repo.PullRequestChangeInput{
			Operation:     strings.TrimSpace(input.Operation),
			DocumentID:    strings.TrimSpace(input.DocumentID),
			FolderID:      strings.TrimSpace(input.FolderID),
			BaseVersionID: strings.TrimSpace(input.BaseVersionID),
			Title:         strings.TrimSpace(input.Title),
			Path:          strings.TrimSpace(input.Path),
		}
		if input.ContentHTML != nil {
			change.HTML = []byte(*input.ContentHTML)
			if len(change.HTML) > maxHTMLBytes {
				return nil, false
			}
		}
		changes = append(changes, change)
	}
	return changes, true
}

func respondPullRequestChangeError(c *gin.Context, err error) {
	var conflict *repo.VersionConflictError
	switch {
	case errors.As(err, &conflict):
		c.JSON(http.StatusConflict, errorResponse("version_conflict", "a base_version_id is stale", versionConflictDetails{CurrentVersionID: conflict.LatestVersionID}))
	case errors.Is(err, repo.ErrInvalidPullRequestChange):
		badRequest(c, "changes must use update, create, rename, move, or delete and include the fields required by that operation")
	case errors.Is(err, repo.ErrRepositoryBoundary):
		badRequest(c, "every document and folder must belong to this repository")
	case errors.Is(err, repo.ErrConflict):
		c.JSON(http.StatusConflict, errorResponse("conflict", "the proposed change conflicts with the current repository tree", nil))
	case errors.Is(err, repo.ErrNotFound):
		notFound(c)
	case errors.Is(err, blob.ErrStorage):
		c.JSON(http.StatusBadGateway, errorResponse("blob_storage_failed", "could not store proposed HTML", nil))
	case respondPullRequestStateError(c, err):
	default:
		internalErr(c, err)
	}
}

// createPullRequestChangeSet godoc
// @Summary Open a multi-document pull request
// @Description Propose document content and tree changes as one atomic review unit.
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Param id path string true "Repository ID"
// @Param body body object true "Pull request and changes"
// @Success 201 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests/change-set [post]
func (h handlers) createPullRequestChangeSet(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleEditor) {
		return
	}
	if !h.requireActiveRepository(c, repositoryID) {
		return
	}
	var in struct {
		Title         string                   `json:"title"`
		Body          string                   `json:"body"`
		ChangeSummary string                   `json:"change_summary"`
		Changes       []pullRequestChangeInput `json:"changes"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Title) == "" {
		badRequest(c, "title and changes are required")
		return
	}
	changes, ok := parsePullRequestChanges(in.Changes)
	if !ok {
		badRequest(c, "changes are required and each HTML payload must not exceed 10 MiB")
		return
	}
	for _, change := range changes {
		if change.HTML != nil {
			observeHTML("pull_request_change_set", len(change.HTML))
		}
	}
	pullRequest, err := h.deps.repository.CreatePullRequestChangeSet(c.Request.Context(), repositoryID, strings.TrimSpace(in.Title), in.Body, in.ChangeSummary, changes, *current(c))
	if err != nil {
		respondPullRequestChangeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, pullRequest)
}

// createPullRequestPatchset godoc
// @Summary Add a pull-request patchset
// @Description Replace the proposed change set and dismiss approvals from the previous patchset.
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Param body body object true "Replacement changes"
// @Success 200 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests/{number}/patchsets [post]
func (h handlers) createPullRequestPatchset(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleEditor) {
		return
	}
	if !h.requireActiveRepository(c, repositoryID) {
		return
	}
	number, ok := pullRequestNumber(c)
	if !ok {
		return
	}
	var in struct {
		ChangeSummary string                   `json:"change_summary"`
		Changes       []pullRequestChangeInput `json:"changes"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "valid JSON changes are required")
		return
	}
	changes, valid := parsePullRequestChanges(in.Changes)
	if !valid {
		badRequest(c, "changes are required and each HTML payload must not exceed 10 MiB")
		return
	}
	pullRequest, err := h.deps.repository.CreatePullRequestPatchset(c.Request.Context(), repositoryID, number, in.ChangeSummary, changes, *current(c))
	if err != nil {
		respondPullRequestChangeError(c, err)
		return
	}
	c.JSON(http.StatusOK, pullRequest)
}

// rebasePullRequest godoc
// @Summary Rebase a pull request
// @Description Move every base to the current published document heads and create a new patchset.
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Success 200 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests/{number}/rebase [post]
func (h handlers) rebasePullRequest(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleEditor) {
		return
	}
	if !h.requireActiveRepository(c, repositoryID) {
		return
	}
	number, ok := pullRequestNumber(c)
	if !ok {
		return
	}
	pullRequest, err := h.deps.repository.RebasePullRequest(c.Request.Context(), repositoryID, number, *current(c))
	if err != nil {
		respondPullRequestChangeError(c, err)
		return
	}
	c.JSON(http.StatusOK, pullRequest)
}

// listPullRequests godoc
// @Summary List pull requests
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param status query string false "Filter by open, merged, or closed"
// @Success 200 {object} listResponse[repo.PullRequest]
// @Router /v1/repositories/{id}/pull-requests [get]
func (h handlers) listPullRequests(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleViewer) {
		return
	}
	items, err := h.deps.repository.ListPullRequests(c.Request.Context(), repositoryID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && status != repo.PullRequestStatusOpen && status != repo.PullRequestStatusMerged && status != repo.PullRequestStatusClosed {
		badRequest(c, "status must be open, merged, or closed")
		return
	}
	if status != "" {
		filtered := make([]repo.PullRequest, 0, len(items))
		for _, pullRequest := range items {
			if pullRequest.Status == status {
				filtered = append(filtered, pullRequest)
			}
		}
		items = filtered
	}
	c.JSON(http.StatusOK, listResponse[repo.PullRequest]{Items: items})
}

// getPullRequest godoc
// @Summary Get pull request
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Success 200 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests/{number} [get]
func (h handlers) getPullRequest(c *gin.Context) {
	repositoryID := c.Param("id")
	number, ok := pullRequestNumber(c)
	if !ok {
		return
	}
	pullRequest, err := h.deps.repository.GetPullRequest(c.Request.Context(), repositoryID, number)
	if err != nil {
		notFound(c)
		return
	}
	repositoryRole, _ := h.deps.repository.RoleForRepository(c.Request.Context(), *current(c), repositoryID)
	if !atLeast(repositoryRole, repo.RoleViewer) {
		documentID, singleDocument := pullRequestDocumentFallbackID(pullRequest)
		if !singleDocument {
			notFound(c)
			return
		}
		documentRole, _ := h.deps.repository.RoleForDocument(c.Request.Context(), *current(c), documentID)
		if !atLeast(documentRole, repo.RoleViewer) {
			// Deliberately conceal the pull request from callers who can access neither
			// the repository nor every document described by the response.
			notFound(c)
			return
		}
	}
	c.JSON(http.StatusOK, pullRequest)
}

// pullRequestDocumentFallbackID returns the only document a pull request
// describes. Repository-less access is safe only when every change targets
// that same existing document; compatibility fields represent only the first
// change and therefore cannot prove that a multi-document response is visible.
func pullRequestDocumentFallbackID(pullRequest repo.PullRequest) (string, bool) {
	if len(pullRequest.Changes) == 0 {
		return "", false
	}
	documentID := strings.TrimSpace(pullRequest.Changes[0].DocumentID)
	if documentID == "" {
		return "", false
	}
	for _, change := range pullRequest.Changes[1:] {
		if strings.TrimSpace(change.DocumentID) != documentID {
			return "", false
		}
	}
	return documentID, true
}

func respondPullRequestStateError(c *gin.Context, err error) bool {
	code, message := "", ""
	switch {
	case errors.Is(err, repo.ErrApprovalRequired):
		code, message = "approval_required", "required approvals have not been met"
	case errors.Is(err, repo.ErrChangesRequested):
		code, message = "changes_requested", "a reviewer has requested changes"
	case errors.Is(err, repo.ErrStalePullRequest):
		code, message = "stale_pull_request", "the document has changed since this pull request was opened"
	case errors.Is(err, repo.ErrPullRequestClosed):
		code, message = "pull_request_closed", "the pull request is not open"
	default:
		return false
	}
	c.JSON(http.StatusConflict, errorResponse(code, message, nil))
	return true
}

// reviewPullRequest godoc
// @Summary Review pull request
// @Description Approve the current proposed head or request changes. Authors cannot review their own pull requests.
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Param body body object true "Review state and note"
// @Success 200 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests/{number}/reviews [post]
func (h handlers) reviewPullRequest(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleEditor) {
		return
	}
	number, ok := pullRequestNumber(c)
	if !ok {
		return
	}
	var in struct {
		State string `json:"state"`
		Body  string `json:"body"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || (in.State != repo.ReviewStateApproved && in.State != repo.ReviewStateChangesRequested) {
		badRequest(c, "state must be approved or changes_requested")
		return
	}
	pullRequest, err := h.deps.repository.ReviewPullRequest(c.Request.Context(), repositoryID, number, *current(c), in.State, in.Body)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrSelfReview):
			c.JSON(http.StatusForbidden, errorResponse("self_review", "pull request authors cannot review their own changes", nil))
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		case respondPullRequestStateError(c, err):
		default:
			internalErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, pullRequest)
}

// mergePullRequest godoc
// @Summary Merge pull request
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Success 200 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests/{number}/merge [post]
func (h handlers) mergePullRequest(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleEditor) {
		return
	}
	if !h.requireActiveRepository(c, repositoryID) {
		return
	}
	number, ok := pullRequestNumber(c)
	if !ok {
		return
	}
	if !h.requireConfiguredReviewApprovals(c, repositoryID, number) {
		return
	}
	pullRequest, err := h.deps.repository.MergePullRequest(c.Request.Context(), repositoryID, number, *current(c))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
		} else if !respondPullRequestStateError(c, err) {
			internalErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, pullRequest)
}

// patchPullRequest godoc
// @Summary Close pull request
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param number path int true "Pull request number"
// @Param body body object true "Status must be closed"
// @Success 200 {object} repo.PullRequest
// @Router /v1/repositories/{id}/pull-requests/{number} [patch]
func (h handlers) patchPullRequest(c *gin.Context) {
	repositoryID := c.Param("id")
	if !h.needRepositoryRole(c, repositoryID, repo.RoleEditor) {
		return
	}
	number, ok := pullRequestNumber(c)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Status != repo.PullRequestStatusClosed {
		badRequest(c, "status must be closed")
		return
	}
	pullRequest, err := h.deps.repository.ClosePullRequest(c.Request.Context(), repositoryID, number, *current(c))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
		} else if !respondPullRequestStateError(c, err) {
			internalErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, pullRequest)
}
