package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
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

// RecordDownloads vuelca los contadores de descargas acumulados en
// memoria ((día, filename) → n).
func (s *Store) RecordDownloads(ctx context.Context, counts map[DayFile]int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for key, n := range counts {
		if n <= 0 {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO downloads (day, filename, count) VALUES ($1, $2, $3)
			ON CONFLICT (day, filename) DO UPDATE SET count = downloads.count + EXCLUDED.count`,
			key.Day.Format("2006-01-02"), key.Filename, n)
		if err != nil {
			return err
		}
	}
	return nil
}

// DayFile identifica el acumulador de descargas de un archivo en un día.
type DayFile struct {
	Day      time.Time
	Filename string
}

type DownloadStats struct {
	Today  int64
	Last7  int64
	Last30 int64
	Total  int64
}

func (s *Store) GetDownloadStats(ctx context.Context) (DownloadStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var ds DownloadStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(count) FILTER (WHERE day = CURRENT_DATE), 0),
			COALESCE(SUM(count) FILTER (WHERE day > CURRENT_DATE - 7), 0),
			COALESCE(SUM(count) FILTER (WHERE day > CURRENT_DATE - 30), 0),
			COALESCE(SUM(count), 0)
		FROM downloads`,
	).Scan(&ds.Today, &ds.Last7, &ds.Last30, &ds.Total)
	return ds, err
}

type BookDownloads struct {
	Title  string
	Slug   string
	Author string
	Count  int64
}

// GetTopDownloads devuelve los libros más descargados (histórico). Los
// registros de libros ya borrados no matchean el JOIN y quedan fuera.
func (s *Store) GetTopDownloads(ctx context.Context, limit int) ([]BookDownloads, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT b.title, b.slug, TRIM(COALESCE(a.name, '') || ' ' || a.last_name), SUM(d.count) AS n
		FROM downloads d
		JOIN books b ON b.filename = d.filename
		JOIN authors a ON b.auth_id = a.id
		GROUP BY b.title, b.slug, a.name, a.last_name
		ORDER BY n DESC, b.title ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	top := []BookDownloads{}
	for rows.Next() {
		var bd BookDownloads
		if err := rows.Scan(&bd.Title, &bd.Slug, &bd.Author, &bd.Count); err != nil {
			return nil, err
		}
		top = append(top, bd)
	}
	return top, rows.Err()
}

// DayCountry identifica el acumulador de visitas de un país en un día.
type DayCountry struct {
	Day     time.Time
	Country string
}

type CountryCount struct {
	Country string
	Count   int64
}

// LookupCountries resuelve un lote de IPs a su país usando la tabla de
// rangos. Las que no caen en ningún rango no aparecen en el resultado.
func (s *Store) LookupCountries(ctx context.Context, ips []string) (map[string]string, error) {
	if len(ips) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT host(ip.addr), (
			SELECT r.country FROM ip_country_ranges r
			WHERE r.start_ip <= ip.addr AND r.end_ip >= ip.addr
			ORDER BY r.start_ip DESC
			LIMIT 1
		)
		FROM unnest($1::inet[]) AS ip(addr)`, pq.Array(ips))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var ip string
		var country sql.NullString
		if err := rows.Scan(&ip, &country); err != nil {
			return nil, err
		}
		if country.Valid && country.String != "" {
			out[ip] = country.String
		}
	}
	return out, rows.Err()
}

// HasGeoIPData indica si la tabla de rangos está cargada.
func (s *Store) HasGeoIPData(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM ip_country_ranges LIMIT 1)`).Scan(&exists)
	return exists, err
}

