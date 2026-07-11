package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/jesarx/pirateca/internal/store"
)

// bookJSONLD arma el structured data schema.org/Book del detalle de un
// libro. Se serializa en Go (no en la plantilla) para que el escape JSON
// sea siempre correcto.
func bookJSONLD(baseURL string, b *store.Book) template.JS {
	ld := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Book",
		"name":     b.Title,
		"url":      baseURL + "/books/" + b.Slug,
		"author": map[string]any{
			"@type": "Person",
			"name":  b.AuthorFullName(),
		},
		"publisher": map[string]any{
			"@type": "Organization",
			"name":  b.PublisherName,
		},
		"inLanguage": "es",
	}
	if b.ISBN != "" {
		ld["isbn"] = b.ISBN
	}
	if b.Year > 0 {
		ld["datePublished"] = fmt.Sprintf("%d", b.Year)
	}
	if b.Pages > 0 {
		ld["numberOfPages"] = b.Pages
	}
	if b.Filename != "" {
		ld["image"] = baseURL + "/images?file=" + b.Filename + ".jpg"
	}

	out, err := json.Marshal(ld)
	if err != nil {
		return ""
	}
	return template.JS(out)
}

func (app *application) robotsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprintf(w, `User-agent: *
Allow: /
Disallow: /dashboard
Disallow: /admin

Sitemap: %s/sitemap.xml
`, app.config.baseURL)
}

func (app *application) sitemapHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	entries, err := app.store.GetSitemapEntries(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	fmt.Fprint(w, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`+"\n")

	writeURL := func(path string, lastmod time.Time) {
		fmt.Fprintf(w, "  <url><loc>%s%s</loc>", app.config.baseURL, path)
		if !lastmod.IsZero() {
			fmt.Fprintf(w, "<lastmod>%s</lastmod>", lastmod.Format("2006-01-02"))
		}
		fmt.Fprint(w, "</url>\n")
	}

	for _, p := range []string{"/books", "/authors", "/publishers", "/tags", "/manifest", "/contact", "/news/mal-que-dura-cien-anos"} {
		writeURL(p, time.Time{})
	}
	for _, e := range entries {
		writeURL(e.Path, e.LastMod)
	}

	fmt.Fprint(w, "</urlset>\n")
}
