package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCLIRefreshesBundledSkillWhenBinaryIsCurrent(t *testing.T) {
	oldVersion := Version
	Version = "commit-sha"
	t.Cleanup(func() { Version = oldVersion })

	home := t.TempDir()
	t.Setenv("HOME", home)

	var skillHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			_ = json.NewEncoder(w).Encode(serverVersion{Version: "commit-sha"})
		case "/skills/aidocs/SKILL.md":
			skillHits++
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = w.Write([]byte("---\nname: aidocs\ndescription: test skill\n---\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	v, changed, skillRefreshed, err := updateCLI(context.Background(), &Client{Base: srv.URL, HTTP: srv.Client()}, false)
	if err != nil {
		t.Fatalf("updateCLI returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false for already-current binary")
	}
	if !skillRefreshed {
		t.Fatalf("skillRefreshed = false, want true")
	}
	if v.Version != "commit-sha" {
		t.Fatalf("server version = %q", v.Version)
	}
	if skillHits != 1 {
		t.Fatalf("skill endpoint hits = %d, want 1", skillHits)
	}
	for _, p := range []string{
		filepath.Join(home, ".agents", "skills", "aidocs", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "aidocs", "SKILL.md"),
	} {
		b := readTestFile(t, p)
		if string(b) != "---\nname: aidocs\ndescription: test skill\n---\n" {
			t.Fatalf("%s content = %q", p, string(b))
		}
	}
}

func TestUpdateCLIRefreshesBundledSkillFromDevServer(t *testing.T) {
	oldVersion := Version
	Version = "local-build"
	t.Cleanup(func() { Version = oldVersion })

	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			_ = json.NewEncoder(w).Encode(serverVersion{Version: "dev"})
		case "/skills/aidocs/SKILL.md":
			_, _ = w.Write([]byte("---\nname: aidocs\ndescription: dev skill\n---\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	v, changed, skillRefreshed, err := updateCLI(context.Background(), &Client{Base: srv.URL, HTTP: srv.Client()}, false)
	if err == nil || !strings.Contains(err.Error(), "no CLI asset") {
		t.Fatalf("updateCLI error = %v, want missing asset error", err)
	}
	if v.Version != "dev" {
		t.Fatalf("server version = %q", v.Version)
	}
	if !skillRefreshed {
		t.Fatalf("skillRefreshed = false, want true")
	}
	if changed {
		t.Fatalf("changed = true, want false because dev server published no binary asset")
	}
	b := readTestFile(t, filepath.Join(home, ".agents", "skills", "aidocs", "SKILL.md"))
	if string(b) != "---\nname: aidocs\ndescription: dev skill\n---\n" {
		t.Fatalf("skill content = %q", string(b))
	}
}