func (s *Store) RecordCountries(ctx context.Context, counts map[DayCountry]int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for key, n := range counts {
		if n <= 0 {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO visitor_countries (day, country, count) VALUES ($1, $2, $3)
			ON CONFLICT (day, country) DO UPDATE SET count = visitor_countries.count + EXCLUDED.count`,
			key.Day.Format("2006-01-02"), key.Country, n)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetTopCountries devuelve los países con más visitas. Con days > 0 se
// limita a esa ventana de días; con 0, es el histórico completo.
func (s *Store) GetTopCountries(ctx context.Context, days, limit int) ([]CountryCount, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := `
		SELECT country, SUM(count) AS n
		FROM visitor_countries
		WHERE ($1 = 0 OR day > CURRENT_DATE - $1::int)
		GROUP BY country
		ORDER BY n DESC, country ASC
		LIMIT $2`

	rows, err := s.db.QueryContext(ctx, query, days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CountryCount{}
	for rows.Next() {
		var cc CountryCount
		if err := rows.Scan(&cc.Country, &cc.Count); err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, rows.Err()
}

// DayHost identifica el acumulador de referrers de un host en un día.
type DayHost struct {
	Day  time.Time
	Host string
}

type ReferrerCount struct {
	Host  string
	Count int64
}

type ReferrerHit struct {
	SeenAt time.Time
	Host   string
	URL    string
	Path   string
}

// maxReferrerHits es cuántos hits recientes se conservan; los más
// viejos se podan en cada flush para que la tabla no crezca sin fin.
const maxReferrerHits = 500

// RecordReferrers vuelca los contadores por host y los hits recientes.
func (s *Store) RecordReferrers(ctx context.Context, counts map[DayHost]int64, hits []ReferrerHit) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for key, n := range counts {
		if n <= 0 {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO referrers (day, host, count) VALUES ($1, $2, $3)
			ON CONFLICT (day, host) DO UPDATE SET count = referrers.count + EXCLUDED.count`,
			key.Day.Format("2006-01-02"), key.Host, n)
		if err != nil {
			return err
		}
	}

	for _, hit := range hits {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO referrer_hits (seen_at, host, url, path) VALUES ($1, $2, $3, $4)`,
			hit.SeenAt, hit.Host, hit.URL, hit.Path)
		if err != nil {
			return err
		}
	}

	if len(hits) > 0 {
		_, err := s.db.ExecContext(ctx, `
			DELETE FROM referrer_hits
			WHERE id NOT IN (SELECT id FROM referrer_hits ORDER BY seen_at DESC, id DESC LIMIT $1)`,
			maxReferrerHits)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetTopReferrers devuelve los orígenes con más visitas (histórico).
func (s *Store) GetTopReferrers(ctx context.Context, limit int) ([]ReferrerCount, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT host, SUM(count) AS n
		FROM referrers
		GROUP BY host
		ORDER BY n DESC, host ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	top := []ReferrerCount{}
	for rows.Next() {
		var rc ReferrerCount
		if err := rows.Scan(&rc.Host, &rc.Count); err != nil {
			return nil, err
		}
		top = append(top, rc)
	}
	return top, rows.Err()
}

// GetRecentReferrers devuelve los últimos enlaces por los que llegó
// alguien, del más reciente al más viejo.
func (s *Store) GetRecentReferrers(ctx context.Context, limit int) ([]ReferrerHit, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT seen_at, host, url, path
		FROM referrer_hits
		ORDER BY seen_at DESC, id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hits := []ReferrerHit{}
	for rows.Next() {
		var h ReferrerHit
		if err := rows.Scan(&h.SeenAt, &h.Host, &h.URL, &h.Path); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

type SitemapEntry struct {
	Path    string
	LastMod time.Time
}

// GetSitemapEntries devuelve las rutas públicas de libros, autores y
// editoriales para el sitemap.
func (s *Store) GetSitemapEntries(ctx context.Context) ([]SitemapEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT '/books/' || slug, created_at FROM books WHERE slug IS NOT NULL
		UNION ALL
		SELECT '/authors/' || slug, created_at FROM authors WHERE slug IS NOT NULL
		UNION ALL
		SELECT '/publishers/' || slug, created_at FROM publishers WHERE slug IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []SitemapEntry
	for rows.Next() {
		var e SitemapEntry
		if err := rows.Scan(&e.Path, &e.LastMod); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
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
