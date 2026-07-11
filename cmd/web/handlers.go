package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	app.renderBookList(w, r, "", bookFiltersFromQuery(r.URL.Query()))
}

func (app *application) staticPage(page string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app.render(w, r, http.StatusOK, page, app.newTemplateData(r))
	}
}

func (app *application) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	app.notFound(w, r)
}

// authorBooksHandler y publisherBooksHandler reutilizan el listado de
// libros fijando el filtro desde el path (/authors/{slug}), que es como
// funcionaba el sitio viejo con ?authslug=.
func (app *application) authorBooksHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	author, err := app.store.GetAuthorBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	filters := bookFiltersFromQuery(r.URL.Query())
	filters.AuthorSlug = author.Slug
	app.renderBookList(w, r, "Libros de "+author.FullName(), filters)
}

func (app *application) publisherBooksHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	publisher, err := app.store.GetPublisherBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	filters := bookFiltersFromQuery(r.URL.Query())
	filters.PublisherSlug = publisher.Slug
	app.renderBookList(w, r, "Libros de "+publisher.Name, filters)
}

func (app *application) renderBookList(w http.ResponseWriter, r *http.Request, heading string, filters store.BookFilters) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	books, metadata, err := app.store.ListBooks(r.Context(), filters)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Heading = heading
	data.Books = books
	data.Metadata = metadata
	data.Filters = filters
	data.SortOptions = bookSortOptions(filters.Sort)
	data.Pagination = buildPagination(metadata, r.URL.Path, r.URL.Query())
	if heading != "" {
		data.MetaDescription = truncate(heading+" — descarga libre en PDF o torrent en Pirateca.", 158)
	}
	// El anuncio solo aparece en la portada del catálogo, sin filtros.
	data.ShowNews = r.URL.Path == "/books" && filters.Search == "" &&
		len(filters.Tags) == 0 && filters.AuthorSlug == "" &&
		filters.PublisherSlug == "" && filters.Page <= 1

	app.render(w, r, http.StatusOK, "books.html", data)
}

func (app *application) bookDetailHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	book, err := app.store.GetBookBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	data := app.newTemplateData(r)
	data.Book = book
	if book.Description != "" {
		data.MetaDescription = truncate(book.Description, 158)
	} else {
		data.MetaDescription = truncate(fmt.Sprintf("«%s» de %s (%s, %d). Descarga libre en PDF o torrent.",
			book.Title, book.AuthorFullName(), book.PublisherName, book.Year), 158)
	}
	data.JSONLD = bookJSONLD(app.config.baseURL, book)
	app.render(w, r, http.StatusOK, "book.html", data)
}

func (app *application) authorsHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	qs := r.URL.Query()
	filters := store.AuthorFilters{
		Search: strings.TrimSpace(qs.Get("name")),
		Sort:   qs.Get("sort"),
		Page:   queryInt(qs, "page"),
	}

	authors, metadata, err := app.store.ListAuthors(r.Context(), filters)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Authors = authors
	data.Metadata = metadata
	data.Search = filters.Search
	data.SortOptions = authorSortOptions(filters.Sort)
	data.Pagination = buildPagination(metadata, r.URL.Path, qs)

	app.render(w, r, http.StatusOK, "authors.html", data)
}

func (app *application) publishersHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	qs := r.URL.Query()
	filters := store.PublisherFilters{
		Search: strings.TrimSpace(qs.Get("name")),
		Sort:   qs.Get("sort"),
		Page:   queryInt(qs, "page"),
	}

	publishers, metadata, err := app.store.ListPublishers(r.Context(), filters)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Publishers = publishers
	data.Metadata = metadata
	data.Search = filters.Search
	data.SortOptions = publisherSortOptions(filters.Sort)
	data.Pagination = buildPagination(metadata, r.URL.Path, qs)

	app.render(w, r, http.StatusOK, "publishers.html", data)
}

func (app *application) tagsHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	tags, err := app.store.ListTags(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Tags = tags
	data.TagCloud = buildTagCloud(tags)
	app.render(w, r, http.StatusOK, "tags.html", data)
}

func bookFiltersFromQuery(qs url.Values) (f store.BookFilters) {
	f.Search = strings.TrimSpace(qs.Get("title"))
	f.AuthorSlug = strings.TrimSpace(qs.Get("authslug"))
	f.PublisherSlug = strings.TrimSpace(qs.Get("pubslug"))
	f.Sort = qs.Get("sort")
	if tags := strings.TrimSpace(qs.Get("tags")); tags != "" {
		f.Tags = strings.Split(tags, ",")
	}
	f.Page = queryInt(qs, "page")
	return f
}

func queryInt(qs url.Values, key string) int {
	n, err := strconv.Atoi(qs.Get(key))
	if err != nil {
		return 0
	}
	return n
}
