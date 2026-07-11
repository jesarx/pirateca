package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/pirateca/internal/store"
	"github.com/jesarx/pirateca/ui"
)

type templateData struct {
	CurrentYear     int
	CurrentPath     string
	BaseURL         string
	CSSVersion      string
	MetaDescription string
	JSONLD          template.JS
	IsAuthenticated bool
	Heading         string
	Search          string
	Form            any
	Books           []store.Book
	Book            *store.Book
	Authors         []store.Author
	Publishers      []store.Publisher
	Tags            []store.Tag
	Metadata        store.Metadata
	Filters         store.BookFilters
	SortOptions     []sortOption
	Pagination      []pageLink
	Dash            *dashData
	ShowNews        bool
	TagCloud        []chartBar
}

// cssVersion cambia en cada arranque del proceso: rompe el caché de los
// navegadores tras cada deploy sin necesitar hashes de archivos.
var cssVersion = strconv.FormatInt(time.Now().Unix(), 36)

func (app *application) newTemplateData(r *http.Request) templateData {
	return templateData{
		CurrentYear:     time.Now().Year(),
		CurrentPath:     r.URL.Path,
		BaseURL:         app.config.baseURL,
		CSSVersion:      cssVersion,
		IsAuthenticated: app.isAuthenticated(r),
	}
}

// truncate corta un texto a n runas para meta descriptions.
func truncate(s string, n int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= n {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:n-1])) + "…"
}

var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
}

func newTemplateCache() (map[string]*template.Template, error) {
	cache := map[string]*template.Template{}

	pages, err := fs.Glob(ui.Files, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		base := page[len("templates/pages/"):]

		patterns := []string{
			"templates/base.html",
			"templates/partials/*.html",
			page,
		}

		ts, err := template.New(base).Funcs(templateFuncs).ParseFS(ui.Files, patterns...)
		if err != nil {
			return nil, err
		}

		cache[base] = ts
	}

	return cache, nil
}

func (app *application) render(w http.ResponseWriter, r *http.Request, status int, page string, data templateData) {
	ts, ok := app.templates[page]
	if !ok {
		app.serverError(w, r, fmt.Errorf("template %q does not exist", page))
		return
	}

	buf := new(bytes.Buffer)
	if err := ts.ExecuteTemplate(buf, "base", data); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(status)
	buf.WriteTo(w)
}
