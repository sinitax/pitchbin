package main

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// abbreviationDef matches *[ABBR]: Full text
var abbreviationDef = regexp.MustCompile(`^\*\[([^\]]+)\]:\s*(.+)$`)

var abbrContextKey = parser.NewContextKey()

// abbrTransformer walks the AST after parsing, finds abbreviation definition
// paragraphs, removes them, and stores the definitions.
type abbrTransformer struct{}

func (t *abbrTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	abbrs := make(map[string]string)
	var toRemove []ast.Node

	ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Kind() != ast.KindParagraph {
			return ast.WalkContinue, nil
		}

		// Check if entire paragraph consists of abbreviation definitions
		lines := node.Lines()
		allDefs := true
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			text := strings.TrimSpace(string(line.Value(reader.Source())))
			if !abbreviationDef.MatchString(text) {
				allDefs = false
				break
			}
		}

		if !allDefs {
			return ast.WalkContinue, nil
		}

		// Extract all definitions
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			text := strings.TrimSpace(string(line.Value(reader.Source())))
			m := abbreviationDef.FindStringSubmatch(text)
			if m != nil {
				abbrs[m[1]] = m[2]
			}
		}
		toRemove = append(toRemove, node)

		return ast.WalkContinue, nil
	})

	for _, node := range toRemove {
		node.Parent().RemoveChild(node.Parent(), node)
	}

	if len(abbrs) > 0 {
		doc.SetAttribute([]byte("abbrs"), abbrs)
	}
}

// abbrRenderer replaces abbreviation occurrences in text nodes with <abbr> tags.
type abbrRenderer struct {
	htmlCfg html.Config
}

func (r *abbrRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindText, r.renderText)
}

func (r *abbrRenderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	textNode := node.(*ast.Text)
	segment := textNode.Segment
	value := segment.Value(source)

	// Find document root and get abbreviations
	var abbrs map[string]string
	n := node.Parent()
	for n != nil {
		if doc, ok := n.(*ast.Document); ok {
			if v, ok := doc.AttributeString("abbrs"); ok {
				abbrs, _ = v.(map[string]string)
			}
			break
		}
		n = n.Parent()
	}

	if len(abbrs) > 0 {
		text := string(value)
		replaced := replaceAbbreviations(text, abbrs)
		w.WriteString(replaced)
	} else {
		w.Write(value)
	}

	// Always handle line breaks
	if textNode.HardLineBreak() || (textNode.SoftLineBreak() && r.htmlCfg.HardWraps) {
		if r.htmlCfg.XHTML {
			w.WriteString("<br />\n")
		} else {
			w.WriteString("<br>\n")
		}
	} else if textNode.SoftLineBreak() {
		w.WriteByte('\n')
	}

	return ast.WalkContinue, nil
}

func replaceAbbreviations(text string, abbrs map[string]string) string {
	for abbr, title := range abbrs {
		escaped := regexp.QuoteMeta(abbr)
		re := regexp.MustCompile(`\b` + escaped + `\b`)
		replacement := `<abbr title="` + strings.ReplaceAll(title, `"`, `&quot;`) + `">` + abbr + `</abbr>`
		text = re.ReplaceAllString(text, replacement)
	}
	return text
}

// Abbreviations is the goldmark extension.
type Abbreviations struct{}

func (a *Abbreviations) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(
		parser.WithASTTransformers(util.Prioritized(&abbrTransformer{}, 100)),
	)
	md.Renderer().AddOptions(
		renderer.WithNodeRenderers(util.Prioritized(&abbrRenderer{}, 100)),
	)
}
