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

	return app.recoverPanic(app.logRequest(app.securityHeaders(mux)))
}
