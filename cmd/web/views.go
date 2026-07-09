package main

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/jesarx/pirateca/internal/store"
)

var errNoDatabase = errors.New("no database configured (set -db-dsn or PIRATECA_DB_DSN)")

type sortOption struct {
	Value    string
	Label    string
	Selected bool
}

func bookSortOptions(current string) []sortOption {
	opts := []sortOption{
		{Value: "-created_at", Label: "Fecha de agregado (reciente a antiguo)"},
		{Value: "created_at", Label: "Fecha de agregado (antiguo a reciente)"},
		{Value: "title", Label: "Título (A-Z)"},
		{Value: "-title", Label: "Título (Z-A)"},
		{Value: "year", Label: "Año de publicación (antiguo a reciente)"},
		{Value: "-year", Label: "Año de publicación (reciente a antiguo)"},
		{Value: "random", Label: "Aleatorio"},
	}
	for i := range opts {
		opts[i].Selected = opts[i].Value == current
	}
	return opts
}

type pageLink struct {
	Label    string
	URL      string
	Current  bool
	Ellipsis bool
}

// buildPagination arma los enlaces de paginación conservando el resto de
// los query params: primera y última página siempre visibles, ventana de
// ±2 alrededor de la actual y elipsis en los huecos.
func buildPagination(m store.Metadata, qs url.Values) []pageLink {
	if m.LastPage <= 1 {
		return nil
	}

	pageURL := func(page int) string {
		values := url.Values{}
		for k, v := range qs {
			values[k] = v
		}
		values.Set("page", strconv.Itoa(page))
		return "/books?" + values.Encode()
	}

	var links []pageLink
	prevShown := 0
	for page := 1; page <= m.LastPage; page++ {
		show := page == 1 || page == m.LastPage ||
			(page >= m.CurrentPage-2 && page <= m.CurrentPage+2)
		if !show {
			continue
		}
		if prevShown != 0 && page != prevShown+1 {
			links = append(links, pageLink{Ellipsis: true})
		}
		links = append(links, pageLink{
			Label:   strconv.Itoa(page),
			URL:     pageURL(page),
			Current: page == m.CurrentPage,
		})
		prevShown = page
	}
	return links
}
