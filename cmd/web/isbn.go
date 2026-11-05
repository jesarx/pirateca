package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Autollenado por ISBN.
//
// Antes lo hacía el navegador consultando solo OpenLibrary, que tiene
// poca cobertura de libros en español y a veces tarda o falla. Ahora la
// consulta la hace el servidor (funciona igual desde Tor o con JS
// restringido) contra varias fuentes EN PARALELO y combina los
// resultados campo por campo:
//
//  1. Google Books (la mejor cobertura en español; acepta una API key
//     opcional en PIRATECA_GOOGLE_BOOKS_KEY porque sin key Google
//     comparte una cuota diaria que a veces se agota).
//  2. OpenLibrary, API de datos por ISBN.
//  3. OpenLibrary, buscador (encuentra ediciones que la API de datos no
//     tiene indexadas por ISBN).
//
// Se busca con el ISBN-13 y, si existe, también con el ISBN-10.

type isbnAuthor struct {
	Name     string `json:"name"`
	LastName string `json:"last_name"`
}

type isbnResult struct {
	Found       bool         `json:"found"`
	ISBN        string       `json:"isbn"`
	Sources     []string     `json:"sources"`
	Title       string       `json:"title,omitempty"`
	Subtitle    string       `json:"subtitle,omitempty"`
	Authors     []isbnAuthor `json:"authors,omitempty"`
	Publisher   string       `json:"publisher,omitempty"`
	Year        int          `json:"year,omitempty"`
	Pages       int          `json:"pages,omitempty"`
	Description string       `json:"description,omitempty"`
	Errors      []string     `json:"errors,omitempty"`
}

// isbnRecord es lo que aporta una fuente.
type isbnRecord struct {
	source      string
	title       string
	subtitle    string
	authors     []string
	publisher   string
	year        int
	pages       int
	description string
}

type isbnLookup struct {
	client      *http.Client
	googleKey   string
	googleBase  string
	openLibBase string

	mu    sync.Mutex
	cache map[string]isbnCacheEntry
}

type isbnCacheEntry struct {
	result isbnResult
	at     time.Time
}

func newISBNLookup(googleKey string) *isbnLookup {
	return &isbnLookup{
		client:      &http.Client{Timeout: 10 * time.Second},
		googleKey:   googleKey,
		googleBase:  "https://www.googleapis.com",
		openLibBase: "https://openlibrary.org",
		cache:       map[string]isbnCacheEntry{},
	}
}

// normalizeISBN limpia guiones/espacios, valida el dígito verificador y
// devuelve el ISBN-13 y (si aplica) el ISBN-10 equivalente.
func normalizeISBN(raw string) (isbn13, isbn10 string, ok bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if (r >= '0' && r <= '9') || r == 'X' {
			b.WriteRune(r)
		} else if r != '-' && r != ' ' && r != '‐' && r != '‑' {
			return "", "", false
		}
	}
	s := b.String()
	switch len(s) {
	case 10:
		if !validISBN10(s) {
			return "", "", false
		}
		return isbn10to13(s), s, true
	case 13:
		if !validISBN13(s) {
			return "", "", false
		}
		if strings.HasPrefix(s, "978") {
			return s, isbn13to10(s), true
		}
		return s, "", true
	}
	return "", "", false
}

func validISBN10(s string) bool {
	sum := 0
	for i := 0; i < 10; i++ {
		var d int
		switch {
		case s[i] >= '0' && s[i] <= '9':
			d = int(s[i] - '0')
		case s[i] == 'X' && i == 9:
			d = 10
		default:
			return false
		}
		sum += d * (10 - i)
	}
	return sum%11 == 0
}

