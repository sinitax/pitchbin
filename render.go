package main

import (
	"bytes"

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
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
			),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)

	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("code", "pre", "span", "div")
	policy.AllowAttrs("style").OnElements("span", "pre", "code")

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
