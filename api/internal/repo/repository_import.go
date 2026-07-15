package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/razorpay/aidocs/api/internal/auth"
)

const (
	RepositoryImportKindFolder   = "folder"
	RepositoryImportKindDocument = "document"

	MaxRepositoryImportFiles      = 500
	MaxRepositoryImportFileBytes  = 10 * 1024 * 1024
	MaxRepositoryImportTotalBytes = 50 * 1024 * 1024
	// Repository roots count as the first folder level. The rest of aidocs
	// currently permits four nested folders below that root.
	MaxRepositoryImportDirectories = 4
)

var (
	ErrInvalidRepositoryImport  = errors.New("invalid_repository_import")
	ErrRepositoryImportTooLarge = errors.New("repository_import_too_large")
)

// RepositoryImportFile is one self-contained HTML document and its path
// relative to the local folder selected by the user.
type RepositoryImportFile struct {
	Path string
	HTML []byte
}

// RepositoryImportResult returns the complete visible result of the atomic
// import. Documents are ordered by their original relative paths.
type RepositoryImportResult struct {
	Repository RepositoryRecord `json:"repository"`
	Documents  []Document       `json:"documents"`
}

func invalidRepositoryImport(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRepositoryImport, fmt.Sprintf(format, args...))
}

func oversizedRepositoryImport(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRepositoryImportTooLarge, fmt.Sprintf(format, args...))
}

func hasControl(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

// ValidateRepositoryImport normalizes and validates an import at the
// persistence boundary so callers cannot bypass the HTTP upload checks.
//
//nolint:gocyclo // The ordered, side-effect-free validation pipeline preserves deterministic rejection precedence.
func ValidateRepositoryImport(kind string, files []RepositoryImportFile) ([]RepositoryImportFile, error) {
	if kind != RepositoryImportKindFolder && kind != RepositoryImportKindDocument {
		return nil, invalidRepositoryImport("import_kind must be folder or document")
	}
	if len(files) == 0 {
		return nil, invalidRepositoryImport("at least one HTML file is required")
	}
	if kind == RepositoryImportKindDocument && len(files) != 1 {
		return nil, invalidRepositoryImport("document import requires exactly one HTML file")
	}
	if len(files) > MaxRepositoryImportFiles {
		return nil, oversizedRepositoryImport("imports are limited to %d files", MaxRepositoryImportFiles)
	}

	normalized := make([]RepositoryImportFile, 0, len(files))
	seenPaths := make(map[string]string, len(files))
	seenDocuments := make(map[string]string, len(files))
	directoryCasing := map[string]string{}
	total := 0
	for _, input := range files {
		relativePath := strings.TrimSpace(input.Path)
		if relativePath == "" {
			return nil, invalidRepositoryImport("every file requires a relative path")
		}
		if relativePath != input.Path || strings.Contains(relativePath, "\\") || strings.HasPrefix(relativePath, "/") || path.Clean(relativePath) != relativePath {
			return nil, invalidRepositoryImport("path %q is not a normalized relative path", input.Path)
		}
		if kind == RepositoryImportKindDocument && strings.Contains(relativePath, "/") {
			return nil, invalidRepositoryImport("document import path must be a file name")
		}
		if len(relativePath) > 1024 || hasControl(relativePath) {
			return nil, invalidRepositoryImport("path %q is invalid", relativePath)
		}
		segments := strings.Split(relativePath, "/")
		if len(segments)-1 > MaxRepositoryImportDirectories {
			return nil, invalidRepositoryImport("path %q exceeds the %d nested-folder limit", relativePath, MaxRepositoryImportDirectories)
		}
		for _, segment := range segments {
			if segment == "" || segment == "." || segment == ".." || segment != strings.TrimSpace(segment) || len(segment) > 255 || hasControl(segment) {
				return nil, invalidRepositoryImport("path %q contains an invalid segment", relativePath)
			}
		}
		extension := strings.ToLower(path.Ext(relativePath))
		if extension != ".html" && extension != ".htm" {
			return nil, invalidRepositoryImport("path %q is not an HTML file", relativePath)
		}
		title := strings.TrimSuffix(path.Base(relativePath), path.Ext(relativePath))
		if strings.TrimSpace(title) == "" {
			return nil, invalidRepositoryImport("path %q has no document title", relativePath)
		}
		if len(input.HTML) > MaxRepositoryImportFileBytes {
			return nil, oversizedRepositoryImport("file %q exceeds 10 MiB", relativePath)
		}
		total += len(input.HTML)
		if total > MaxRepositoryImportTotalBytes {
			return nil, oversizedRepositoryImport("import exceeds 50 MiB")
		}
		trimmed := bytes.TrimSpace(input.HTML)
		if len(trimmed) == 0 || !utf8.Valid(input.HTML) || bytes.IndexByte(input.HTML, 0) >= 0 || trimmed[0] != '<' {
			return nil, invalidRepositoryImport("file %q is not valid text HTML", relativePath)
		}

		foldedPath := strings.ToLower(relativePath)
		if previous, exists := seenPaths[foldedPath]; exists {
			return nil, invalidRepositoryImport("paths %q and %q collide", previous, relativePath)
		}
		seenPaths[foldedPath] = relativePath

		directory := path.Dir(relativePath)
		if directory == "." {
			directory = ""
		}
		documentKey := strings.ToLower(directory + "/" + title)
		if previous, exists := seenDocuments[documentKey]; exists {
			return nil, invalidRepositoryImport("documents %q and %q have the same title", previous, relativePath)
		}
		seenDocuments[documentKey] = relativePath

		for depth := 1; depth < len(segments); depth++ {
			directoryPath := strings.Join(segments[:depth], "/")
			foldedDirectory := strings.ToLower(directoryPath)
			if previous, exists := directoryCasing[foldedDirectory]; exists && previous != directoryPath {
				return nil, invalidRepositoryImport("folders %q and %q differ only by case", previous, directoryPath)
			}
			directoryCasing[foldedDirectory] = directoryPath
		}

		normalized = append(normalized, RepositoryImportFile{Path: relativePath, HTML: append([]byte(nil), input.HTML...)})
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path })
	return normalized, nil
}

