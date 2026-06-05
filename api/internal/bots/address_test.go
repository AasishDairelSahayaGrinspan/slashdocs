package bots

import (
	"errors"
	"strings"
	"testing"
)

func TestAddressValidationAndSplit(t *testing.T) {
	validLabels := []string{"a", "bot-1", "abc123"}
	for _, label := range validLabels {
		if err := ValidateLabel(label); err != nil {
			t.Fatalf("ValidateLabel(%q) = %v", label, err)
		}
	}
	invalidLabels := []string{"", "-bot", "bot-", "UPPER", strings.Repeat("a", 32)}
	for _, label := range invalidLabels {
		if err := ValidateLabel(label); !errors.Is(err, ErrInvalidLabel) {
			t.Fatalf("ValidateLabel(%q) = %v, want ErrInvalidLabel", label, err)
		}
	}

	validDomains := []string{"ops.bot", "team.ops.bot", "a-b.c1.bot"}
	for _, domain := range validDomains {
		if err := ValidateDomain(domain); err != nil {
			t.Fatalf("ValidateDomain(%q) = %v", domain, err)
		}
	}
	invalidDomains := []string{"bot", ".bot", "ops.com", "-ops.bot", "ops-.bot", strings.Repeat("a", 64) + ".bot"}
	for _, domain := range invalidDomains {
		if err := ValidateDomain(domain); !errors.Is(err, ErrInvalidDomain) {
			t.Fatalf("ValidateDomain(%q) = %v, want ErrInvalidDomain", domain, err)
		}
	}

	label, domain, ok := Split("release@ops.bot")
	if !ok || label != "release" || domain != "ops.bot" {
		t.Fatalf("Split = %q %q %v, want release ops.bot true", label, domain, ok)
	}
	for _, address := range []string{"missing-at", "a@b@c", "@ops.bot", "bot@"} {
		if _, _, ok := Split(address); ok {
			t.Fatalf("Split(%q) ok=true, want false", address)
		}
	}
}

func TestBotAddressGeneration(t *testing.T) {
	address := Compose("release", "ops.bot")
	if address != "release@ops.bot" || !IsBotAddress(address) {
		t.Fatalf("composed address %q was not valid", address)
	}
	for _, address := range []string{"release@ops.com", "Release@ops.bot", "release"} {
		if IsBotAddress(address) {
			t.Fatalf("IsBotAddress(%q) = true, want false", address)
		}
	}

	for _, domain := range []string{GenerateDomain(), GenerateDomainExtended()} {
		if err := ValidateDomain(domain); err != nil {
			t.Fatalf("generated domain %q invalid: %v", domain, err)
		}
	}
}
