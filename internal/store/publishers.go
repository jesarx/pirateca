package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Publisher struct {
	ID    int64
	Name  string
	Slug  string
	Books int
}

type PublisherFilters struct {
	Search   string
	Sort     string
	Page     int
	PageSize int
}

var publisherSortSafelist = map[string]string{
	"name":        "p.name ASC",
	"-name":       "p.name DESC",
	"book_count":  "book_count ASC",
	"-book_count": "book_count DESC",
}

func (f *PublisherFilters) normalize() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 50
	}
	if _, ok := publisherSortSafelist[f.Sort]; !ok {
		f.Sort = "name"
	}
}

func (s *Store) ListPublishers(ctx context.Context, f PublisherFilters) ([]Publisher, Metadata, error) {
	f.normalize()

	query := fmt.Sprintf(`
		SELECT
			count(*) OVER(),
			p.id, p.name, p.slug,
			COUNT(b.id) AS book_count
		FROM publishers p
		LEFT JOIN books b ON b.pub_id = p.id
		WHERE ($1 = '' OR unaccent(p.name) ILIKE '%%' || unaccent($1) || '%%')
		GROUP BY p.id, p.name, p.slug
		ORDER BY %s, p.name ASC, p.id ASC
		LIMIT $2 OFFSET $3`,
		publisherSortSafelist[f.Sort],
	)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, query, f.Search, f.PageSize, (f.Page-1)*f.PageSize)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	totalRecords := 0
	publishers := []Publisher{}
	for rows.Next() {
		var p Publisher
		if err := rows.Scan(&totalRecords, &p.ID, &p.Name, &p.Slug, &p.Books); err != nil {
			return nil, Metadata{}, err
		}
		publishers = append(publishers, p)
	}
	if err := rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	return publishers, calculateMetadata(totalRecords, f.Page, f.PageSize), nil
}

func (s *Store) GetPublisherBySlug(ctx context.Context, slug string) (*Publisher, error) {
	if slug == "" {
		return nil, ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var p Publisher
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug FROM publishers WHERE slug = $1`, slug,
	).Scan(&p.ID, &p.Name, &p.Slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}
