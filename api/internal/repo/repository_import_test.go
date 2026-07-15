package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
)

func TestValidateRepositoryImportRejectsUnsafeOrAmbiguousFiles(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		files []RepositoryImportFile
	}{
		{name: "traversal", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: "../secret.html", HTML: []byte("<p>secret</p>")}}},
		{name: "absolute", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: "/secret.html", HTML: []byte("<p>secret</p>")}}},
		{name: "backslash", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: `guides\secret.html`, HTML: []byte("<p>secret</p>")}}},
		{name: "non html extension", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: "readme.md", HTML: []byte("<p>secret</p>")}}},
		{name: "binary", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: "secret.html", HTML: []byte{0, 1, 2}}}},
		{name: "case duplicate", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: "Start.html", HTML: []byte("<p>one</p>")}, {Path: "start.HTML", HTML: []byte("<p>two</p>")}}},
		{name: "folder case collision", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: "Guides/one.html", HTML: []byte("<p>one</p>")}, {Path: "guides/two.html", HTML: []byte("<p>two</p>")}}},
		{name: "same title", kind: RepositoryImportKindFolder, files: []RepositoryImportFile{{Path: "start.html", HTML: []byte("<p>one</p>")}, {Path: "start.htm", HTML: []byte("<p>two</p>")}}},
		{name: "multiple loose documents", kind: RepositoryImportKindDocument, files: []RepositoryImportFile{{Path: "one.html", HTML: []byte("<p>one</p>")}, {Path: "two.html", HTML: []byte("<p>two</p>")}}},
		{name: "nested loose document", kind: RepositoryImportKindDocument, files: []RepositoryImportFile{{Path: "folder/one.html", HTML: []byte("<p>one</p>")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ValidateRepositoryImport(test.kind, test.files); !errors.Is(err, ErrInvalidRepositoryImport) {
				t.Fatalf("ValidateRepositoryImport() error = %v, want %v", err, ErrInvalidRepositoryImport)
			}
		})
	}
}

func TestValidateRepositoryImportEnforcesFileAndTotalLimits(t *testing.T) {
	tooMany := make([]RepositoryImportFile, MaxRepositoryImportFiles+1)
	for index := range tooMany {
		tooMany[index] = RepositoryImportFile{Path: "doc-" + string(rune(index+1000)) + ".html", HTML: []byte("<p>x</p>")}
	}
	if _, err := ValidateRepositoryImport(RepositoryImportKindFolder, tooMany); !errors.Is(err, ErrRepositoryImportTooLarge) {
		t.Fatalf("too many files error = %v, want %v", err, ErrRepositoryImportTooLarge)
	}
	if _, err := ValidateRepositoryImport(RepositoryImportKindDocument, []RepositoryImportFile{{Path: "big.html", HTML: make([]byte, MaxRepositoryImportFileBytes+1)}}); !errors.Is(err, ErrRepositoryImportTooLarge) {
		t.Fatalf("oversized file error = %v, want %v", err, ErrRepositoryImportTooLarge)
	}
}

func TestMemoryImportRepositoryCreatesNestedGovernedTreeAtomically(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "import_owner", Email: "owner@example.com", Name: "Owner"}
	files := []RepositoryImportFile{
		{Path: "guides/setup/install.html", HTML: []byte("<html><body>install</body></html>")},
		{Path: "overview.html", HTML: []byte("<html><body>overview</body></html>")},
	}
	result, err := store.ImportRepository(ctx, owner, CreateRepositoryParams{
		Name: "Imported handbook", Slug: "imported-handbook", Scope: RepositoryScopePersonal,
		NamespaceID: owner.ID, NamespaceName: owner.Name, RequiredApprovals: 1, Protected: true,
	}, RepositoryImportKindFolder, files)
	if err != nil {
		t.Fatal(err)
	}
	if result.Repository.RootFolderID == "" || len(result.Documents) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if result.Documents[0].Title != "install" || result.Documents[1].Title != "overview" {
		t.Fatalf("documents = %+v, want deterministic path order", result.Documents)
	}
	if result.Documents[0].ParentFolderID == result.Repository.RootFolderID {
		t.Fatalf("nested document parent = root; want imported nested folders")
	}
	for index, document := range result.Documents {
		version, err := store.GetVersion(ctx, document.CurrentVersionID)
		if err != nil {
			t.Fatal(err)
		}
		html, err := store.GetVersionHTML(ctx, version.ID)
		if err != nil {
			t.Fatal(err)
		}
		if string(html) != string(files[index].HTML) {
			t.Fatalf("document %q HTML = %q, want %q", document.Title, html, files[index].HTML)
		}
		if _, err := store.CreateVersion(ctx, document.ID, document.CurrentVersionID, "bypass", []byte("<p>changed</p>"), owner); !errors.Is(err, ErrPullRequestRequired) {
			t.Fatalf("CreateVersion() error = %v, want %v", err, ErrPullRequestRequired)
		}
	}

	beforeRepositories, _ := store.ListRepositories(ctx, owner)
	beforeDocuments, _ := store.ListDocuments(ctx, owner)
	_, err = store.ImportRepository(ctx, owner, CreateRepositoryParams{
		Name: "Broken import", Slug: "broken-import", Scope: RepositoryScopePersonal,
		NamespaceID: owner.ID, NamespaceName: owner.Name, Protected: true,
	}, RepositoryImportKindFolder, []RepositoryImportFile{
		{Path: "valid.html", HTML: []byte("<p>valid</p>")},
		{Path: "../escape.html", HTML: []byte("<p>escape</p>")},
	})
	if !errors.Is(err, ErrInvalidRepositoryImport) {
		t.Fatalf("invalid import error = %v", err)
	}
	afterRepositories, _ := store.ListRepositories(ctx, owner)
	afterDocuments, _ := store.ListDocuments(ctx, owner)
	if len(afterRepositories) != len(beforeRepositories) || len(afterDocuments) != len(beforeDocuments) {
		t.Fatalf("failed import changed state: repositories %d -> %d, documents %d -> %d", len(beforeRepositories), len(afterRepositories), len(beforeDocuments), len(afterDocuments))
	}
}
