package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsUpdateRefreshesFromServer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	skill := "---\nname: aidocs\ndescription: refreshed skill\n---\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/skills/aidocs/SKILL.md" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = io.WriteString(w, skill)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AIDOCS_SERVER", srv.URL)

	got, err := Execute([]string{"skills", "update"})
	if err != nil {
		t.Fatalf("skills update returned error: %v", err)
	}
	if !strings.Contains(got, "Updated aidocs skill") || !strings.Contains(got, srv.URL) {
		t.Fatalf("unexpected output: %q", got)
	}
	for _, p := range []string{
		filepath.Join(home, ".agents", "skills", "aidocs", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "aidocs", "SKILL.md"),
	} {
		b := readTestFile(t, p)
		if string(b) != skill {
			t.Fatalf("%s content = %q", p, string(b))
		}
	}
}
