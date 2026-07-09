package main

import (
	"net/http"

	"github.com/jesarx/pirateca/ui"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.FileServerFS(ui.Files))

	mux.HandleFunc("GET /health", app.healthHandler)
	mux.HandleFunc("GET /{$}", app.homeHandler)
	mux.HandleFunc("GET /books", app.booksHandler)
	mux.HandleFunc("GET /books/{slug}", app.bookDetailHandler)
	mux.HandleFunc("GET /authors", app.authorsHandler)
	mux.HandleFunc("GET /authors/{slug}", app.authorBooksHandler)
	mux.HandleFunc("GET /publishers", app.publishersHandler)
	mux.HandleFunc("GET /publishers/{slug}", app.publisherBooksHandler)
	mux.HandleFunc("GET /tags", app.tagsHandler)

	mux.HandleFunc("GET /images", app.serveCover)
	mux.HandleFunc("GET /pdfs", app.servePdf)
	mux.HandleFunc("GET /epubs", app.serveEpub)
	mux.HandleFunc("GET /torrs", app.serveTorrent)

	mux.HandleFunc("GET /admin/login", app.loginFormHandler)
	mux.HandleFunc("POST /admin/login", app.loginHandler)
	mux.HandleFunc("POST /admin/logout", app.logoutHandler)

	mux.HandleFunc("GET /dashboard", app.requireAuth(app.dashboardBooksHandler))
	mux.HandleFunc("GET /dashboard/books/new", app.requireAuth(app.newBookFormHandler))
	mux.HandleFunc("POST /dashboard/books/new", app.requireAuth(app.createBookHandler))
	mux.HandleFunc("GET /dashboard/books/{id}/edit", app.requireAuth(app.editBookFormHandler))
	mux.HandleFunc("POST /dashboard/books/{id}/edit", app.requireAuth(app.updateBookHandler))
	mux.HandleFunc("POST /dashboard/books/{id}/delete", app.requireAuth(app.deleteBookHandler))

	mux.HandleFunc("GET /dashboard/authors", app.requireAuth(app.dashboardAuthorsHandler))
	mux.HandleFunc("GET /dashboard/authors/{id}/edit", app.requireAuth(app.editAuthorFormHandler))
	mux.HandleFunc("POST /dashboard/authors/{id}/edit", app.requireAuth(app.updateAuthorHandler))
	mux.HandleFunc("POST /dashboard/authors/{id}/delete", app.requireAuth(app.deleteAuthorHandler))

	mux.HandleFunc("GET /dashboard/publishers", app.requireAuth(app.dashboardPublishersHandler))
	mux.HandleFunc("GET /dashboard/publishers/{id}/edit", app.requireAuth(app.editPublisherFormHandler))
	mux.HandleFunc("POST /dashboard/publishers/{id}/edit", app.requireAuth(app.updatePublisherHandler))
	mux.HandleFunc("POST /dashboard/publishers/{id}/delete", app.requireAuth(app.deletePublisherHandler))

	return app.recoverPanic(app.logRequest(app.securityHeaders(app.checkOrigin(mux))))
}
