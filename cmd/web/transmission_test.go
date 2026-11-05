package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTorrentInfoHash(t *testing.T) {
	// testdata/sample.torrent lo generó transmission-create con el mismo
	// pipeline de la app; el hash esperado es el que reporta
	// transmission-show (y transmission-daemon) para ese archivo.
	got, err := torrentInfoHash("testdata/sample.torrent")
	if err != nil {
		t.Fatal(err)
	}
	if want := "f687c417128f5ed05eac1915bc7a3460b285e4f6"; got != want {
		t.Fatalf("info hash = %s, want %s", got, want)
	}
}

func TestInfoHashFromBytesRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "no soy torrent", "d4:infoi1e", "d8:announce3:abce", "d4:info5:abc"} {
		if _, err := infoHashFromBytes([]byte(in)); err == nil {
			t.Errorf("%q: se esperaba error", in)
		}
	}
}

// fakeTransmission imita el RPC de transmission-daemon: exige el id de
// sesión (respondiendo 409 la primera vez) y autenticación básica.
func fakeTransmission(t *testing.T, torrents map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Transmission-Session-Id") != "sid123" {
			w.Header().Set("X-Transmission-Session-Id", "sid123")
			w.WriteHeader(http.StatusConflict)
			return
		}
		var req struct {
			Method    string `json:"method"`
			Arguments struct {
				IDs             []string `json:"ids"`
				DeleteLocalData bool     `json:"delete-local-data"`
			} `json:"arguments"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "torrent-get":
			var list []map[string]any
			for _, id := range req.Arguments.IDs {
				if name, ok := torrents[id]; ok {
					list = append(list, map[string]any{"hashString": id, "name": name, "status": 6, "percentDone": 1})
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"result": "success", "arguments": map[string]any{"torrents": list}})
		case "torrent-remove":
			if req.Arguments.DeleteLocalData {
				t.Error("torrent-remove NO debe pedir borrar datos")
			}
			for _, id := range req.Arguments.IDs {
				delete(torrents, id)
			}
			json.NewEncoder(w).Encode(map[string]any{"result": "success"})
		default:
			json.NewEncoder(w).Encode(map[string]any{"result": "method name not recognized"})
		}
	}))
}

func TestTransmissionRemoveOnlyThatTorrent(t *testing.T) {
	torrents := map[string]string{
		"aaaa": "Uno.pdf",
		"bbbb": "Dos.pdf",
	}
	srv := fakeTransmission(t, torrents)
	defer srv.Close()

	c := newTransmissionClient(srv.URL, "u", "p")
	ctx := context.Background()

	got, err := c.findTorrent(ctx, "aaaa")
	if err != nil || got == nil || got.Name != "Uno.pdf" || got.StatusLabel() != "sembrando" {
		t.Fatalf("findTorrent = %+v, %v", got, err)
	}
	if err := c.removeTorrent(ctx, "aaaa"); err != nil {
		t.Fatal(err)
	}
	if _, ok := torrents["aaaa"]; ok {
		t.Fatal("el torrent sigue cargado")
	}
	if _, ok := torrents["bbbb"]; !ok {
		t.Fatal("se quitó un torrent que no debía")
	}
	missing, err := c.findTorrent(ctx, "aaaa")
	if err != nil || missing != nil {
		t.Fatalf("después de quitarlo: %+v, %v", missing, err)
	}
}

func TestTransmissionBadCredentials(t *testing.T) {
	srv := fakeTransmission(t, map[string]string{})
	defer srv.Close()

	c := newTransmissionClient(srv.URL, "u", "mala")
	_, err := c.findTorrent(context.Background(), "aaaa")
	if err == nil || !strings.Contains(err.Error(), "usuario/contraseña") {
		t.Fatalf("err = %v", err)
	}
}

func TestTransmissionDisabled(t *testing.T) {
	if newTransmissionClient("off", "", "") != nil || newTransmissionClient("", "", "") != nil {
		t.Fatal("con url vacía u \"off\" no debe haber cliente")
	}
}

func TestValidBase(t *testing.T) {
	ok := []string{"Garcia_Marquez_Gabriel-Cien_anos", "Debord-Sociedad"}
	bad := []string{"", ".", "..", "../etc/passwd", "a/b", `a\b`, "x..y"}
	for _, s := range ok {
		if !validBase(s) {
			t.Errorf("%q debería ser válido", s)
		}
	}
	for _, s := range bad {
		if validBase(s) {
			t.Errorf("%q NO debería ser válido", s)
		}
	}
}
