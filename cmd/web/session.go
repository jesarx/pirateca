package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sesión mínima para un solo admin: cookie firmada con HMAC-SHA256 que
// contiene solo la fecha de expiración. Sin estado en el servidor. Si el
// secret no se configura se genera uno aleatorio al arrancar (las
// sesiones no sobreviven reinicios en ese caso).
const sessionCookieName = "pirateca_session"
const sessionTTL = 7 * 24 * time.Hour

func (app *application) signSession(expiry int64) string {
	payload := strconv.FormatInt(expiry, 10)
	mac := hmac.New(sha256.New, app.sessionSecret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

func (app *application) setSessionCookie(w http.ResponseWriter) {
	expiry := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    app.signSession(expiry.Unix()),
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		Secure:   app.config.env == "production",
		SameSite: http.SameSiteLaxMode,
	})
}

func (app *application) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   app.config.env == "production",
		SameSite: http.SameSiteLaxMode,
	})
}

func (app *application) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}

	payload, _, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return false
	}
	expiry, err := strconv.ParseInt(payload, 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}

	expected := app.signSession(expiry)
	return hmac.Equal([]byte(cookie.Value), []byte(expected))
}

func decodeSessionSecret(s string) ([]byte, error) {
	if len(s) < 32 {
		return nil, fmt.Errorf("session secret must be at least 32 characters")
	}
	return []byte(s), nil
}
