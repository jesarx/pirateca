package store

import (
	"context"
	"time"
)

type Tag struct {
	Name  string
	Books int
}

func (s *Store) ListTags(ctx context.Context) ([]Tag, error) {
	query := `
		SELECT t.tag, COUNT(b.id) AS book_count
		FROM (SELECT DISTINCT UNNEST(tags) AS tag FROM books) AS t
		LEFT JOIN books b ON t.tag = ANY(b.tags)
		GROUP BY t.tag
		ORDER BY t.tag ASC`

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Name, &t.Books); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tags, nil
}
