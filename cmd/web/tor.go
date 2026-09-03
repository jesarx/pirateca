package main

import (
	"net/http"
	"strings"
)

// Soporte para servir el sitio también como servicio onion de Tor.
//
// La idea central: cuando alguien entra por la dirección .onion, ninguna
// URL absoluta que genere el sitio debe apuntar al clearnet. Si el
// canonical, el og:url o el sitemap dijeran "https://pirateca.com", el
// navegador de quien nos visita en Tor podría salirse de la red — justo
// lo que vino a evitar. Por eso la URL base se deriva de la petición.

// isOnionRequest indica si la petición llegó por el servicio onion.
func isOnionRequest(r *http.Request) bool {
	return strings.HasSuffix(strings.ToLower(requestHost(r)), ".onion")
}

// requestHost devuelve el host de la petición sin el puerto.
func requestHost(r *http.Request) string {
	host := r.Host
	if h, _, found := strings.Cut(host, ":"); found {
		host = h
	}
	return strings.TrimSpace(host)
}

// baseURLFor es la URL base para enlaces absolutos de ESTA petición: la
// propia dirección onion si se entró por Tor, y si no la URL pública
// configurada (que es la canónica para buscadores, aunque se haya
// entrado por www o por IP).
func (app *application) baseURLFor(r *http.Request) string {
	if isOnionRequest(r) {
		// Los servicios onion se sirven por HTTP: el cifrado y la
		// autenticación del destino los da la propia red Tor.
		return "http://" + requestHost(r)
	}
	return app.config.baseURL
}

// onionLocation es la dirección onion completa que se anuncia a Tor
// Browser. Vacía si no hay servicio onion configurado.
func (app *application) onionLocation(r *http.Request) string {
	if app.config.onionAddress == "" || isOnionRequest(r) {
		return ""
	}
	return "http://" + app.config.onionAddress + r.URL.RequestURI()
}

// isSecureRequest indica si la conexión con el cliente va cifrada, para
// decidir el flag Secure de la cookie de sesión. En el servicio onion el
// tráfico viaja cifrado dentro de Tor pero el esquema es http://, y
// marcar la cookie como Secure ahí impediría iniciar sesión en algunos
// navegadores.
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
