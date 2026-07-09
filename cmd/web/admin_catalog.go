package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jesarx/pirateca/internal/store"
)

// CRUD de autores y editoriales en el dashboard. La creación no tiene
// formulario propio: los autores/editoriales nacen al crear un libro.

func (app *application) dashboardAuthorsHandler(w http.ResponseWriter, r *http.Request) {
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
	data.Pagination = buildPagination(metadata, r.URL.Path, qs)
	app.render(w, r, http.StatusOK, "dashboard-authors.html", data)
}

func (app *application) editAuthorFormHandler(w http.ResponseWriter, r *http.Request) {
	author, ok := app.authorFromPath(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Form = map[string]any{"Author": author, "Errors": map[string]string{}}
	app.render(w, r, http.StatusOK, "author-form.html", data)
}

func (app *application) updateAuthorHandler(w http.ResponseWriter, r *http.Request) {
	author, ok := app.authorFromPath(w, r)
	if !ok {
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	lastName := strings.TrimSpace(r.PostFormValue("last_name"))

	fail := func(msg string) {
		author.Name = name
		author.LastName = lastName
		data := app.newTemplateData(r)
		data.Form = map[string]any{"Author": author, "Errors": map[string]string{"general": msg}}
		app.render(w, r, http.StatusUnprocessableEntity, "author-form.html", data)
	}

	if lastName == "" {
		fail("El apellido es obligatorio.")
		return
	}

	err := app.store.UpdateAuthor(r.Context(), author.ID, name, lastName)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail("Ya existe un autor con ese nombre.")
			return
		}
		app.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/authors", http.StatusSeeOther)
}

func (app *application) deleteAuthorHandler(w http.ResponseWriter, r *http.Request) {
	author, ok := app.authorFromPath(w, r)
	if !ok {
		return
	}

	err := app.store.DeleteAuthor(r.Context(), author.ID)
	if err != nil {
		if errors.Is(err, store.ErrHasBooks) {
			app.badRequest(w, "No se puede borrar: el autor tiene libros asociados.")
			return
		}
		app.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/authors", http.StatusSeeOther)
}

func (app *application) dashboardPublishersHandler(w http.ResponseWriter, r *http.Request) {
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
	data.Pagination = buildPagination(metadata, r.URL.Path, qs)
	app.render(w, r, http.StatusOK, "dashboard-publishers.html", data)
}

func (app *application) editPublisherFormHandler(w http.ResponseWriter, r *http.Request) {
	publisher, ok := app.publisherFromPath(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Form = map[string]any{"Publisher": publisher, "Errors": map[string]string{}}
	app.render(w, r, http.StatusOK, "publisher-form.html", data)
}

func (app *application) updatePublisherHandler(w http.ResponseWriter, r *http.Request) {
	publisher, ok := app.publisherFromPath(w, r)
	if !ok {
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))

	fail := func(msg string) {
		publisher.Name = name
		data := app.newTemplateData(r)
		data.Form = map[string]any{"Publisher": publisher, "Errors": map[string]string{"general": msg}}
		app.render(w, r, http.StatusUnprocessableEntity, "publisher-form.html", data)
	}

	if name == "" {
		fail("El nombre es obligatorio.")
		return
	}

	err := app.store.UpdatePublisher(r.Context(), publisher.ID, name)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail("Ya existe una editorial con ese nombre.")
			return
		}
		app.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/publishers", http.StatusSeeOther)
}

func (app *application) deletePublisherHandler(w http.ResponseWriter, r *http.Request) {
	publisher, ok := app.publisherFromPath(w, r)
	if !ok {
		return
	}

	err := app.store.DeletePublisher(r.Context(), publisher.ID)
	if err != nil {
		if errors.Is(err, store.ErrHasBooks) {
			app.badRequest(w, "No se puede borrar: la editorial tiene libros asociados.")
			return
		}
		app.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/publishers", http.StatusSeeOther)
}

func (app *application) authorFromPath(w http.ResponseWriter, r *http.Request) (*store.Author, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		app.notFound(w)
		return nil, false
	}
	author, err := app.store.GetAuthorByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w)
		} else {
			app.serverError(w, r, err)
		}
		return nil, false
	}
	return author, true
}

func (app *application) publisherFromPath(w http.ResponseWriter, r *http.Request) (*store.Publisher, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		app.notFound(w)
		return nil, false
	}
	publisher, err := app.store.GetPublisherByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w)
		} else {
			app.serverError(w, r, err)
		}
		return nil, false
	}
	return publisher, true
}
