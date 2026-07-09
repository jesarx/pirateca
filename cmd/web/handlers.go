package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jesarx/pirateca/internal/store"
)

func (app *application) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok"))
}

func (app *application) homeHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/books", http.StatusMovedPermanently)
}

func (app *application) booksHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	qs := r.URL.Query()
	filters := bookFiltersFromQuery(qs)

	books, metadata, err := app.store.ListBooks(r.Context(), filters)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Books = books
	data.Metadata = metadata
	data.Filters = filters
	data.SortOptions = bookSortOptions(filters.Sort)
	data.Pagination = buildPagination(metadata, qs)

	app.render(w, r, http.StatusOK, "books.html", data)
}

func bookFiltersFromQuery(qs map[string][]string) (f store.BookFilters) {
	get := func(key string) string {
		if v, ok := qs[key]; ok && len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}

	f.Search = get("title")
	f.AuthorSlug = get("authslug")
	f.PublisherSlug = get("pubslug")
	f.Sort = get("sort")
	if tags := get("tags"); tags != "" {
		f.Tags = strings.Split(tags, ",")
	}
	if page, err := strconv.Atoi(get("page")); err == nil {
		f.Page = page
	}
	return f
}
