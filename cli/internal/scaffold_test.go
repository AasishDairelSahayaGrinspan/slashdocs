package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemeStyleBlockExtracted(t *testing.T) {
	css, err := themeStyleBlock()
	if err != nil {
		t.Fatal(err)
	}
	trimmed := strings.TrimSpace(css)
	if !strings.HasPrefix(trimmed, "<style>") || !strings.HasSuffix(trimmed, "</style>") {
		t.Errorf("expected a <style>…</style> block, got %.40q…", css)
	}
	for _, want := range []string{"--rzp-primary", `data-aidocs-theme="dark"`} {
		if !strings.Contains(css, want) {
			t.Errorf("theme style block missing %q", want)
		}
	}
}

func TestAssembleScaffoldIsValidAndCSPSafe(t *testing.T) {
	for _, typ := range scaffoldTypes() {
		html, err := assembleScaffold(typ, "My Title")
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		for _, want := range []string{"<!doctype html>", "<style>", "--rzp-primary", "data-aidocs-theme", "My Title"} {
			if !strings.Contains(html, want) {
				t.Errorf("%s scaffold missing %q", typ, want)
			}
		}
		for _, bad := range []string{`rel="stylesheet"`, "<script src", "@import", "@font-face", "googleapis", "mermaid"} {
			if strings.Contains(strings.ToLower(html), bad) {
				t.Errorf("%s scaffold contains CSP-forbidden %q", typ, bad)
			}
		}
	}
}

func TestUnknownScaffoldTypeErrors(t *testing.T) {
	if _, err := assembleScaffold("nope", "t"); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestDocsNewSurfacedInContract(t *testing.T) {
	core, err := readGuideline("core.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(core, "docs new") {
		t.Error("core.md should mention `aidocs docs new` as the fastest correct start")
	}
	skill, err := os.ReadFile("../../skills/aidocs/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), "docs new") {
		t.Error("SKILL.md should mention `aidocs docs new`")
	}
}

func TestDocsNewWritesFileAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.html")
	var out bytes.Buffer

	cmd := docsNewCmd(&globals{}, &out)
	cmd.SetArgs([]string{path, "--type", "report", "--title", "Q3 RCA"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("docs new: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "Q3 RCA") {
		t.Fatalf("file not written with title: %v", err)
	}

	cmd2 := docsNewCmd(&globals{}, &out)
	cmd2.SetArgs([]string{path, "--type", "report"})
	if err := cmd2.Execute(); err == nil {
		t.Fatal("expected refusal to overwrite without --force")
	}
}
