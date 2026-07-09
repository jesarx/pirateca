package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Book struct {
	ID              int64
	CreatedAt       time.Time
	Title           string
	ShortTitle      string
	Year            int
	Tags            []string
	Slug            string
	Filename        string
	ISBN            string
	Description     string
	Pages           int
	ExternalLink    string
	DirDwl          bool
	Version         int
	AuthorID        int64
	AuthorName      string
	AuthorLastName  string
	AuthorSlug      string
	Author2ID       *int64
	Author2Name     *string
	Author2LastName *string
	Author2Slug     *string
	PublisherID     int64
	PublisherName   string
	PublisherSlug   string
}

// AuthorFullName arma "Nombre Apellido" tolerando el nombre vacío
// (authors.name es nullable en el esquema).
func (b Book) AuthorFullName() string {
	return strings.TrimSpace(b.AuthorName + " " + b.AuthorLastName)
}

func (b Book) Author2FullName() string {
	if b.Author2LastName == nil {
		return ""
	}
	name := ""
	if b.Author2Name != nil {
		name = *b.Author2Name
	}
	return strings.TrimSpace(name + " " + *b.Author2LastName)
}

// BookFilters son los filtros del listado público; se mapean 1:1 con los
// query params históricos del sitio (title, tags, authslug, pubslug, sort,
// page) para no romper URLs.
type BookFilters struct {
	Search        string
	Tags          []string
	AuthorSlug    string
	PublisherSlug string
	Sort          string
	Page          int
	PageSize      int
}

var bookSortSafelist = map[string]string{
	"created_at":  "b.created_at ASC",
	"-created_at": "b.created_at DESC",
	"title":       "b.title ASC",
	"-title":      "b.title DESC",
	"year":        "b.year ASC",
	"-year":       "b.year DESC",
	"random":      "random()",
}

func (f *BookFilters) normalize() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	if _, ok := bookSortSafelist[f.Sort]; !ok {
		f.Sort = "-created_at"
	}
	if f.Tags == nil {
		f.Tags = []string{}
	}
}

func (s *Store) ListBooks(ctx context.Context, f BookFilters) ([]Book, Metadata, error) {
	f.normalize()

	query := fmt.Sprintf(`
		SELECT
			count(*) OVER(),
			b.id, b.created_at, b.title, b.short_title, b.year, b.tags,
			b.slug, COALESCE(b.filename, ''), b.dir_dwl, b.version,
			b.auth_id, COALESCE(a.name, ''), a.last_name, a.slug,
			b.auth2_id, a2.name, a2.last_name, a2.slug,
			b.pub_id, p.name, p.slug
		FROM books b
		JOIN authors a ON b.auth_id = a.id
		LEFT JOIN authors a2 ON b.auth2_id = a2.id
		JOIN publishers p ON b.pub_id = p.id
		WHERE
			($1 = '' OR to_tsvector('spanish', unaccent(b.title)) @@ plainto_tsquery('spanish', unaccent($1)))
			AND (b.tags @> $2 OR $2 = '{}')
			AND ($3 = '' OR a.slug = $3 OR a2.slug = $3)
			AND ($4 = '' OR p.slug = $4)
		ORDER BY %s, b.title ASC
		LIMIT $5 OFFSET $6`,
		bookSortSafelist[f.Sort],
	)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	args := []any{f.Search, pq.Array(f.Tags), f.AuthorSlug, f.PublisherSlug, f.PageSize, (f.Page - 1) * f.PageSize}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	totalRecords := 0
	books := []Book{}

	for rows.Next() {
		var b Book
		err := rows.Scan(
			&totalRecords,
			&b.ID, &b.CreatedAt, &b.Title, &b.ShortTitle, &b.Year, pq.Array(&b.Tags),
			&b.Slug, &b.Filename, &b.DirDwl, &b.Version,
			&b.AuthorID, &b.AuthorName, &b.AuthorLastName, &b.AuthorSlug,
			&b.Author2ID, &b.Author2Name, &b.Author2LastName, &b.Author2Slug,
			&b.PublisherID, &b.PublisherName, &b.PublisherSlug,
		)
		if err != nil {
			return nil, Metadata{}, err
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	return books, calculateMetadata(totalRecords, f.Page, f.PageSize), nil
}

func (s *Store) GetBookBySlug(ctx context.Context, slug string) (*Book, error) {
	if slug == "" {
		return nil, ErrNotFound
	}

	query := `
		SELECT
			b.id, b.created_at, b.title, b.short_title, b.year, b.tags,
			b.slug, COALESCE(b.filename, ''), COALESCE(b.isbn, ''),
			COALESCE(b.description, ''), COALESCE(b.pages, 0),
			COALESCE(b.external_link, ''), b.dir_dwl, b.version,
			b.auth_id, COALESCE(a.name, ''), a.last_name, a.slug,
			b.auth2_id, a2.name, a2.last_name, a2.slug,
			b.pub_id, p.name, p.slug
		FROM books b
		JOIN authors a ON b.auth_id = a.id
		LEFT JOIN authors a2 ON b.auth2_id = a2.id
		JOIN publishers p ON b.pub_id = p.id
		WHERE b.slug = $1`

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var b Book
	err := s.db.QueryRowContext(ctx, query, slug).Scan(
		&b.ID, &b.CreatedAt, &b.Title, &b.ShortTitle, &b.Year, pq.Array(&b.Tags),
		&b.Slug, &b.Filename, &b.ISBN, &b.Description, &b.Pages,
		&b.ExternalLink, &b.DirDwl, &b.Version,
		&b.AuthorID, &b.AuthorName, &b.AuthorLastName, &b.AuthorSlug,
		&b.Author2ID, &b.Author2Name, &b.Author2LastName, &b.Author2Slug,
		&b.PublisherID, &b.PublisherName, &b.PublisherSlug,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &b, nil
}
