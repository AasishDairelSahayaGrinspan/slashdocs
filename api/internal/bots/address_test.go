package bots

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateLabel(t *testing.T) {
	valid := []string{"a", "n8n", "n8n-prod", "a1-b2-c3", strings.Repeat("a", 31)}
	for _, label := range valid {
		if err := ValidateLabel(label); err != nil {
			t.Errorf("ValidateLabel(%q) = %v, want nil", label, err)
		}
	}
	invalid := []string{"", "-lead", "trail-", "UPPER", "has space", "dot.ted", "a_b", strings.Repeat("a", 32)}
	for _, label := range invalid {
		if err := ValidateLabel(label); !errors.Is(err, ErrInvalidLabel) {
			t.Errorf("ValidateLabel(%q) = %v, want ErrInvalidLabel", label, err)
		}
	}
}

func TestValidateDomain(t *testing.T) {
	valid := []string{"brave.otter.bot", "a.bot", "a-1.b-2.bot", "one.two.three.bot"}
	for _, d := range valid {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("ValidateDomain(%q) = %v, want nil", d, err)
		}
	}
	invalid := []string{
		"",
		".bot",                             // nothing before the TLD
		"brave.otter.com",                  // wrong TLD
		"bot",                              // no TLD separator
		"Brave.otter.bot",                  // uppercase
		"-brave.otter.bot",                 // leading hyphen
		"brave-.otter.bot",                 // trailing hyphen in a label
		"brave..otter.bot",                 // empty label
		strings.Repeat("a", 60) + ".b.bot", // 66 chars, over the 63 limit
	}
	for _, d := range invalid {
		if err := ValidateDomain(d); !errors.Is(err, ErrInvalidDomain) {
			t.Errorf("ValidateDomain(%q) = %v, want ErrInvalidDomain", d, err)
		}
	}
}

func TestSplitAndCompose(t *testing.T) {
	label, domain, ok := Split("n8n-prod@brave.otter.bot")
	if !ok || label != "n8n-prod" || domain != "brave.otter.bot" {
		t.Fatalf("Split = (%q, %q, %v)", label, domain, ok)
	}
	if got := Compose(label, domain); got != "n8n-prod@brave.otter.bot" {
		t.Fatalf("Compose = %q", got)
	}
	for _, s := range []string{"", "no-at", "@domain.bot", "label@", "a@b@c"} {
		if _, _, ok := Split(s); ok {
			t.Errorf("Split(%q) ok, want failure", s)
		}
	}
}

func TestIsBotAddress(t *testing.T) {
	if !IsBotAddress("n8n-prod@brave.otter.bot") {
		t.Error("valid bot address rejected")
	}
	for _, s := range []string{"user@example.com", "n8n-prod@brave.otter.com", "UPPER@brave.otter.bot", "n8n prod@brave.otter.bot", "@brave.otter.bot"} {
		if IsBotAddress(s) {
			t.Errorf("IsBotAddress(%q) = true, want false", s)
		}
	}
}

// TestGeneratedDomainsValidate proves every word-list combination survives
// ValidateDomain, so retry loops around GenerateDomain never spin on
// self-inflicted invalid output.
func TestGeneratedDomainsValidate(t *testing.T) {
	for _, w := range append(append([]string{}, adjectives...), nouns...) {
		if !labelRE.MatchString(w) {
			t.Errorf("word list entry %q is not a valid label", w)
		}
	}
	var longestAdj, longestNoun string
	for _, w := range adjectives {
		if len(w) > len(longestAdj) {
			longestAdj = w
		}
	}
	for _, w := range nouns {
		if len(w) > len(longestNoun) {
			longestNoun = w
		}
	}
	worst := longestAdj + "." + longestNoun + "." + longestAdj + tld
	if err := ValidateDomain(worst); err != nil {
		t.Fatalf("worst-case extended domain %q invalid: %v", worst, err)
	}
	for range 32 {
		if err := ValidateDomain(GenerateDomain()); err != nil {
			t.Fatal(err)
		}
		if err := ValidateDomain(GenerateDomainExtended()); err != nil {
			t.Fatal(err)
		}
	}
}
