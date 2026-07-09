package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
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

func (app *application) flushVisits(ctx context.Context) {
	counts := app.visits.drain()
	if counts == nil || app.store == nil {
		return
	}
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
