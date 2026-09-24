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
	"strings"

	"github.com/sakusi4/monolith/internal/money"
)

//go:embed templates static
var files embed.FS

var pages = parsePages()

func parsePages() map[string]*template.Template {
	funcs := template.FuncMap{"usd": money.FormatUSD}
	names, err := fs.Glob(files, "templates/*.html")
	if err != nil {
		panic(err)
	}
	pages := make(map[string]*template.Template)
	for _, name := range names {
		if name == "templates/layout.html" {
			continue
		}
		t := template.New("layout.html").Funcs(funcs)
		pages[strings.TrimSuffix(path.Base(name), ".html")] = template.Must(t.ParseFS(files, "templates/layout.html", name))
	}
	return pages
}

func Render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	t, ok := pages[page]
	if !ok {
		ServerError(w, r, fmt.Errorf("unknown page %q", page))
		return
	}
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
