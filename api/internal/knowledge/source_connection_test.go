package knowledge

import (
	"context"
	"testing"
)

func TestMemorySourcePreservesConnectionID(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	want := Source{ID: "src_1", RepositoryID: "repo_1", ConnectionID: "conn_1", Provider: ProviderGitHub}
	if _, err := store.PutSource(ctx, want); err != nil {
		t.Fatalf("put source: %v", err)
	}
	got, err := store.GetSource(ctx, want.RepositoryID, want.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if got.ConnectionID != want.ConnectionID {
		t.Fatalf("connection ID = %q, want %q", got.ConnectionID, want.ConnectionID)
	}
	listed, err := store.ListSources(ctx, want.RepositoryID)
	if err != nil || len(listed) != 1 || listed[0].ConnectionID != want.ConnectionID {
		t.Fatalf("listed sources = %#v, err %v", listed, err)
	}
}
