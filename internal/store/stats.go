package store

import (
	"context"
	"time"
)

type CatalogStats struct {
	Books      int
	Authors    int
	Publishers int
	Tags       int
}

func (s *Store) GetCatalogStats(ctx context.Context) (CatalogStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var cs CatalogStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM books),
			(SELECT count(*) FROM authors),
			(SELECT count(*) FROM publishers),
			(SELECT count(DISTINCT tag) FROM books, UNNEST(tags) AS tag)`,
	).Scan(&cs.Books, &cs.Authors, &cs.Publishers, &cs.Tags)
	return cs, err
}

type DayCount struct {
	Day   time.Time
	Count int64
}

type MonthCount struct {
	Month time.Time
	Count int64
}

type VisitStats struct {
	Today  int64
	Last7  int64
	Last30 int64
	Total  int64
	Daily  []DayCount // últimos 30 días, incluye días en cero
}

// RecordVisits vuelca los contadores acumulados en memoria (día → n).
func (s *Store) RecordVisits(ctx context.Context, counts map[time.Time]int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for day, n := range counts {
		if n <= 0 {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO visits (day, count) VALUES ($1, $2)
			ON CONFLICT (day) DO UPDATE SET count = visits.count + EXCLUDED.count`,
			day.Format("2006-01-02"), n)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetVisitStats(ctx context.Context) (VisitStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var vs VisitStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(count) FILTER (WHERE day = CURRENT_DATE), 0),
			COALESCE(SUM(count) FILTER (WHERE day > CURRENT_DATE - 7), 0),
			COALESCE(SUM(count) FILTER (WHERE day > CURRENT_DATE - 30), 0),
			COALESCE(SUM(count), 0)
		FROM visits`,
	).Scan(&vs.Today, &vs.Last7, &vs.Last30, &vs.Total)
	if err != nil {
		return vs, err
	}

	// Serie diaria de los últimos 30 días con los huecos en cero.
	rows, err := s.db.QueryContext(ctx, `
		SELECT d::date, COALESCE(v.count, 0)
		FROM generate_series(CURRENT_DATE - 29, CURRENT_DATE, '1 day') AS d
		LEFT JOIN visits v ON v.day = d::date
		ORDER BY d`)
	if err != nil {
		return vs, err
	}
	defer rows.Close()

	for rows.Next() {
		var dc DayCount
		if err := rows.Scan(&dc.Day, &dc.Count); err != nil {
			return vs, err
		}
		vs.Daily = append(vs.Daily, dc)
	}
	return vs, rows.Err()
}

// GetBooksPerMonth devuelve los libros agregados por mes en los últimos
// 12 meses, incluyendo meses en cero.
func (s *Store) GetBooksPerMonth(ctx context.Context) ([]MonthCount, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT m, COALESCE(b.n, 0)
		FROM generate_series(
			date_trunc('month', CURRENT_DATE) - interval '11 months',
			date_trunc('month', CURRENT_DATE),
			'1 month') AS m
		LEFT JOIN (
			SELECT date_trunc('month', created_at) AS month, count(*) AS n
			FROM books GROUP BY 1
		) b ON b.month = m
		ORDER BY m`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var months []MonthCount
	for rows.Next() {
		var mc MonthCount
		if err := rows.Scan(&mc.Month, &mc.Count); err != nil {
			return nil, err
		}
		months = append(months, mc)
	}
	return months, rows.Err()
}
