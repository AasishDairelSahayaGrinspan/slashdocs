package internal

import (
	"os"
	"strings"
	"testing"
)

// TestSkillMdHasNoFalseClaims keeps the distributed aidocs skill honest: it must
// not repeat the CSP-blocked advice (external stylesheets / Mermaid) and must
// route agents to the authoritative `aidocs guidelines` command.
func TestSkillMdHasNoFalseClaims(t *testing.T) {
	b, err := os.ReadFile("../../skills/aidocs/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Ban the old false-permission phrasings (not corrective mentions like
	// "no Mermaid" / "No external CSS").
	for _, bad := range []string{"stylesheets, fonts", "or Mermaid", "scripts allowed"} {
		if strings.Contains(s, bad) {
			t.Errorf("SKILL.md still implies external assets work: %q (CSP blocks them)", bad)
		}
	}
	for _, want := range []string{"aidocs guidelines", "inline SVG"} {
		if !strings.Contains(s, want) {
			t.Errorf("SKILL.md should mention %q", want)
		}
	}
}
