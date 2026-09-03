package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testOnion = "piratecaabcdefghijklmnopqrstuvwxyz234567abcdefghijklmnop.onion"

func testApp() *application {
	return &application{config: config{
		env:          "production",
		baseURL:      "https://pirateca.com",
		onionAddress: testOnion,
	}}
}

func request(host, target string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Host = host
	return r
}

func TestIsOnionRequest(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{testOnion, true},
		{testOnion + ":8080", true},
		{"PIRATECA" + testOnion[8:], true}, // insensible a mayúsculas
		{"pirateca.com", false},
		{"www.pirateca.com", false},
		{"pirateca.com.evil.example", false}, // no basta con contener ".onion"
		{"", false},
	}
	for _, tc := range tests {
		if got := isOnionRequest(request(tc.host, "/books")); got != tc.want {
			t.Errorf("isOnionRequest(%q) = %v, se esperaba %v", tc.host, got, tc.want)
		}
	}
}

// La garantía central: entrando por Tor, ninguna URL absoluta puede
// apuntar al clearnet, o el navegador de quien nos lee saldría de la red.
func TestBaseURLForKeepsOnionVisitorsInTor(t *testing.T) {
	app := testApp()

	if got := app.baseURLFor(request(testOnion, "/books")); got != "http://"+testOnion {
		t.Errorf("por onion la URL base fue %q, se esperaba la propia .onion", got)
	}
	if got := app.baseURLFor(request("pirateca.com", "/books")); got != "https://pirateca.com" {
		t.Errorf("por clearnet la URL base fue %q, se esperaba la canónica", got)
	}
	// Entrar por www o por un alias no debe cambiar la URL canónica.
	if got := app.baseURLFor(request("www.pirateca.com", "/books")); got != "https://pirateca.com" {
		t.Errorf("por www la URL base fue %q, se esperaba la canónica", got)
	}
}

func TestOnionLocation(t *testing.T) {
	app := testApp()

	want := "http://" + testOnion + "/books?sort=random"
	if got := app.onionLocation(request("pirateca.com", "/books?sort=random")); got != want {
		t.Errorf("onionLocation = %q, se esperaba %q", got, want)
	}
	// Ya dentro del servicio onion no se anuncia a sí mismo.
	if got := app.onionLocation(request(testOnion, "/books")); got != "" {
		t.Errorf("onionLocation dentro de onion = %q, se esperaba vacío", got)
	}
	// Sin servicio onion configurado no hay nada que anunciar.
	noOnion := &application{config: config{env: "production", baseURL: "https://pirateca.com"}}
	if got := noOnion.onionLocation(request("pirateca.com", "/books")); got != "" {
		t.Errorf("onionLocation sin configurar = %q, se esperaba vacío", got)
	}
}

// Marcar la cookie como Secure sobre el http:// del servicio onion
// impediría iniciar sesión; fuera de ahí siempre debe ir marcada.
func TestSecureCookie(t *testing.T) {
	app := testApp()

	if app.secureCookie(request(testOnion, "/admin/login")) {
		t.Error("la cookie del servicio onion no debe ser Secure")
	}
	if !app.secureCookie(request("pirateca.com", "/admin/login")) {
		t.Error("la cookie del clearnet debe ser Secure")
	}

	// Un onion servido tras TLS (poco común, pero válido) sí la admite.
	r := request(testOnion, "/admin/login")
	r.Header.Set("X-Forwarded-Proto", "https")
	if !app.secureCookie(r) {
		t.Error("con X-Forwarded-Proto=https la cookie debe ser Secure")
	}

	// En desarrollo nunca, o no habría login sobre http://localhost.
	dev := &application{config: config{env: "development", baseURL: "http://localhost:4000"}}
	if dev.secureCookie(request("localhost", "/admin/login")) {
		t.Error("en desarrollo la cookie no debe ser Secure")
	}
}

// Un referrer del propio sitio no es "origen del tráfico", tampoco
// cuando el sitio se sirve por Tor.
func TestSelfHostsIncludesOnion(t *testing.T) {
	app := testApp()
	hosts := app.selfHosts(request(testOnion, "/books"))

	if _, ok := normalizeReferrer("http://"+testOnion+"/books", hosts...); ok {
		t.Error("la navegación interna del servicio onion se contó como referrer externo")
	}
	if _, ok := normalizeReferrer("https://pirateca.com/tags", hosts...); ok {
		t.Error("el clearnet propio se contó como referrer externo")
	}
	if host, ok := normalizeReferrer("https://duckduckgo.com/", hosts...); !ok || host != "duckduckgo.com" {
		t.Errorf("un referrer externo debería contarse; se obtuvo %q, %v", host, ok)
	}
}
