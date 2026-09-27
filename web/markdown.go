package web

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const maxHeadingLevel = 6

var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(headingShift{}, 100))),
)

// headingShift moves every heading one level down, with h6 as the lowest.
type headingShift struct{}

func (headingShift) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	shiftHeadings(doc)
}

func shiftHeadings(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if h, ok := c.(*ast.Heading); ok {
			h.Level = min(h.Level+1, maxHeadingLevel)
		}
		shiftHeadings(c)
	}
}

// renderMarkdown renders GFM source as HTML. It leaves out raw HTML and the targets of links with
// unsafe schemes such as javascript:.
func renderMarkdown(source string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := markdownParser.Convert([]byte(source), &buf); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return template.HTML(buf.String()), nil
}