func repositoryImportDirectories(files []RepositoryImportFile) []string {
	directories := map[string]bool{}
	for _, file := range files {
		segments := strings.Split(file.Path, "/")
		for depth := 1; depth < len(segments); depth++ {
			directories[strings.Join(segments[:depth], "/")] = true
		}
	}
	out := make([]string, 0, len(directories))
	for directory := range directories {
		out = append(out, directory)
	}
	sort.Slice(out, func(i, j int) bool {
		leftDepth := strings.Count(out[i], "/")
		rightDepth := strings.Count(out[j], "/")
		if leftDepth == rightDepth {
			return out[i] < out[j]
		}
		return leftDepth < rightDepth
	})
	return out
}

func repositoryImportTitle(relativePath string) string {
	base := path.Base(relativePath)
	return strings.TrimSuffix(base, path.Ext(base))
}

func repositoryImportSHA(html []byte) string {
	sum := sha256.Sum256(html)
	return hex.EncodeToString(sum[:])
}

// ImportRepository creates a new repository, its folder tree, every document,
// and every initial version while holding one memory-store lock. Validation and
// conflicts happen before any state is changed.
//
//nolint:gocyclo // The in-memory import validates and builds the complete tree before atomically publishing it.
func (m *Memory) ImportRepository(_ context.Context, creator auth.Principal, params CreateRepositoryParams, kind string, inputs []RepositoryImportFile) (RepositoryImportResult, error) {
	files, err := ValidateRepositoryImport(kind, inputs)
	if err != nil {
		return RepositoryImportResult{}, err
	}
	if params.Name == "" || params.Slug == "" || (params.Scope != RepositoryScopePersonal && params.Scope != RepositoryScopeOrganization) || params.RootFolderID != "" || params.DocumentID != "" {
		return RepositoryImportResult{}, invalidRepositoryImport("invalid repository settings")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.repos {
		if existing.Scope == params.Scope && existing.NamespaceID == params.NamespaceID && strings.EqualFold(existing.Slug, params.Slug) {
			return RepositoryImportResult{}, ErrConflict
		}
	}
	if m.folderConflictLocked(creator, "", params.Name, "") {
		return RepositoryImportResult{}, ErrConflict
	}

	nextFolderID := func() string {
		for {
			m.folderN++
			id := fmt.Sprintf("fld_%d", m.folderN)
			if _, exists := m.folders[id]; !exists {
				return id
			}
		}
	}
	rootID := nextFolderID()
	directoryIDs := map[string]string{"": rootID}
	for _, directory := range repositoryImportDirectories(files) {
		directoryIDs[directory] = nextFolderID()
	}

	for {
		m.repoN++
		if _, exists := m.repos[fmt.Sprintf("repo_%d", m.repoN)]; !exists {
			break
		}
	}
	repositoryID := fmt.Sprintf("repo_%d", m.repoN)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	repository := RepositoryRecord{
		ID: repositoryID, Name: params.Name, Slug: params.Slug, Description: params.Description,
		Scope: params.Scope, NamespaceID: params.NamespaceID, NamespaceName: params.NamespaceName,
		RootFolderID: rootID, CreatedBy: creator, RequiredApprovals: params.RequiredApprovals,
		Protected: params.Protected, CreatedAt: now, UpdatedAt: now,
	}

	documents := make([]Document, 0, len(files))
	versions := make([]Version, 0, len(files))
	for _, file := range files {
		documentID := m.nextDocumentIDLocked()
		versionID := m.nextVersionIDLocked()
		directory := path.Dir(file.Path)
		if directory == "." {
			directory = ""
		}
		documents = append(documents, Document{
			ID: documentID, Title: repositoryImportTitle(file.Path), Owner: creator,
			CurrentVersionID: versionID, ParentFolderID: directoryIDs[directory], UpdatedAt: now,
		})
		versions = append(versions, Version{
			ID: versionID, Number: 1, DocumentID: documentID, CreatedBy: creator,
			SHA256: repositoryImportSHA(file.HTML),
		})
	}

	m.folders[rootID] = Folder{ID: rootID, Title: params.Name, Owner: creator}
	for _, directory := range repositoryImportDirectories(files) {
		parent := path.Dir(directory)
		if parent == "." {
			parent = ""
		}
		m.folders[directoryIDs[directory]] = Folder{
			ID: directoryIDs[directory], Title: path.Base(directory), Owner: creator,
			ParentFolderID: directoryIDs[parent],
		}
	}
	m.repos[repository.ID] = repository
	for index, document := range documents {
		m.docs[document.ID] = document
		m.versions[versions[index].ID] = versions[index]
		m.versionHTML[versions[index].ID] = append([]byte(nil), files[index].HTML...)
	}
	if params.GrantOrgViewer {
		m.grantN++
		m.grants = append(m.grants, Grant{ID: fmt.Sprintf("gr_%d", m.grantN), DocumentID: rootID, ResourceType: "folder", ResourceID: rootID, Principal: auth.Principal{Type: auth.PrincipalAnyone}, Role: RoleViewer, GrantedBy: creator})
	}
	return RepositoryImportResult{Repository: repository, Documents: documents}, nil
}
