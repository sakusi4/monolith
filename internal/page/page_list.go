package page

import (
	"net/url"
	"strings"

	"github.com/sakusi4/monolith/web"
)

const maxResults = 100

// pageQuery is the state of the Pages list in the URL: the search text, empty for the pages at the
// top, and the kind of pages shown.
type pageQuery struct {
	Q    string
	Kind kind
}

func parsePageQuery(v url.Values) (pageQuery, error) {
	k, err := web.ParseChoice("kind", v.Get("kind"), kinds, kindAll)
	if err != nil {
		return pageQuery{}, err
	}
	return pageQuery{Q: strings.TrimSpace(v.Get("q")), Kind: k}, nil
}

// pick keeps the entries of q's kind, only the first 100 of them for a search.
func (q pageQuery) pick(entries []Entry) []Entry {
	var picked []Entry
	for _, e := range entries {
		if q.Kind == kindAll || e.Kind == q.Kind {
			picked = append(picked, e)
		}
	}
	if q.Q != "" {
		picked = picked[:min(len(picked), maxResults)]
	}
	return picked
}

func (q pageQuery) filters() web.FilterBar {
	return web.FilterBar{
		Action:  pagesURL,
		Search:  web.Search{Name: "q", Label: "Search", Value: q.Q},
		Filters: []web.Filter{{Name: "kind", Label: "Show", Options: web.Options(kinds, kind.Label, q.Kind)}},
	}
}
