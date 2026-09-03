package main

import (
	"fmt"
	"net/http"
	"net/url"
)

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				app.serverError(w, r, fmt.Errorf("%s", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Info("request", "method", r.Method, "path", r.URL.Path, "ip", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func (app *application) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")

		if isOnionRequest(r) {
			// Desde el servicio onion no se filtra a dónde se navega:
			// mandar nuestro origen revelaría la dirección .onion a los
			// sitios enlazados.
			w.Header().Set("Referrer-Policy", "no-referrer")
		} else {
			w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
			// Anuncia la versión onion: Tor Browser ofrece saltar a ella
			// con un botón en la barra de direcciones.
			if loc := app.onionLocation(r); loc != "" {
				w.Header().Set("Onion-Location", loc)
			}
		}

		next.ServeHTTP(w, r)
	})
}

// checkOrigin es la protección CSRF: los POST deben venir del propio
// sitio. Se complementa con SameSite=Lax en la cookie de sesión.
func (app *application) checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = r.Header.Get("Referer")
			}
			if origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					http.Error(w, "invalid origin", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func cacheControl(value string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(w, r)
	})
}

// requireAuth protege las rutas del dashboard.
func (app *application) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.isAuthenticated(r) {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next(w, r)
	}
}
