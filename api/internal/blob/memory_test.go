package blob

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestMemoryBlobStoreCopiesAndDeletesObjects(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	if err := store.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	body := []byte("hello")
	if err := store.Put(ctx, "doc.html", "text/html", body); err != nil {
		t.Fatal(err)
	}
	body[0] = 'H'

	got, ct, err := store.Get(ctx, "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	if ct != "text/html" || !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("Get = %q %q, want hello text/html", got, ct)
	}
	got[0] = 'H'
	again, _, err := store.Get(ctx, "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, []byte("hello")) {
		t.Fatalf("Get returned mutable backing slice: %q", again)
	}

	if err := store.Delete(ctx, "doc.html"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(ctx, "doc.html"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}
