package page

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var (
	placeholder    = regexp.MustCompile(`\[\[([^\[\]\n]+)\]\]`)
	markdownParser = goldmark.DefaultParser()
)

// subpageLink is a [[Title]] placeholder of a body, from Start to End, with its title cleaned.
type subpageLink struct {
	Start int
	End   int
	Title string
}

// subpageLinks finds the placeholders of body that are outside code and name a title.
func subpageLinks(body string) []subpageLink {
	code := codeRanges(markdownParser.Parse(text.NewReader([]byte(body))), nil)
	var links []subpageLink
	for _, m := range placeholder.FindAllStringSubmatchIndex(body, -1) {
		if slices.ContainsFunc(code, func(r [2]int) bool { return m[0] < r[1] && r[0] < m[1] }) {
			continue
		}
		title, err := CleanTitle(body[m[2]:m[3]])
		if err != nil {
			continue
		}
		links = append(links, subpageLink{Start: m[0], End: m[1], Title: title})
	}
	return links
}

// codeRanges adds the byte ranges of the code spans and code blocks under n to ranges.
func codeRanges(n ast.Node, ranges [][2]int) [][2]int {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			lines := c.Lines()
			for i := range lines.Len() {
				seg := lines.At(i)
				ranges = append(ranges, [2]int{seg.Start, seg.Stop})
			}
		case *ast.CodeSpan:
			for t := c.FirstChild(); t != nil; t = t.NextSibling() {
				if txt, ok := t.(*ast.Text); ok {
					ranges = append(ranges, [2]int{txt.Segment.Start, txt.Segment.Stop})
				}
			}
		default:
			ranges = codeRanges(c, ranges)
		}
	}
	return ranges
}

// uniqueTitles lists the titles of links once each, in the order they first appear.
func uniqueTitles(links []subpageLink) []string {
	var titles []string
	for _, l := range links {
		if !slices.Contains(titles, l.Title) {
			titles = append(titles, l.Title)
		}
	}
	return titles
}

// replaceSubpages turns the placeholders links of body into links to the pages that ids maps their
// titles to.
func replaceSubpages(body string, links []subpageLink, ids map[string]int64) string {
	var b strings.Builder
	last := 0
	for _, l := range links {
		b.WriteString(body[last:l.Start])
		b.WriteString("[" + l.Title + "](" + PageURL(ids[l.Title]) + ")")
		last = l.End
	}
	b.WriteString(body[last:])
	return b.String()
}

// linkSubpages turns the placeholders of body into links to the pages under parent with their
// titles, creating in tx the ones that are missing.
func linkSubpages(ctx context.Context, tx *sql.Tx, parent int64, body string) (string, error) {
	links := subpageLinks(body)
	if len(links) == 0 {
		return body, nil
	}
	titles := uniqueTitles(links)
	ids, err := subpageIDs(ctx, tx, `
		SELECT DISTINCT ON (title) id, title FROM pages
		WHERE parent_id = $1 AND title = ANY($2) AND trashed_at IS NULL
		ORDER BY title, id`, parent, titles)
	if err != nil {
		return "", err
	}
	var missing []string
	for _, title := range titles {
		if _, ok := ids[title]; !ok {
			missing = append(missing, title)
		}
	}
	if len(missing) > 0 {
		created, err := subpageIDs(ctx, tx, `INSERT INTO pages (parent_id, title) SELECT $1, unnest($2::text[]) RETURNING id, title`, parent, missing)
		if err != nil {
			return "", err
		}
		maps.Copy(ids, created)
	}
	return replaceSubpages(body, links, ids), nil
}

// subpageIDs runs query in tx with parent and titles and maps the title of each page it returns to
// its id.
func subpageIDs(ctx context.Context, tx *sql.Tx, query string, parent int64, titles []string) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, query, parent, titles)
	if err != nil {
		return nil, fmt.Errorf("query subpages: %w", err)
	}
	defer rows.Close()
	ids := make(map[string]int64)
	for rows.Next() {
		var (
			id    int64
			title string
		)
		if err := rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("scan subpage: %w", err)
		}
		ids[title] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query subpages: %w", err)
	}
	return ids, nil
}
