package main

import (
	"bytes"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

type Renderer struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
}

func NewRenderer() *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Typographer,
			extension.NewFootnote(),
			extension.DefinitionList,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
			),
			&Abbreviations{},
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)

	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("code", "pre", "span", "div")
	policy.AllowStyles("color", "background-color", "font-weight", "font-style", "text-decoration").OnElements("span", "pre", "code")
	// Footnotes
	policy.AllowAttrs("id", "class", "href").OnElements("a")
	policy.AllowAttrs("id", "class").OnElements("section", "sup", "li", "ol")
	// Definition lists
	policy.AllowElements("dl", "dt", "dd")
	// Abbreviations
	policy.AllowAttrs("title").OnElements("abbr")

	return &Renderer{md: md, policy: policy}
}

// StripFrontmatter removes YAML frontmatter (--- delimited) from markdown,
// returning the body without it.
func StripFrontmatter(markdown []byte) []byte {
	s := string(markdown)
	if !strings.HasPrefix(s, "---\n") {
		return markdown
	}
	lines := strings.SplitAfter(s[4:], "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r\n")
		if trimmed == "---" {
			return []byte(strings.Join(lines[i+1:], ""))
		}
	}
	return markdown
}

func (r *Renderer) Render(markdown []byte, title string) (string, error) {
	body := StripFrontmatter(markdown)
	var buf bytes.Buffer
	if err := r.md.Convert(body, &buf); err != nil {
		return "", err
	}
	safe := r.policy.SanitizeBytes(buf.Bytes())
	if title != "" {
		return stripFirstH1(string(safe), title), nil
	}
	return string(safe), nil
}

// stripFirstH1 removes the leading <h1> if its text matches the page title,
// since the title is already displayed in the page header.
func stripFirstH1(html string, title string) string {
	start := strings.Index(html, "<h1")
	if start < 0 || strings.TrimSpace(html[:start]) != "" {
		return html
	}
	end := strings.Index(html[start:], "</h1>")
	if end < 0 {
		return html
	}
	// Extract text content between <h1...> and </h1>
	tagClose := strings.Index(html[start:], ">")
	if tagClose < 0 {
		return html
	}
	h1Text := strings.TrimSpace(html[start+tagClose+1 : start+end])
	if h1Text != title {
		return html
	}
	rest := html[start+end+len("</h1>"):]
	return strings.TrimLeft(rest, "\n")
}
