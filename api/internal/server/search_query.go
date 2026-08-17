package server

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/razorpay/aidocs/api/internal/repo"
)

const (
	maxSearchQueryBytes = 512
	maxSearchValueBytes = 128
	maxSearchLabels     = 10
)

var errInvalidSearch = errors.New("invalid search query")

// parseDocumentSearch turns the user-facing search language into validated
// regex patterns consumed by the repository. Literal values are escaped and
// therefore remain substring searches. /pattern/ values opt into the small
// regex surface supported by both Go's RE2 validator and PostgreSQL ARE.
func parseDocumentSearch(raw string) (repo.DocumentSearch, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxSearchQueryBytes {
		return repo.DocumentSearch{}, fmt.Errorf("%w: query is too long", errInvalidSearch)
	}
	tokens, err := searchTokens(raw)
	if err != nil {
		return repo.DocumentSearch{}, err
	}

	var out repo.DocumentSearch
	text := make([]string, 0, len(tokens))
	for _, token := range tokens {
		key, value, structured := strings.Cut(token, ":")
		key = strings.ToLower(key)
		if !structured || (key != "author" && key != "labels" && key != "in") {
			text = append(text, token)
			continue
		}
		if value == "" {
			return repo.DocumentSearch{}, fmt.Errorf("%w: %s: needs a value", errInvalidSearch, key)
		}
		if err := applySearchModifier(&out, key, value); err != nil {
			return repo.DocumentSearch{}, err
		}
	}
	if len(text) > 0 {
		out.TitlePattern, err = searchPattern(strings.Join(text, " "))
		if err != nil {
			return repo.DocumentSearch{}, fmt.Errorf("title: %w", err)
		}
	}
	return out, nil
}

func applySearchModifier(out *repo.DocumentSearch, key, value string) error {
	switch key {
	case "author":
		if out.AuthorPattern != "" {
			return fmt.Errorf("%w: author: may only be used once", errInvalidSearch)
		}
		pattern, err := searchModifierPattern(value)
		if err != nil {
			return fmt.Errorf("author: %w", err)
		}
		out.AuthorPattern = pattern
	case "in":
		if out.FolderPattern != "" {
			return fmt.Errorf("%w: in: may only be used once", errInvalidSearch)
		}
		pattern, err := searchModifierPattern(value)
		if err != nil {
			return fmt.Errorf("in: %w", err)
		}
		out.FolderPattern = pattern
	case "labels":
		values, err := splitSearchLabels(value)
		if err != nil {
			return err
		}
		if len(out.LabelPatterns)+len(values) > maxSearchLabels {
			return fmt.Errorf("%w: at most %d labels are allowed", errInvalidSearch, maxSearchLabels)
		}
		for _, label := range values {
			pattern, err := searchModifierPattern(label)
			if err != nil {
				return fmt.Errorf("labels: %w", err)
			}
			out.LabelPatterns = append(out.LabelPatterns, pattern)
		}
	}
	return nil
}

// searchTokens is a deliberately small shell-like lexer. Quotes preserve
// whitespace and backslash escapes the next character.
func searchTokens(raw string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range raw {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		return nil, fmt.Errorf("%w: trailing escape", errInvalidSearch)
	}
	if quote != 0 {
		return nil, fmt.Errorf("%w: unterminated quote", errInvalidSearch)
	}
	flush()
	return tokens, nil
}

func splitSearchLabels(raw string) ([]string, error) {
	var values []string
	var current strings.Builder
	inRegex := false
	escaped := false
	flush := func() error {
		value := strings.TrimSpace(current.String())
		current.Reset()
		if value == "" {
			return fmt.Errorf("%w: labels: contains an empty label", errInvalidSearch)
		}
		values = append(values, value)
		return nil
	}
	for _, r := range raw {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if r == '/' {
			inRegex = !inRegex
			current.WriteRune(r)
			continue
		}
		if r == ',' && !inRegex {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		current.WriteRune(r)
	}
	if inRegex {
		return nil, fmt.Errorf("%w: labels: has an unterminated regex", errInvalidSearch)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return values, nil
}

func searchPattern(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: empty value", errInvalidSearch)
	}
	if len(value) > maxSearchValueBytes {
		return "", fmt.Errorf("%w: value is too long", errInvalidSearch)
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		if len(value) < 2 || !strings.HasPrefix(value, "/") || !strings.HasSuffix(value, "/") {
			return "", fmt.Errorf("%w: regex must be wrapped in /.../", errInvalidSearch)
		}
		value = value[1 : len(value)-1]
		if value == "" {
			return "", fmt.Errorf("%w: regex cannot be empty", errInvalidSearch)
		}
		if strings.Contains(value, "(?") {
			return "", fmt.Errorf("%w: regex extensions are not supported", errInvalidSearch)
		}
		if _, err := regexp.Compile(value); err != nil {
			return "", fmt.Errorf("%w: invalid regex: %v", errInvalidSearch, err)
		}
		return value, nil
	}
	return escapePostgresRegex(value), nil
}

func searchModifierPattern(value string) (string, error) {
	isRegex := strings.HasPrefix(strings.TrimSpace(value), "/")
	pattern, err := searchPattern(value)
	if err != nil || isRegex {
		return pattern, err
	}
	return "^" + pattern + "$", nil
}

func escapePostgresRegex(value string) string {
	const metacharacters = `\\.^$*+?()[]{}|`
	var out strings.Builder
	for _, r := range value {
		if strings.ContainsRune(metacharacters, r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}
