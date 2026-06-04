package internal

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// hslLuminance returns the WCAG relative luminance of an hsl() color
// (h in degrees, s and l in percent).
func hslLuminance(h, s, l float64) float64 {
	s /= 100
	l /= 100
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	lin := func(v float64) float64 {
		v += m
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrast(a, b [3]float64) float64 {
	la := hslLuminance(a[0], a[1], a[2])
	lb := hslLuminance(b[0], b[1], b[2])
	if lb > la {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

var tokenRe = regexp.MustCompile(`--rzp-([a-z-]+):hsl\(\s*([\d.]+)\s+([\d.]+)%\s+([\d.]+)%`)

// parseTokens extracts every --rzp-* hsl token within a CSS block.
func parseTokens(block string) map[string][3]float64 {
	m := map[string][3]float64{}
	for _, mt := range tokenRe.FindAllStringSubmatch(block, -1) {
		h, _ := strconv.ParseFloat(mt[2], 64)
		s, _ := strconv.ParseFloat(mt[3], 64)
		l, _ := strconv.ParseFloat(mt[4], 64)
		m[mt[1]] = [3]float64{h, s, l}
	}
	return m
}

// themeBlocks returns the light and dark token maps parsed from the shipped
// razorpay-theme.md, so the AA check tracks the real theme and cannot drift.
func themeBlocks(t *testing.T) (light, dark map[string][3]float64) {
	t.Helper()
	body, err := readGuideline("razorpay-theme.md")
	if err != nil {
		t.Fatalf("read theme: %v", err)
	}
	const darkSel = `:root[data-aidocs-theme="dark"]`
	i := strings.Index(body, darkSel)
	if i < 0 {
		t.Fatal("theme has no dark block")
	}
	return parseTokens(body[:i]), parseTokens(body[i:])
}

func TestThemeMeetsAA(t *testing.T) {
	light, dark := themeBlocks(t)
	const aaNormal = 4.5
	cases := []struct {
		name        string
		set         map[string][3]float64
		fg, bg      string
	}{
		{"light ink on bg", light, "ink", "bg"},
		{"light ink-soft on bg", light, "ink-soft", "bg"},
		{"light primary text on surface", light, "primary", "surface"},
		{"dark ink on bg", dark, "ink", "bg"},
		{"dark primary text on surface", dark, "primary", "surface"},
	}
	for _, c := range cases {
		fg, okf := c.set[c.fg]
		bg, okb := c.set[c.bg]
		if !okf || !okb {
			t.Fatalf("%s: missing token (%s=%v, %s=%v)", c.name, c.fg, okf, c.bg, okb)
		}
		if got := contrast(fg, bg); got < aaNormal {
			t.Errorf("%s: contrast %.2f < %.1f (AA)", c.name, got, aaNormal)
		}
	}
}

func TestFeedbackCalloutsMeetAA(t *testing.T) {
	light, dark := themeBlocks(t)
	const aaNormal = 4.5
	for _, set := range []map[string][3]float64{light, dark} {
		for _, intent := range []string{"positive", "negative", "notice", "information"} {
			fg, okf := set[intent]
			bg, okb := set[intent+"-subtle"]
			if !okf || !okb {
				t.Fatalf("missing %s tokens", intent)
			}
			if got := contrast(fg, bg); got < aaNormal {
				t.Errorf("%s on %s-subtle: contrast %.2f < %.1f", intent, intent, got, aaNormal)
			}
		}
	}
}
