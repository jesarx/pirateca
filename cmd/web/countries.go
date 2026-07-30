package main

import (
	"net"
	"net/http"
	"strings"
)

// clientIP obtiene la IP real del visitante. El servicio escucha solo en
// 127.0.0.1 detrás de nginx, así que las cabeceras que pone el proxy son
// de fiar; si algún día se expusiera directo, r.RemoteAddr sigue siendo
// el respaldo.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// El primero de la lista es el cliente original.
		if first, _, found := strings.Cut(xff, ","); found {
			xff = first
		}
		if ip := net.ParseIP(strings.TrimSpace(xff)); ip != nil {
			return ip.String()
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if ip := net.ParseIP(real); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return ip.String()
	}
	return ""
}

// countryFromHeaders lee el país si el proxy ya lo resolvió (Cloudflare
// manda CF-IPCountry). Ahorra el lookup contra la base de rangos.
func countryFromHeaders(r *http.Request) string {
	for _, h := range []string{"CF-IPCountry", "X-Country-Code"} {
		if v := strings.TrimSpace(r.Header.Get(h)); len(v) == 2 {
			v = strings.ToUpper(v)
			if v != "XX" && v != "T1" { // desconocido / red Tor
				return v
			}
		}
	}
	return ""
}

// countryFlag deriva la bandera emoji del código ISO 3166-1 alpha-2
// (cada letra se mapea a su símbolo indicador regional).
func countryFlag(code string) string {
	if len(code) != 2 {
		return "🏳"
	}
	code = strings.ToUpper(code)
	var flag []rune
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return "🏳"
		}
		flag = append(flag, rune(0x1F1E6+(c-'A')))
	}
	return string(flag)
}

// countryNames traduce el código ISO al nombre en español. La lista
// cubre los países de habla hispana y los de mayor tráfico en la web;
// para el resto se muestra el código.
var countryNames = map[string]string{
	"MX": "México", "ES": "España", "AR": "Argentina", "CO": "Colombia",
	"CL": "Chile", "PE": "Perú", "VE": "Venezuela", "EC": "Ecuador",
	"GT": "Guatemala", "CU": "Cuba", "BO": "Bolivia", "DO": "República Dominicana",
	"HN": "Honduras", "PY": "Paraguay", "SV": "El Salvador", "NI": "Nicaragua",
	"CR": "Costa Rica", "PA": "Panamá", "UY": "Uruguay", "PR": "Puerto Rico",
	"GQ": "Guinea Ecuatorial",
	"US": "Estados Unidos", "CA": "Canadá", "BR": "Brasil",
	"PT": "Portugal", "FR": "Francia", "DE": "Alemania", "IT": "Italia",
	"GB": "Reino Unido", "IE": "Irlanda", "NL": "Países Bajos", "BE": "Bélgica",
	"CH": "Suiza", "AT": "Austria", "SE": "Suecia", "NO": "Noruega",
	"DK": "Dinamarca", "FI": "Finlandia", "PL": "Polonia", "CZ": "Chequia",
	"GR": "Grecia", "RO": "Rumanía", "HU": "Hungría", "RU": "Rusia",
	"UA": "Ucrania", "TR": "Turquía", "IL": "Israel",
	"CN": "China", "JP": "Japón", "KR": "Corea del Sur", "IN": "India",
	"ID": "Indonesia", "PH": "Filipinas", "VN": "Vietnam", "TH": "Tailandia",
	"AU": "Australia", "NZ": "Nueva Zelanda",
	"MA": "Marruecos", "DZ": "Argelia", "EG": "Egipto", "ZA": "Sudáfrica",
	"NG": "Nigeria", "KE": "Kenia",
	"??": "Sin identificar",
}

func countryName(code string) string {
	if name, ok := countryNames[code]; ok {
		return name
	}
	return code
}
