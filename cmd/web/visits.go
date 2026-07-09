package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jesarx/pirateca/internal/store"
)

// visitCounter acumula visitas en memoria y las vuelca a la base cada
// cierto tiempo, para que el conteo nunca agregue latencia ni escrituras
// por request. Si el proceso muere de golpe se pierden como mucho los
// últimos segundos de conteo — aceptable para un contador informativo.
type visitCounter struct {
	mu     sync.Mutex
	counts map[time.Time]int64
}

func newVisitCounter() *visitCounter {
	return &visitCounter{counts: map[time.Time]int64{}}
}

func (vc *visitCounter) add() {
	day := time.Now().UTC().Truncate(24 * time.Hour)
	vc.mu.Lock()
	vc.counts[day]++
	vc.mu.Unlock()
}

// drain devuelve lo acumulado y resetea el mapa.
func (vc *visitCounter) drain() map[time.Time]int64 {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	if len(vc.counts) == 0 {
		return nil
	}
	out := vc.counts
	vc.counts = map[time.Time]int64{}
	return out
}

// downloadCounter acumula descargas de PDFs en memoria, por día y nombre
// base de archivo, con el mismo esquema de flush que visitCounter.
type downloadCounter struct {
	mu     sync.Mutex
	counts map[store.DayFile]int64
}

func newDownloadCounter() *downloadCounter {
	return &downloadCounter{counts: map[store.DayFile]int64{}}
}

func (dc *downloadCounter) add(filename string) {
	key := store.DayFile{
		Day:      time.Now().UTC().Truncate(24 * time.Hour),
		Filename: filename,
	}
	dc.mu.Lock()
	dc.counts[key]++
	dc.mu.Unlock()
}

func (dc *downloadCounter) drain() map[store.DayFile]int64 {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if len(dc.counts) == 0 {
		return nil
	}
	out := dc.counts
	dc.counts = map[store.DayFile]int64{}
	return out
}

func (app *application) flushVisits(ctx context.Context) {
	if app.store == nil {
		return
	}

	if counts := app.visits.drain(); counts != nil {
		if err := app.store.RecordVisits(ctx, counts); err != nil {
			app.logger.Error("failed to flush visits", "error", err.Error())
			// Devolver los conteos no volcados para reintentar en el
			// siguiente flush.
			app.visits.mu.Lock()
			for day, n := range counts {
				app.visits.counts[day] += n
			}
			app.visits.mu.Unlock()
		}
	}

	if counts := app.downloads.drain(); counts != nil {
		if err := app.store.RecordDownloads(ctx, counts); err != nil {
			app.logger.Error("failed to flush downloads", "error", err.Error())
			app.downloads.mu.Lock()
			for key, n := range counts {
				app.downloads.counts[key] += n
			}
			app.downloads.mu.Unlock()
		}
	}
}

// startVisitFlusher vuelca el contador cada 30s hasta que ctx se cancele,
// con un último flush al salir.
func (app *application) startVisitFlusher(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			app.flushVisits(ctx)
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			app.flushVisits(flushCtx)
			cancel()
			return
		}
	}
}

// countVisits cuenta pageviews de las páginas públicas HTML. Ignora
// estáticos, descargas, admin, health y bots evidentes.
func (app *application) countVisits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && isPublicPage(r.URL.Path) && !looksLikeBot(r.UserAgent()) {
			app.visits.add()
		}
		next.ServeHTTP(w, r)
	})
}

func isPublicPage(path string) bool {
	switch {
	case path == "/", path == "/books", path == "/authors", path == "/publishers",
		path == "/tags", path == "/manifest", path == "/contact":
		return true
	case strings.HasPrefix(path, "/books/"),
		strings.HasPrefix(path, "/authors/"),
		strings.HasPrefix(path, "/publishers/"),
		strings.HasPrefix(path, "/news/"):
		return true
	}
	return false
}

func looksLikeBot(ua string) bool {
	if ua == "" {
		return true
	}
	ua = strings.ToLower(ua)
	for _, marker := range []string{"bot", "crawl", "spider", "slurp", "curl", "wget", "python-requests", "headless"} {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}
