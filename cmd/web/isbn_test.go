package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeISBN(t *testing.T) {
	tests := []struct {
		in       string
		want13   string
		want10   string
		wantOkay bool
	}{
		{"978-84-339-2015-7", "9788433920157", "8433920154", true},
		{"84-339-2015-4", "9788433920157", "8433920154", true},
		{" 9780306406157 ", "9780306406157", "0306406152", true},
		{"0-8044-2957-X", "9780804429573", "080442957X", true},
		{"979-10-90636-07-1", "9791090636071", "", true},
		{"9788433920158", "", "", false}, // dígito verificador malo
		{"12345", "", "", false},
		{"abc", "", "", false},
	}
	for _, tt := range tests {
		got13, got10, ok := normalizeISBN(tt.in)
		if ok != tt.wantOkay || got13 != tt.want13 || got10 != tt.want10 {
			t.Errorf("normalizeISBN(%q) = %q, %q, %v; want %q, %q, %v",
				tt.in, got13, got10, ok, tt.want13, tt.want10, tt.wantOkay)
		}
	}
}

func TestSplitAuthorName(t *testing.T) {
	tests := []struct{ in, name, last string }{
		{"Gabriel García Márquez", "Gabriel García", "Márquez"},
		{"García Márquez, Gabriel", "Gabriel", "García Márquez"},
		{"Simone de Beauvoir", "Simone", "de Beauvoir"},
		{"Ludwig van Beethoven", "Ludwig", "van Beethoven"},
		{"Platón", "", "Platón"},
		{"  Guy   Debord ", "Guy", "Debord"},
	}
	for _, tt := range tests {
		name, last := splitAuthorName(tt.in)
		if name != tt.name || last != tt.last {
			t.Errorf("splitAuthorName(%q) = %q, %q; want %q, %q", tt.in, name, last, tt.name, tt.last)
		}
	}
}

// Respuestas reales (recortadas) de cada fuente.
const googleBody = `{"totalItems":1,"items":[{"volumeInfo":{"title":"Cien años de soledad","authors":["Gabriel García Márquez"],"publisher":"Diana","publishedDate":"1986-05","description":"La saga de los Buendía.","pageCount":351}}]}`
const openLibDataBody = `{"ISBN:9788433920157":{"title":"Cien anos de soledad","authors":[{"name":"Gabriel García Márquez"}],"publishers":[{"name":"Anagrama"}],"publish_date":"2007","number_of_pages":471}}`
const openLibSearchBody = `{"docs":[{"title":"Cien años","author_name":["G. G. Márquez"],"publisher":["Otra"],"publish_year":[1970,2007],"number_of_pages_median":400}]}`

func newFakeISBNSources(t *testing.T, google, olData, olSearch func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.UserAgent(); !strings.HasPrefix(ua, "Pirateca/") {
			t.Errorf("User-Agent inesperado: %q", ua)
		}
		switch r.URL.Path {
		case "/books/v1/volumes":
			google(w)
		case "/api/books":
			olData(w)
		case "/search.json":
			olSearch(w)
		default:
			http.NotFound(w, r)
		}
	}))
}

func body(s string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.Write([]byte(s)) }
}

func status(code int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

func testLookup(srv *httptest.Server) *isbnLookup {
	l := newISBNLookup("")
	l.googleBase, l.openLibBase = srv.URL, srv.URL
	return l
}

func TestISBNLookupMergesByPriority(t *testing.T) {
	srv := newFakeISBNSources(t, body(googleBody), body(openLibDataBody), body(openLibSearchBody))
	defer srv.Close()

	res := testLookup(srv).lookup(context.Background(), "9788433920157", "8433920154")
	if !res.Found || len(res.Sources) != 3 {
		t.Fatalf("res = %+v", res)
	}
	// Google tiene prioridad en todo lo que trae.
	if res.Title != "Cien años de soledad" || res.Publisher != "Diana" || res.Year != 1986 ||
		res.Pages != 351 || res.Description != "La saga de los Buendía." {
		t.Errorf("merge incorrecto: %+v", res)
	}
	if len(res.Authors) != 1 || res.Authors[0].LastName != "Márquez" || res.Authors[0].Name != "Gabriel García" {
		t.Errorf("autores: %+v", res.Authors)
	}
}

func TestISBNLookupFallsBackWhenGoogleFails(t *testing.T) {
	// El caso real de hoy: Google responde 429 (cuota sin API key agotada).
	srv := newFakeISBNSources(t, status(http.StatusTooManyRequests), body(openLibDataBody), body(`{"docs":[]}`))
	defer srv.Close()

	res := testLookup(srv).lookup(context.Background(), "9788433920157", "8433920154")
	if !res.Found || res.Title != "Cien anos de soledad" || res.Publisher != "Anagrama" || res.Year != 2007 || res.Pages != 471 {
		t.Fatalf("res = %+v", res)
	}
	if len(res.Errors) == 0 || !strings.Contains(res.Errors[0], "429") {
		t.Errorf("debería reportar el error de Google: %+v", res.Errors)
	}
}

func TestISBNLookupNotFound(t *testing.T) {
	srv := newFakeISBNSources(t, body(`{"totalItems":0}`), body(`{}`), body(`{"docs":[]}`))
	defer srv.Close()

	res := testLookup(srv).lookup(context.Background(), "9788433920157", "8433920154")
	if res.Found || len(res.Errors) != 0 {
		t.Fatalf("res = %+v", res)
	}
}
