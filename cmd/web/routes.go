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

	return app.recoverPanic(app.logRequest(app.securityHeaders(mux)))
}
