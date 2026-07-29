package main

import (
	"net/http"
	"net/url"
	"strings"
)

// searchEngineHosts agrupa variantes de un mismo origen bajo un host
// canónico: google.com.mx, google.es y google.com son "google.com", y
// los dominios de redirección (t.co, l.facebook.com) se muestran con el
// nombre del sitio real.
var canonicalHosts = map[string]string{
	"l.facebook.com":       "facebook.com",
	"lm.facebook.com":      "facebook.com",
	"m.facebook.com":       "facebook.com",
	"l.instagram.com":      "instagram.com",
	"out.reddit.com":       "reddit.com",
	"old.reddit.com":       "reddit.com",
	"t.co":                 "twitter.com",
	"x.com":                "twitter.com",
	"news.ycombinator.com": "ycombinator.com",
}

// friendlyNames traduce el host a un nombre legible en el dashboard.
var friendlyNames = map[string]string{
	"google.com":       "Google",
	"duckduckgo.com":   "DuckDuckGo",
	"bing.com":         "Bing",
	"ecosia.org":       "Ecosia",
	"search.brave.com": "Brave Search",
	"yandex.com":       "Yandex",
	"startpage.com":    "Startpage",
	"facebook.com":     "Facebook",
	"instagram.com":    "Instagram",
	"twitter.com":      "Twitter / X",
	"reddit.com":       "Reddit",
	"ycombinator.com":  "Hacker News",
	"t.me":             "Telegram",
	"web.telegram.org": "Telegram",
	"wikipedia.org":    "Wikipedia",
	"es.wikipedia.org": "Wikipedia",
	"mastodon.social":  "Mastodon",
	"chatgpt.com":      "ChatGPT",
	"perplexity.ai":    "Perplexity",
}

// normalizeReferrer devuelve el host canónico del referrer. Devuelve
// false si no hay referrer, si es inválido o si viene del propio sitio
// (la navegación interna no es "origen del tráfico").
func normalizeReferrer(raw string, selfHosts ...string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}

	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return "", false
	}

	for _, self := range selfHosts {
		self = strings.ToLower(strings.TrimPrefix(self, "www."))
		if host == self {
			return "", false
		}
	}

	// google.com.mx, google.es, google.co.uk → google.com
	if strings.HasPrefix(host, "google.") {
		host = "google.com"
	}

	if canonical, ok := canonicalHosts[host]; ok {
		host = canonical
	}
	return host, true
}

// referrerLabel es el nombre que se muestra en el dashboard.
func referrerLabel(host string) string {
	if name, ok := friendlyNames[host]; ok {
		return name
	}
	return host
}

// selfHosts son los dominios que cuentan como "el propio sitio": el de
// la petición y el configurado como público.
func (app *application) selfHosts(r *http.Request) []string {
	hosts := []string{}
	if h := r.Host; h != "" {
		if host, _, found := strings.Cut(h, ":"); found {
			hosts = append(hosts, host)
		} else {
			hosts = append(hosts, h)
		}
	}
	if u, err := url.Parse(app.config.baseURL); err == nil && u.Hostname() != "" {
		hosts = append(hosts, u.Hostname())
	}
	return hosts
}
