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

	skill := "---\nname: slashdocs\ndescription: refreshed skill\n---\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/skills/slashdocs/SKILL.md" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = io.WriteString(w, skill)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SLASHDOCS_SERVER", srv.URL)

	got, err := Execute([]string{"skills", "update"})
	if err != nil {
		t.Fatalf("skills update returned error: %v", err)
	}
	if !strings.Contains(got, "Updated slashdocs skill") || !strings.Contains(got, srv.URL) {
		t.Fatalf("unexpected output: %q", got)
	}
	for _, p := range []string{
		filepath.Join(home, ".agents", "skills", "slashdocs", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "slashdocs", "SKILL.md"),
	} {
		b := readTestFile(t, p)
		if string(b) != skill {
			t.Fatalf("%s content = %q", p, string(b))
		}
	}
}
