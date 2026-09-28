package web

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const (
	maxHeadingLevel       = 6
	strikethroughPriority = 500
)

var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.Linkify, extension.Table, extension.TaskList),
	goldmark.WithParserOptions(
		parser.WithInlineParsers(util.Prioritized(doubleTilde{extension.NewStrikethroughParser()}, strikethroughPriority)),
		parser.WithASTTransformers(util.Prioritized(headingShift{}, 100)),
	),
	goldmark.WithRendererOptions(
		renderer.WithNodeRenderers(util.Prioritized(extension.NewStrikethroughHTMLRenderer(), strikethroughPriority)),
	),
)

// doubleTilde lets only a run of two tildes open or close a strikethrough, so 4~5 stays text.
type doubleTilde struct {
	strikethrough parser.InlineParser
}

func (d doubleTilde) Trigger() []byte {
	return d.strikethrough.Trigger()
}

func (d doubleTilde) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if !bytes.HasPrefix(line, []byte("~~")) {
		return nil
	}
	return d.strikethrough.Parse(parent, block, pc)
}

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

// renderMarkdown renders GFM source as HTML, except that only ~~ makes a strikethrough. It leaves out
// raw HTML and the targets of links with unsafe schemes such as javascript:.
func renderMarkdown(source string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := markdownParser.Convert([]byte(source), &buf); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return template.HTML(buf.String()), nil
}
