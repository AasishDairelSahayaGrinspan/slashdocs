// Package markdown converts Markdown files into single-file HTML documents
// suitable for upload to aidocs. Conversion is CLI-side: the server only ever
// stores HTML, so .md/.markdown inputs are rendered here before upload.
package markdown

import (
	"bytes"
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// md is the shared converter. Unsafe() is intentionally NOT enabled: raw HTML
// embedded in markdown is escaped, keeping uploaded documents free of surprise
// scripts. Tables, strikethrough, and autolinks cover the deterministic syntax
// most documents use.
var md = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify),
	goldmark.WithRendererOptions(renderer.WithNodeRenderers()),
	goldmark.WithRendererOptions(gmhtml.WithXHTML()),
)

// pageCSS is a minimal readable-prose baseline. The aidocs reader applies its
// own theme variables on top; these rules just keep a converted document
// legible without fighting the reader.
const pageCSS = `body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;line-height:1.6;max-width:52rem;margin:2rem auto;padding:0 1rem;color:#1f2328;background:#fff}
h1,h2,h3,h4{line-height:1.25}h1{border-bottom:1px solid #d1d9e0;padding-bottom:.3rem}
pre,code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}pre{background:#f6f8fa;padding:1rem;overflow:auto;border-radius:6px}code{background:#eff1f3;padding:.15em .35em;border-radius:4px}pre code{background:none;padding:0}
blockquote{margin:0;padding:0 1rem;color:#59636e;border-left:.25rem solid #d1d9e0}
table{border-collapse:collapse}th,td{border:1px solid #d1d9e0;padding:.4rem .8rem}
img{max-width:100%}a{color:#0969da}`

// ToHTML renders markdown as a complete, self-contained HTML5 document.
// title becomes the <title> and an <h1> is NOT injected (the markdown's own
// headings carry the structure).
func ToHTML(src []byte, title string) ([]byte, error) {
	var body bytes.Buffer
	if err := md.Convert(src, &body); err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</title>\n<style>")
	b.WriteString(pageCSS)
	b.WriteString("</style>\n</head>\n<body>\n")
	b.Write(body.Bytes())
	b.WriteString("\n</body>\n</html>\n")
	return []byte(b.String()), nil
}

// IsMarkdownFile reports whether the path looks like a markdown file.
func IsMarkdownFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}
