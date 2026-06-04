package internal

import (
	"bytes"
	"strings"
	"testing"
)

func runGuidelines(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := guidelinesCmd(&globals{}, &out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("guidelines %v: %v", args, err)
	}
	return out.String()
}

func TestGuidelinesCoreMentionsInlineAndTheme(t *testing.T) {
	got := runGuidelines(t)
	for _, want := range []string{"inline", "data-aidocs-theme", "Categories:"} {
		if !strings.Contains(got, want) {
			t.Errorf("core guidelines missing %q", want)
		}
	}
	if strings.Contains(got, "external stylesheets, fonts, scripts") {
		t.Error("core guidelines still claims external assets work (CSP blocks them)")
	}
}

func TestGuidelinesCategoryPrintsPlaybook(t *testing.T) {
	got := runGuidelines(t, "deck")
	if !strings.Contains(strings.ToLower(got), "slide") {
		t.Errorf("deck playbook did not print; got %d bytes", len(got))
	}
}

func TestGuidelinesUnknownCategoryErrors(t *testing.T) {
	var out bytes.Buffer
	cmd := guidelinesCmd(&globals{}, &out)
	cmd.SetArgs([]string{"nope"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown category")
	}
}
