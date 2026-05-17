package main

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	abbreviations "github.com/zmtcreative/gm-abbreviations"
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
			abbreviations.NewAbbreviations(),
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

func (r *Renderer) Render(markdown []byte) (string, error) {
	var buf bytes.Buffer
	if err := r.md.Convert(markdown, &buf); err != nil {
		return "", err
	}
	safe := r.policy.SanitizeBytes(buf.Bytes())
	return string(safe), nil
}
