package main

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
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

func authorSortOptions(current string) []sortOption {
	opts := []sortOption{
		{Value: "last_name", Label: "Apellido (A-Z)"},
		{Value: "-last_name", Label: "Apellido (Z-A)"},
		{Value: "name", Label: "Nombre (A-Z)"},
		{Value: "-name", Label: "Nombre (Z-A)"},
		{Value: "book_count", Label: "# de libros (menor a mayor)"},
		{Value: "-book_count", Label: "# de libros (mayor a menor)"},
	}
	for i := range opts {
		opts[i].Selected = opts[i].Value == current
	}
	return opts
}

func publisherSortOptions(current string) []sortOption {
	opts := []sortOption{
		{Value: "name", Label: "Nombre (A-Z)"},
		{Value: "-name", Label: "Nombre (Z-A)"},
		{Value: "book_count", Label: "# de libros (menor a mayor)"},
		{Value: "-book_count", Label: "# de libros (mayor a menor)"},
	}
	for i := range opts {
		opts[i].Selected = opts[i].Value == current
	}
	return opts
}

// chartBar es una barra precalculada para las gráficas CSS del dashboard
// (una sola serie, magnitud por longitud, tooltip nativo con title).
type chartBar struct {
	Label   string // etiqueta de eje; vacía = sin etiqueta (se rotulan pocas)
	Value   int64
	Pct     float64 // porcentaje de la barra respecto al máximo (0-100)
	Tooltip string
	URL     string // opcional: la barra enlaza (p. ej. a la categoría)
}

func barPct(value, max int64) float64 {
	if max <= 0 || value <= 0 {
		return 0
	}
	return float64(value) / float64(max) * 100
}

type dashData struct {
	Catalog     store.CatalogStats
	Visits      store.VisitStats
	VisitBars   []chartBar
	MonthBars   []chartBar
	TagBars     []chartBar
	LatestBooks []store.Book
}

var spanishMonths = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

func buildVisitBars(daily []store.DayCount) []chartBar {
	var max int64
	for _, d := range daily {
		if d.Count > max {
			max = d.Count
		}
	}
	bars := make([]chartBar, 0, len(daily))
	for i, d := range daily {
		label := ""
		// Rotular uno de cada cinco días y el último.
		if i%5 == 0 || i == len(daily)-1 {
			label = fmt.Sprintf("%d %s", d.Day.Day(), spanishMonths[d.Day.Month()-1])
		}
		bars = append(bars, chartBar{
			Label:   label,
			Value:   d.Count,
			Pct:     barPct(d.Count, max),
			Tooltip: fmt.Sprintf("%d de %s: %d visitas", d.Day.Day(), spanishMonths[d.Day.Month()-1], d.Count),
		})
	}
	return bars
}

func buildMonthBars(months []store.MonthCount) []chartBar {
	var max int64
	for _, m := range months {
		if m.Count > max {
			max = m.Count
		}
	}
	bars := make([]chartBar, 0, len(months))
	for _, m := range months {
		name := spanishMonths[m.Month.Month()-1]
		bars = append(bars, chartBar{
			Label:   name,
			Value:   m.Count,
			Pct:     barPct(m.Count, max),
			Tooltip: fmt.Sprintf("%s %d: %d libros", name, m.Month.Year(), m.Count),
		})
	}
	return bars
}

func buildTagBars(tags []store.Tag, top int) []chartBar {
	sort.Slice(tags, func(i, j int) bool { return tags[i].Books > tags[j].Books })
	if len(tags) > top {
		tags = tags[:top]
	}
	var max int64
	if len(tags) > 0 {
		max = int64(tags[0].Books)
	}
	bars := make([]chartBar, 0, len(tags))
	for _, t := range tags {
		bars = append(bars, chartBar{
			Label:   t.Name,
			Value:   int64(t.Books),
			Pct:     barPct(int64(t.Books), max),
			Tooltip: fmt.Sprintf("%s: %d libros", t.Name, t.Books),
			URL:     "/books?tags=" + url.QueryEscape(t.Name),
		})
	}
	return bars
}

type pageLink struct {
	Label    string
	URL      string
	Current  bool
	Ellipsis bool
}

// buildPagination arma los enlaces de paginación conservando el path
// actual y el resto de los query params: primera y última página siempre
// visibles, ventana de ±2 alrededor de la actual y elipsis en los huecos.
func buildPagination(m store.Metadata, path string, qs url.Values) []pageLink {
	if m.LastPage <= 1 {
		return nil
	}

	pageURL := func(page int) string {
		values := url.Values{}
		for k, v := range qs {
			values[k] = v
		}
		values.Set("page", strconv.Itoa(page))
		return path + "?" + values.Encode()
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
