package server

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMCPDisplayClientNameBoundsAndNeutralizesSelfReportedNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"keeps an ordinary name", "Claude Desktop", "Claude Desktop"},
		{"drops bidi overrides and isolates", "Claude‮Desktop⁩", "ClaudeDesktop"},
		{"drops zero-width joiners", "Clau​de", "Claude"},
		{"folds controls and line breaks into spaces", "Claude Desk\r\ntop", "Claude Desk top"},
		{"trims and collapses whitespace", "  Claude\t\tDesktop  ", "Claude Desktop"},
		{"falls back when nothing renderable is left", "‮", defaultMCPClientName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpDisplayClientName(tc.in); got != tc.want {
				t.Fatalf("mcpDisplayClientName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// Registration caps the name in bytes; the consent page caps what it
	// renders in runes, so a multi-byte name cannot outgrow that bound either.
	t.Run("caps the rendered length", func(t *testing.T) {
		got := mcpDisplayClientName(strings.Repeat("é", maxMCPDisplayNameRunes+10))
		if want := strings.Repeat("é", maxMCPDisplayNameRunes) + "…"; got != want {
			t.Fatalf("mcpDisplayClientName truncated to %q, want %q", got, want)
		}
		if count := utf8.RuneCountInString(got); count != maxMCPDisplayNameRunes+1 {
			t.Fatalf("rendered name is %d runes, want %d plus an ellipsis", count, maxMCPDisplayNameRunes)
		}
	})
}
