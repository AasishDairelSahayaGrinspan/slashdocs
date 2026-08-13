package markdown

import (
	"strings"
	"testing"
)

func TestToHTMLHeadingsBoldCode(t *testing.T) {
	src := []byte(`# Heading One

Some **bold** and _italic_ and ` + "`code`" + ` text.

## Sub

- item a
- item b

` + "```go\nfmt.Println()\n```" + `
`)
	out, err := ToHTML(src, "Doc Title")
	if err != nil {
		t.Fatalf("ToHTML: %v", err)
	}
	h := string(out)
	for _, want := range []string{
		"<!DOCTYPE html>",
		"<title>Doc Title</title>",
		"<h1>Heading One</h1>",
		"<strong>bold</strong>",
		"<em>italic</em>",
		"<code>code</code>",
		"<li>item a</li>",
		"<h2>Sub</h2>",
		`class="language-go"`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("output missing %q\n---\n%s", want, h)
		}
	}
}

func TestToHTMLTable(t *testing.T) {
	src := []byte(`| A | B |
|---|---|
| 1 | 2 |
`)
	out, err := ToHTML(src, "t")
	if err != nil {
		t.Fatalf("ToHTML: %v", err)
	}
	h := string(out)
	for _, want := range []string{"<table>", "<th>A</th>", "<td>1</td>"} {
		if !strings.Contains(h, want) {
			t.Errorf("table output missing %q\n%s", want, h)
		}
	}
}

func TestToHTMLEscapesRawHTML(t *testing.T) {
	src := []byte(`text <script>alert(1)</script> more`)
	out, err := ToHTML(src, "t")
	if err != nil {
		t.Fatalf("ToHTML: %v", err)
	}
	if strings.Contains(string(out), "<script>alert") {
		t.Errorf("raw HTML was not escaped:\n%s", out)
	}
}

func TestToHTMLEscapesTitle(t *testing.T) {
	out, err := ToHTML([]byte("hi"), `a<b"c`)
	if err != nil {
		t.Fatalf("ToHTML: %v", err)
	}
	if !strings.Contains(string(out), "<title>a&lt;b&#34;c</title>") {
		t.Errorf("title not escaped:\n%s", out)
	}
}

func TestIsMarkdownFile(t *testing.T) {
	for _, p := range []string{"a.md", "B.MD", "x.markdown", "dir/file.md"} {
		if !IsMarkdownFile(p) {
			t.Errorf("IsMarkdownFile(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"a.html", "b.txt", "md", "a.mdx"} {
		if IsMarkdownFile(p) {
			t.Errorf("IsMarkdownFile(%q) = true, want false", p)
		}
	}
}
