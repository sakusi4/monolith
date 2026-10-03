package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/sakusi4/monolith/internal/money"
)

//go:embed templates/*.html static
var files embed.FS

var pages = parsePages()

func parsePages() map[string]*template.Template {
	funcs := template.FuncMap{"money": money.Format, "current": currentFunc("")}
	names, err := fs.Glob(files, "templates/*.html")
	if err != nil {
		panic(err)
	}
	pages := make(map[string]*template.Template)
	for _, name := range names {
		base := path.Base(name)
		if base == "layout.html" || strings.HasPrefix(base, "_") {
			continue
		}
		t := template.New("layout.html").Funcs(funcs)
		pages[strings.TrimSuffix(base, ".html")] = template.Must(t.ParseFS(files, "templates/layout.html", "templates/_*.html", name))
	}
	return pages
}

func Render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	base, ok := pages[page]
	if !ok {
		ServerError(w, r, fmt.Errorf("unknown page %q", page))
		return
	}
	t, err := base.Clone()
	if err != nil {
		ServerError(w, r, fmt.Errorf("clone %s: %w", page, err))
		return
	}
	t.Funcs(template.FuncMap{"current": currentFunc(r.URL.Path)})
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		ServerError(w, r, fmt.Errorf("render %s: %w", page, err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		slog.WarnContext(r.Context(), "write response", slog.Any("error", err))
	}
}

// currentFunc backs the "current" template function, which reports whether the request
// path is under one of the given prefixes, to mark the navigation link of the current page.
func currentFunc(path string) func(prefixes ...string) bool {
	return func(prefixes ...string) bool {
		return slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(path, prefix) })
	}
}

func ServerError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Any("error", err),
	)
	http.Error(w, "Something went wrong. Please try again later.", http.StatusInternalServerError)
}

func Static() http.Handler {
	static, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServerFS(static))
}
