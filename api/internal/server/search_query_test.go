package server

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/repo"
)

func TestParseDocumentSearch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want repo.DocumentSearch
	}{
		{
			name: "plain title",
			raw:  "payments incident",
			want: repo.DocumentSearch{TitlePattern: "payments incident"},
		},
		{
			name: "all primitives",
			raw:  `migration author:alice@example.com labels:api,review in:"Product Specs"`,
			want: repo.DocumentSearch{TitlePattern: "migration", AuthorPattern: `^alice@example\.com$`, LabelPatterns: []string{"^api$", "^review$"}, FolderPattern: "^Product Specs$"},
		},
		{
			name: "primitive regex",
			raw:  `author:/@razorpay\.com$/ labels:/^api-/,review in:/payments|checkout/`,
			want: repo.DocumentSearch{AuthorPattern: `@razorpay\.com$`, LabelPatterns: []string{"^api-", "^review$"}, FolderPattern: `payments|checkout`},
		},
		{
			name: "unknown prefix stays title text",
			raw:  "status:draft plan",
			want: repo.DocumentSearch{TitlePattern: "status:draft plan"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDocumentSearch(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseDocumentSearch(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseDocumentSearchRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`author:`,
		`in:"unterminated`,
		`labels:api,,review`,
		`labels:/unterminated`,
		`author:/(look)(?=ahead)/`,
		`author:/[/`,
		strings.Repeat("x", maxSearchQueryBytes+1),
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			_, err := parseDocumentSearch(raw)
			if !errors.Is(err, errInvalidSearch) {
				t.Fatalf("parseDocumentSearch(%q) error = %v, want errInvalidSearch", raw, err)
			}
		})
	}
}

func TestParseDocumentTitleSearchTreatsTagsAndRegexAsLiteralText(t *testing.T) {
	t.Parallel()
	got, err := parseDocumentTitleSearch(`author:alice@example.com /incident/`)
	if err != nil {
		t.Fatal(err)
	}
	want := repo.DocumentSearch{TitlePattern: `author:alice@example\.com /incident/`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDocumentTitleSearch() = %#v, want %#v", got, want)
	}
}
