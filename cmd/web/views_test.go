package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/jesarx/pirateca/internal/store"
)

func TestBuildPaginationArrows(t *testing.T) {
	qs := url.Values{"sort": {"-year"}, "page": {"3"}}

	p := buildPagination(store.Metadata{CurrentPage: 3, LastPage: 10}, "/books", qs)
	if p == nil {
		t.Fatal("se esperaba paginación")
	}
	if !strings.Contains(p.PrevURL, "page=2") || !strings.Contains(p.PrevURL, "sort=-year") {
		t.Errorf("PrevURL = %q", p.PrevURL)
	}
	if !strings.Contains(p.NextURL, "page=4") || !strings.Contains(p.NextURL, "sort=-year") {
		t.Errorf("NextURL = %q", p.NextURL)
	}

	first := buildPagination(store.Metadata{CurrentPage: 1, LastPage: 10}, "/books", url.Values{})
	if first.PrevURL != "" || first.NextURL == "" {
		t.Errorf("primera página: prev=%q next=%q", first.PrevURL, first.NextURL)
	}
	last := buildPagination(store.Metadata{CurrentPage: 10, LastPage: 10}, "/books", url.Values{})
	if last.NextURL != "" || last.PrevURL == "" {
		t.Errorf("última página: prev=%q next=%q", last.PrevURL, last.NextURL)
	}

	if buildPagination(store.Metadata{CurrentPage: 1, LastPage: 1}, "/books", url.Values{}) != nil {
		t.Error("con una sola página no debe haber paginación")
	}
}