func validISBN13(s string) bool {
	sum := 0
	for i := 0; i < 13; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		d := int(s[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return sum%10 == 0
}

func isbn10to13(s string) string {
	base := "978" + s[:9]
	sum := 0
	for i := 0; i < 12; i++ {
		d := int(base[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return base + strconv.Itoa((10-sum%10)%10)
}

func isbn13to10(s string) string {
	base := s[3:12]
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(base[i]-'0') * (10 - i)
	}
	check := (11 - sum%11) % 11
	if check == 10 {
		return base + "X"
	}
	return base + strconv.Itoa(check)
}

var yearRe = regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`)

func parseYear(s string) int {
	if m := yearRe.FindString(s); m != "" {
		y, _ := strconv.Atoi(m)
		return y
	}
	return 0
}

func (l *isbnLookup) getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	// OpenLibrary pide identificar a la aplicación.
	req.Header.Set("User-Agent", "Pirateca/1.0 (+https://pirateca.com)")
	req.Header.Set("Accept", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		if resp.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("cuota agotada (HTTP 429)")
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

func (l *isbnLookup) fromGoogle(ctx context.Context, isbns []string) (*isbnRecord, error) {
	var lastErr error
	for _, isbn := range isbns {
		u := l.googleBase + "/books/v1/volumes?q=isbn:" + url.QueryEscape(isbn)
		if l.googleKey != "" {
			u += "&key=" + url.QueryEscape(l.googleKey)
		}
		var out struct {
			Items []struct {
				VolumeInfo struct {
					Title         string   `json:"title"`
					Subtitle      string   `json:"subtitle"`
					Authors       []string `json:"authors"`
					Publisher     string   `json:"publisher"`
					PublishedDate string   `json:"publishedDate"`
					PageCount     int      `json:"pageCount"`
					Description   string   `json:"description"`
				} `json:"volumeInfo"`
			} `json:"items"`
		}
		if err := l.getJSON(ctx, u, &out); err != nil {
			lastErr = err
			continue
		}
		if len(out.Items) == 0 {
			continue
		}
		v := out.Items[0].VolumeInfo
		return &isbnRecord{
			source:      "Google Books",
			title:       v.Title,
			subtitle:    v.Subtitle,
			authors:     v.Authors,
			publisher:   v.Publisher,
			year:        parseYear(v.PublishedDate),
			pages:       v.PageCount,
			description: v.Description,
		}, nil
	}
	return nil, lastErr
}

func (l *isbnLookup) fromOpenLibraryData(ctx context.Context, isbns []string) (*isbnRecord, error) {
	keys := make([]string, len(isbns))
	for i, isbn := range isbns {
		keys[i] = "ISBN:" + isbn
	}
	u := l.openLibBase + "/api/books?format=json&jscmd=data&bibkeys=" + url.QueryEscape(strings.Join(keys, ","))
	type named struct {
		Name string `json:"name"`
	}
	var out map[string]struct {
		Title         string  `json:"title"`
		Subtitle      string  `json:"subtitle"`
		Authors       []named `json:"authors"`
		Publishers    []named `json:"publishers"`
		PublishDate   string  `json:"publish_date"`
		NumberOfPages int     `json:"number_of_pages"`
	}
	if err := l.getJSON(ctx, u, &out); err != nil {
		return nil, err
	}
	for _, key := range keys {
		d, ok := out[key]
		if !ok || d.Title == "" {
			continue
		}
		rec := &isbnRecord{
			source:   "OpenLibrary",
			title:    d.Title,
			subtitle: d.Subtitle,
			year:     parseYear(d.PublishDate),
			pages:    d.NumberOfPages,
		}
		for _, a := range d.Authors {
			rec.authors = append(rec.authors, a.Name)
		}
		if len(d.Publishers) > 0 {
			rec.publisher = d.Publishers[0].Name
		}
		return rec, nil
	}
	return nil, nil
}

func (l *isbnLookup) fromOpenLibrarySearch(ctx context.Context, isbns []string) (*isbnRecord, error) {
	u := l.openLibBase + "/search.json?limit=1&fields=title,subtitle,author_name,publisher,publish_year,first_publish_year,number_of_pages_median&isbn=" + url.QueryEscape(isbns[0])
	var out struct {
		Docs []struct {
			Title            string   `json:"title"`
			Subtitle         string   `json:"subtitle"`
			AuthorName       []string `json:"author_name"`
			Publisher        []string `json:"publisher"`
			PublishYear      []int    `json:"publish_year"`
			FirstPublishYear int      `json:"first_publish_year"`
			Pages            int      `json:"number_of_pages_median"`
		} `json:"docs"`
	}
	if err := l.getJSON(ctx, u, &out); err != nil {
		return nil, err
	}
	if len(out.Docs) == 0 || out.Docs[0].Title == "" {
		return nil, nil
	}
	d := out.Docs[0]
	rec := &isbnRecord{
		source:   "OpenLibrary (buscador)",
		title:    d.Title,
		subtitle: d.Subtitle,
		authors:  d.AuthorName,
		pages:    d.Pages,
	}
	// La edición buscada suele ser la más reciente de las listadas.
	for _, y := range d.PublishYear {
		if y > rec.year {
			rec.year = y
		}
	}
	if rec.year == 0 {
		rec.year = d.FirstPublishYear
	}
	if len(d.Publisher) > 0 {
		rec.publisher = d.Publisher[0]
	}
	return rec, nil
}

// lookup consulta todas las fuentes en paralelo y combina: para cada
// campo gana la primera fuente (en orden de prioridad) que lo tenga.
func (l *isbnLookup) lookup(ctx context.Context, isbn13, isbn10 string) isbnResult {
	l.mu.Lock()
	if e, ok := l.cache[isbn13]; ok && time.Since(e.at) < 24*time.Hour {
		l.mu.Unlock()
		return e.result
	}
	l.mu.Unlock()

	isbns := []string{isbn13}
	if isbn10 != "" {
		isbns = append(isbns, isbn10)
	}

	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	type sourceFn struct {
		name string
		fn   func(context.Context, []string) (*isbnRecord, error)
	}
	sources := []sourceFn{
		{"Google Books", l.fromGoogle},
		{"OpenLibrary", l.fromOpenLibraryData},
		{"OpenLibrary (buscador)", l.fromOpenLibrarySearch},
	}
	records := make([]*isbnRecord, len(sources))
	errs := make([]error, len(sources))
	var wg sync.WaitGroup
	for i, s := range sources {
		wg.Add(1)
		go func(i int, s sourceFn) {
			defer wg.Done()
			records[i], errs[i] = s.fn(ctx, isbns)
		}(i, s)
	}
	wg.Wait()

	res := isbnResult{ISBN: isbn13}
	for i, rec := range records {
		if errs[i] != nil {
			res.Errors = append(res.Errors, sources[i].name+": "+errs[i].Error())
		}
		if rec == nil {
			continue
		}
		res.Found = true
		res.Sources = append(res.Sources, rec.source)
		if res.Title == "" {
			res.Title, res.Subtitle = strings.TrimSpace(rec.title), strings.TrimSpace(rec.subtitle)
		}
		if len(res.Authors) == 0 {
			for _, a := range rec.authors {
				if a = strings.TrimSpace(a); a != "" {
					name, last := splitAuthorName(a)
					res.Authors = append(res.Authors, isbnAuthor{Name: name, LastName: last})
				}
			}
		}
		if res.Publisher == "" {
			res.Publisher = strings.TrimSpace(rec.publisher)
		}
		if res.Year == 0 {
			res.Year = rec.year
		}
		if res.Pages == 0 && rec.pages > 0 {
			res.Pages = rec.pages
		}
		if res.Description == "" {
			res.Description = strings.TrimSpace(rec.description)
		}
	}

	// Solo se cachean las respuestas completas (sin errores de red), para
	// no recordar un "no encontrado" causado por una fuente caída.
	if len(res.Errors) == 0 {
		l.mu.Lock()
		if len(l.cache) > 500 {
			l.cache = map[string]isbnCacheEntry{}
		}
		l.cache[isbn13] = isbnCacheEntry{result: res, at: time.Now()}
		l.mu.Unlock()
	}
	return res
}

// Partículas que forman parte del apellido: "Simone de Beauvoir",
// "Ludwig van Beethoven", "Gabriel de la Torre".
var surnameParticles = map[string]bool{
	"de": true, "del": true, "la": true, "las": true, "los": true, "y": true,
	"van": true, "von": true, "der": true, "den": true, "da": true, "das": true,
	"do": true, "dos": true, "di": true, "du": true, "le": true, "st.": true,
}

// splitAuthorName separa "Nombre Apellido" o "Apellido, Nombre". Para
// nombres con dos apellidos no hay forma segura de saberlo: se toma la
// última palabra (más sus partículas) como apellido y el formulario deja
// corregirlo.
func splitAuthorName(full string) (name, lastName string) {
	full = strings.Join(strings.Fields(full), " ")
	if before, after, ok := strings.Cut(full, ","); ok {
		return strings.TrimSpace(after), strings.TrimSpace(before)
	}
	words := strings.Fields(full)
	if len(words) <= 1 {
		return "", full
	}
	start := len(words) - 1
	for start > 1 && surnameParticles[strings.ToLower(words[start-1])] {
		start--
	}
	return strings.Join(words[:start], " "), strings.Join(words[start:], " ")
}

// isbnLookupHandler responde JSON para el botón "Autollenar por ISBN".
func (app *application) isbnLookupHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	isbn13, isbn10, ok := normalizeISBN(r.URL.Query().Get("isbn"))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "ISBN inválido: revisa que tenga 10 o 13 dígitos y el dígito verificador correcto."})
		return
	}
	res := app.isbn.lookup(r.Context(), isbn13, isbn10)
	if len(res.Errors) > 0 {
		app.logger.Warn("isbn lookup source errors", "isbn", isbn13, "errors", strings.Join(res.Errors, "; "))
	}
	json.NewEncoder(w).Encode(res)
}
