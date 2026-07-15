package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/db"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

type recordingImportBlobStore struct {
	inner blob.Store
	mu    sync.Mutex
	keys  map[string]bool
}

func newRecordingImportBlobStore() *recordingImportBlobStore {
	return &recordingImportBlobStore{inner: blob.NewMemory(), keys: map[string]bool{}}
}

func (s *recordingImportBlobStore) Put(ctx context.Context, key, contentType string, body []byte) error {
	if err := s.inner.Put(ctx, key, contentType, body); err != nil {
		return err
	}
	s.mu.Lock()
	s.keys[key] = true
	s.mu.Unlock()
	return nil
}

func (s *recordingImportBlobStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	return s.inner.Get(ctx, key)
}

func (s *recordingImportBlobStore) Delete(ctx context.Context, key string) error {
	if err := s.inner.Delete(ctx, key); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.keys, key)
	s.mu.Unlock()
	return nil
}

func (s *recordingImportBlobStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.keys)
}

func TestPostgresImportRepositoryIsAtomicAndPreservesFolderTree(t *testing.T) {
	databaseURL := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	blobs := newRecordingImportBlobStore()
	store, err := pgrepo.ConnectWithBlob(ctx, databaseURL, blobs)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_import_" + suffix, Email: "import-" + suffix + "@example.com", Name: "Import Owner"}
	params := repo.CreateRepositoryParams{
		Name: "Imported repository " + suffix, Slug: "imported-repository-" + suffix,
		Scope: repo.RepositoryScopePersonal, NamespaceID: owner.ID, NamespaceName: owner.Name,
		RequiredApprovals: 1, Protected: true,
	}
	files := []repo.RepositoryImportFile{
		{Path: "guides/setup.html", HTML: []byte("<html><body>setup</body></html>")},
		{Path: "overview.html", HTML: []byte("<html><body>overview</body></html>")},
	}
	result, err := store.ImportRepository(ctx, owner, params, repo.RepositoryImportKindFolder, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Documents) != 2 || blobs.count() != 2 {
		t.Fatalf("import result documents=%d blobs=%d, want 2/2", len(result.Documents), blobs.count())
	}
	if result.Documents[0].ParentFolderID == result.Repository.RootFolderID || result.Documents[1].ParentFolderID != result.Repository.RootFolderID {
		t.Fatalf("folder structure was not preserved: %+v", result.Documents)
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
	}

	// This writes a candidate blob before discovering the unique repository
	// slug conflict. The transaction rollback and blob compensation must leave
	// both durable stores exactly as they were before the failed import.
	_, err = store.ImportRepository(ctx, owner, params, repo.RepositoryImportKindDocument, []repo.RepositoryImportFile{{Path: "conflict.html", HTML: []byte("<p>conflict</p>")}})
	if !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("conflicting import error = %v, want %v", err, repo.ErrConflict)
	}
	if blobs.count() != 2 {
		t.Fatalf("conflicting import left %d blobs, want the original 2", blobs.count())
	}
	documents, err := store.ListRepositoryDocuments(ctx, owner, result.Repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 2 {
		t.Fatalf("conflicting import changed repository documents to %d, want 2", len(documents))
	}
}
