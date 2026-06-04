package internal

import (
	"os"
	"strings"
	"testing"
)

// TestPublishGateWiredIn keeps the humanization step from silently disappearing:
// the core contract and the distributed skill must both point authors at
// `aidocs guidelines writing-voice` before they publish.
func TestPublishGateWiredIn(t *testing.T) {
	core, err := readGuideline("core.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(core, "writing-voice") {
		t.Error("core.md should route authors to `aidocs guidelines writing-voice`")
	}

	skill, err := os.ReadFile("../../skills/aidocs/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), "writing-voice") {
		t.Error("SKILL.md should reference the writing-voice humanization step")
	}
}
