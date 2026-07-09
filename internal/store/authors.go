package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Author struct {
	ID       int64
	Name     string
	LastName string
	Slug     string
	Books    int
}

func (a Author) FullName() string {
	return strings.TrimSpace(a.Name + " " + a.LastName)
}

type AuthorFilters struct {
	Search   string
	Sort     string
	Page     int
	PageSize int
}

var authorSortSafelist = map[string]string{
	"last_name":   "a.last_name ASC",
	"-last_name":  "a.last_name DESC",
	"name":        "a.name ASC",
	"-name":       "a.name DESC",
	"book_count":  "book_count ASC",
	"-book_count": "book_count DESC",
}

func (f *AuthorFilters) normalize() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 50
	}
	if _, ok := authorSortSafelist[f.Sort]; !ok {
		f.Sort = "last_name"
	}
}

func (s *Store) ListAuthors(ctx context.Context, f AuthorFilters) ([]Author, Metadata, error) {
	f.normalize()

	query := fmt.Sprintf(`
		SELECT
			count(*) OVER(),
			a.id, COALESCE(a.name, ''), a.last_name, a.slug,
			COUNT(DISTINCT b.id) AS book_count
		FROM authors a
		LEFT JOIN books b ON (b.auth_id = a.id OR b.auth2_id = a.id)
		WHERE ($1 = '' OR unaccent(COALESCE(a.name, '') || ' ' || a.last_name) ILIKE '%%' || unaccent($1) || '%%')
		GROUP BY a.id, a.name, a.last_name, a.slug
		ORDER BY %s, a.last_name ASC, a.id ASC
		LIMIT $2 OFFSET $3`,
		authorSortSafelist[f.Sort],
	)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, query, f.Search, f.PageSize, (f.Page-1)*f.PageSize)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	totalRecords := 0
	authors := []Author{}
	for rows.Next() {
		var a Author
		if err := rows.Scan(&totalRecords, &a.ID, &a.Name, &a.LastName, &a.Slug, &a.Books); err != nil {
			return nil, Metadata{}, err
		}
		authors = append(authors, a)
	}
	if err := rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	return authors, calculateMetadata(totalRecords, f.Page, f.PageSize), nil
}

func (s *Store) GetAuthorBySlug(ctx context.Context, slug string) (*Author, error) {
	if slug == "" {
		return nil, ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var a Author
	err := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(name, ''), last_name, slug
		FROM authors
		WHERE slug = $1`, slug,
	).Scan(&a.ID, &a.Name, &a.LastName, &a.Slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}
