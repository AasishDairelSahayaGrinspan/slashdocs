package server

import (
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // Searchers send IANA time zones, and the image has no zoneinfo.
	"unicode"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// Document search follows Slack's search language. Every word must appear in
// the title, "quoted text" must appear as written, and -word excludes titles
// that contain the word. Modifiers filter the results:
//
//	from:<person>    the owner: an email, the name before the @, or me
//	in:<folder>      a folder title or ID, or a folder above it
//	label:<name>     a label; every label is required
//	before:<day>     updated before the day
//	after:<day>      updated after the day
//	on:<day>         updated on the day
//	during:<period>  updated during a day, a month, or a year
//
// Repeated from: and in: modifiers match any of their values. A day is
// YYYY-MM-DD, today, or yesterday. A period is also YYYY, YYYY-MM, or a month
// name. Days start at midnight in the searcher's time zone. As in Slack, a
// modifier without a usable value has no effect, so a partly typed query
// never fails.

const (
	maxSearchQueryBytes = 512
	maxSearchValues     = 10
)

var errInvalidSearch = errors.New("invalid search query")

// searchAliases keeps the earlier author: and labels: names working.
var searchAliases = map[string]string{"author": "from", "labels": "label"}

func parseDocumentSearch(raw string, me auth.Principal, now time.Time) (repo.DocumentSearch, error) {
	if len(raw) > maxSearchQueryBytes {
		return repo.DocumentSearch{}, fmt.Errorf("%w: query is too long", errInvalidSearch)
	}
	var out repo.DocumentSearch
	for _, token := range searchTokens(raw) {
		text := strings.ToLower(token.text)
		if token.phrase {
			out.Terms = append(out.Terms, text)
			continue
		}
		if len(text) > 1 && text[0] == '-' {
			out.ExcludedTerms = append(out.ExcludedTerms, text[1:])
			continue
		}
		key, value, hasKey := strings.Cut(text, ":")
		if alias, ok := searchAliases[key]; ok {
			key = alias
		}
		if !hasKey || !applySearchModifier(&out, key, value, me, now) {
			out.Terms = append(out.Terms, text)
		}
	}
	for _, values := range [][]string{out.Terms, out.ExcludedTerms, out.Owners, out.Folders, out.Labels} {
		if len(values) > maxSearchValues {
			return repo.DocumentSearch{}, fmt.Errorf("%w: use at most %d of each search term", errInvalidSearch, maxSearchValues)
		}
	}
	return out, nil
}

// applySearchModifier reports false when key is not a modifier, so the token
// stays title text. Values are lowercase.
func applySearchModifier(out *repo.DocumentSearch, key, value string, me auth.Principal, now time.Time) bool {
	value = strings.TrimSpace(value)
	switch key {
	case "from":
		value = strings.TrimPrefix(value, "@")
		if value == "me" {
			value = strings.ToLower(me.ID)
		}
		out.Owners = appendNonEmpty(out.Owners, value)
	case "in":
		out.Folders = appendNonEmpty(out.Folders, strings.TrimPrefix(value, "#"))
	case "label":
		for _, label := range strings.Split(value, ",") {
			out.Labels = appendNonEmpty(out.Labels, strings.TrimSpace(label))
		}
	case "before":
		if day, ok := searchDay(value, now); ok {
			narrowUpdated(out, time.Time{}, day)
		}
	case "after":
		if day, ok := searchDay(value, now); ok {
			narrowUpdated(out, day.AddDate(0, 0, 1), time.Time{})
		}
	case "on", "during":
		if start, end, ok := searchPeriod(value, now, key == "during"); ok {
			narrowUpdated(out, start, end)
		}
	default:
		return false
	}
	return true
}

// narrowUpdated limits results to documents updated in [since, before). A
// zero bound leaves that side open. Date modifiers combine with AND.
func narrowUpdated(out *repo.DocumentSearch, since, before time.Time) {
	if since.After(out.UpdatedSince) {
		out.UpdatedSince = since
	}
	if !before.IsZero() && (out.UpdatedBefore.IsZero() || before.Before(out.UpdatedBefore)) {
		out.UpdatedBefore = before
	}
}

func appendNonEmpty(values []string, value string) []string {
	if value == "" {
		return values
	}
	return append(values, value)
}

func searchDay(value string, now time.Time) (time.Time, bool) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch value {
	case "today":
		return today, true
	case "yesterday":
		return today.AddDate(0, 0, -1), true
	}
	day, err := time.ParseInLocation(time.DateOnly, value, now.Location())
	return day, err == nil
}

// searchPeriod returns the half-open range [start, end) of a day. With
// months set, it also accepts YYYY, YYYY-MM, and month names. A month name
// is its most recent occurrence, as in Slack.
func searchPeriod(value string, now time.Time, months bool) (start, end time.Time, ok bool) {
	if day, ok := searchDay(value, now); ok {
		return day, day.AddDate(0, 0, 1), true
	}
	if !months {
		return start, end, false
	}
	if month, err := time.ParseInLocation("2006-01", value, now.Location()); err == nil {
		return month, month.AddDate(0, 1, 0), true
	}
	if year, err := time.ParseInLocation("2006", value, now.Location()); err == nil {
		return year, year.AddDate(1, 0, 0), true
	}
	for m := time.January; m <= time.December; m++ {
		name := strings.ToLower(m.String())
		if value != name && value != name[:3] {
			continue
		}
		year := now.Year()
		if m > now.Month() {
			year--
		}
		start = time.Date(year, m, 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0), true
	}
	return start, end, false
}

type searchToken struct {
	text   string
	phrase bool // The token started with a quote, so it is literal title text.
}

// searchTokens splits on whitespace outside double quotes and removes the
// quotes. A missing closing quote ends at the end of the query, because the
// searcher is often still typing.
func searchTokens(raw string) []searchToken {
	var tokens []searchToken
	var current strings.Builder
	quoted, phrase := false, false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, searchToken{text: current.String(), phrase: phrase})
		}
		current.Reset()
		phrase = false
	}
	for _, r := range raw {
		switch {
		case r == '"':
			phrase = phrase || (!quoted && current.Len() == 0)
			quoted = !quoted
		case quoted || !unicode.IsSpace(r):
			current.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}
