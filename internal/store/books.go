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
			b.id, b.created_at, b.title, b.short_title,
			COALESCE(b.year, 0), COALESCE(b.tags, '{}'),
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

// BookInput reúne los campos editables de un libro para insert/update.
type BookInput struct {
	Title        string
	ShortTitle   string
	Year         int
	Tags         []string
	AuthorID     int64
	Author2ID    *int64
	PublisherID  int64
	Filename     string
	ISBN         string
	Description  string
	Pages        int
	ExternalLink string
	DirDwl       bool
}

func (s *Store) InsertBook(ctx context.Context, in BookInput) (id int64, slug string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// El trigger books_slug_trigger genera el slug BEFORE INSERT, por eso
	// se puede devolver con RETURNING.
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO books (title, short_title, year, tags, auth_id, auth2_id, pub_id,
			filename, isbn, description, pages, external_link, dir_dwl)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, 0), NULLIF($12, ''), $13)
		RETURNING id, slug`,
		in.Title, in.ShortTitle, in.Year, pq.Array(in.Tags), in.AuthorID, in.Author2ID,
		in.PublisherID, in.Filename, in.ISBN, in.Description, in.Pages, in.ExternalLink, in.DirDwl,
	).Scan(&id, &slug)
	return id, slug, err
}

func (s *Store) UpdateBook(ctx context.Context, id int64, version int, in BookInput) (slug string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// El filename no se toca en updates: es la llave de los archivos ya
	// publicados (pdf, portada, torrent) y el torrent dejaría de
	// corresponder si cambiara.
	err = s.db.QueryRowContext(ctx, `
		UPDATE books SET
			title = $1, short_title = $2, year = $3, tags = $4,
			auth_id = $5, auth2_id = $6, pub_id = $7,
			isbn = NULLIF($8, ''), description = NULLIF($9, ''), pages = NULLIF($10, 0),
			external_link = NULLIF($11, ''), dir_dwl = $12,
			version = version + 1
		WHERE id = $13 AND version = $14
		RETURNING slug`,
		in.Title, in.ShortTitle, in.Year, pq.Array(in.Tags), in.AuthorID, in.Author2ID,
		in.PublisherID, in.ISBN, in.Description, in.Pages, in.ExternalLink, in.DirDwl,
		id, version,
	).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrEditConflict
	}
	return slug, err
}

func (s *Store) DeleteBook(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `DELETE FROM books WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FilenameExists(ctx context.Context, filename string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM books WHERE filename = $1)`, filename,
	).Scan(&exists)
	return exists, err
}

func (s *Store) GetBookByID(ctx context.Context, id int64) (*Book, error) {
	if id < 1 {
		return nil, ErrNotFound
	}
	return s.getBook(ctx, "b.id = $1", id)
}

func (s *Store) GetBookBySlug(ctx context.Context, slug string) (*Book, error) {
	if slug == "" {
		return nil, ErrNotFound
	}
	return s.getBook(ctx, "b.slug = $1", slug)
}

func (s *Store) getBook(ctx context.Context, where string, arg any) (*Book, error) {
	query := `
		SELECT
			b.id, b.created_at, b.title, b.short_title,
			COALESCE(b.year, 0), COALESCE(b.tags, '{}'),
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
		WHERE ` + where

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var b Book
	err := s.db.QueryRowContext(ctx, query, arg).Scan(
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
