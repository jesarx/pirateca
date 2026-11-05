package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// ---- Info hash de un .torrent ----
//
// Transmission identifica cada torrent por su info hash: el SHA-1 de los
// bytes EXACTOS del diccionario "info" del archivo .torrent (bencode).
// Con él se le pide que deje de sembrar ese torrent y solo ese.

// torrentInfoHash lee un .torrent y devuelve su info hash en hexadecimal
// (40 caracteres, minúsculas), el mismo que muestra transmission-show.
func torrentInfoHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return infoHashFromBytes(data)
}

func infoHashFromBytes(data []byte) (string, error) {
	if len(data) == 0 || data[0] != 'd' {
		return "", errors.New("torrent: no es un diccionario bencode")
	}
	i := 1
	for i < len(data) && data[i] != 'e' {
		key, next, err := bencodeString(data, i)
		if err != nil {
			return "", err
		}
		end, err := bencodeSkip(data, next)
		if err != nil {
			return "", err
		}
		if key == "info" {
			if data[next] != 'd' {
				return "", errors.New("torrent: info no es un diccionario")
			}
			sum := sha1.Sum(data[next:end])
			return hex.EncodeToString(sum[:]), nil
		}
		i = end
	}
	return "", errors.New("torrent: no tiene diccionario info")
}

// bencodeString lee una cadena "<len>:<bytes>" que empieza en i.
func bencodeString(data []byte, i int) (string, int, error) {
	colon := bytes.IndexByte(data[i:], ':')
	if colon <= 0 {
		return "", 0, errors.New("torrent: cadena bencode inválida")
	}
	n, err := strconv.Atoi(string(data[i : i+colon]))
	if err != nil || n < 0 {
		return "", 0, errors.New("torrent: longitud bencode inválida")
	}
	start := i + colon + 1
	if start+n > len(data) {
		return "", 0, errors.New("torrent: cadena bencode truncada")
	}
	return string(data[start : start+n]), start + n, nil
}

// bencodeSkip devuelve la posición justo después del valor que empieza
// en i (entero, cadena, lista o diccionario).
func bencodeSkip(data []byte, i int) (int, error) {
	return bencodeSkipDepth(data, i, 0)
}

func bencodeSkipDepth(data []byte, i, depth int) (int, error) {
	if depth > 64 {
		return 0, errors.New("torrent: anidamiento bencode excesivo")
	}
	if i >= len(data) {
		return 0, errors.New("torrent: bencode truncado")
	}
	switch c := data[i]; {
	case c == 'i':
		end := bytes.IndexByte(data[i:], 'e')
		if end < 0 {
			return 0, errors.New("torrent: entero bencode sin terminar")
		}
		return i + end + 1, nil
	case c == 'l' || c == 'd':
		i++
		for i < len(data) && data[i] != 'e' {
			next, err := bencodeSkipDepth(data, i, depth+1)
			if err != nil {
				return 0, err
			}
			i = next
		}
		if i >= len(data) {
			return 0, errors.New("torrent: lista/diccionario bencode sin terminar")
		}
		return i + 1, nil
	case c >= '0' && c <= '9':
		_, next, err := bencodeString(data, i)
		return next, err
	default:
		return 0, fmt.Errorf("torrent: byte bencode inesperado %q", c)
	}
}

// ---- Cliente RPC de transmission-daemon ----
//
// Es el mismo protocolo que usa transmission-remote: POST de JSON a
// /transmission/rpc, con la cabecera X-Transmission-Session-Id que el
// daemon entrega en una primera respuesta 409 (protección CSRF).

type transmissionClient struct {
	url  string
	user string
	pass string

	httpClient *http.Client

	mu        sync.Mutex
	sessionID string
}

func newTransmissionClient(url, user, pass string) *transmissionClient {
	if url == "" || url == "off" {
		return nil
	}
	return &transmissionClient{
		url:        url,
		user:       user,
		pass:       pass,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type transmissionTorrent struct {
	HashString  string  `json:"hashString"`
	Name        string  `json:"name"`
	Status      int     `json:"status"`
	PercentDone float64 `json:"percentDone"`
	Error       int     `json:"error"`
	ErrorString string  `json:"errorString"`
}

// StatusLabel traduce el estado numérico de Transmission.
func (t transmissionTorrent) StatusLabel() string {
	switch t.Status {
	case 0:
		return "detenido"
	case 1, 2:
		return "verificando"
	case 3, 4:
		return "descargando"
	case 5:
		return "en cola para sembrar"
	case 6:
		return "sembrando"
	}
	return "desconocido"
}

func (c *transmissionClient) call(ctx context.Context, method string, args, result any) error {
	body, err := json.Marshal(map[string]any{"method": method, "arguments": args})
	if err != nil {
		return err
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.user != "" || c.pass != "" {
			req.SetBasicAuth(c.user, c.pass)
		}
		c.mu.Lock()
		if c.sessionID != "" {
			req.Header.Set("X-Transmission-Session-Id", c.sessionID)
		}
		c.mu.Unlock()

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("no se pudo conectar con transmission (%s): %w", c.url, err)
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusConflict:
			// Primer contacto o sesión vencida: guardar el id y repetir.
			c.mu.Lock()
			c.sessionID = resp.Header.Get("X-Transmission-Session-Id")
			c.mu.Unlock()
			continue
		case http.StatusUnauthorized:
			return errors.New("transmission rechazó el usuario/contraseña (revisa PIRATECA_TRANSMISSION_USER/PASS)")
		case http.StatusOK:
		default:
			return fmt.Errorf("transmission respondió %s", resp.Status)
		}

		var envelope struct {
			Result    string          `json:"result"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(respBody, &envelope); err != nil {
			return fmt.Errorf("respuesta inválida de transmission: %w", err)
		}
		if envelope.Result != "success" {
			return fmt.Errorf("transmission: %s", envelope.Result)
		}
		if result != nil && len(envelope.Arguments) > 0 {
			return json.Unmarshal(envelope.Arguments, result)
		}
		return nil
	}
	return errors.New("transmission no aceptó el id de sesión")
}

// findTorrent busca un torrent por info hash. Devuelve nil si no está
// cargado en transmission.
func (c *transmissionClient) findTorrent(ctx context.Context, hash string) (*transmissionTorrent, error) {
	var out struct {
		Torrents []transmissionTorrent `json:"torrents"`
	}
	err := c.call(ctx, "torrent-get", map[string]any{
		"ids":    []string{hash},
		"fields": []string{"hashString", "name", "status", "percentDone", "error", "errorString"},
	}, &out)
	if err != nil {
		return nil, err
	}
	for _, t := range out.Torrents {
		if t.HashString == hash {
			return &t, nil
		}
	}
	return nil, nil
}

// removeTorrent quita de transmission el torrent con ese info hash (y
// solo ese) SIN borrar datos: los archivos los borra la app, que sabe
// exactamente cuáles son. Después confirma que ya no está.
func (c *transmissionClient) removeTorrent(ctx context.Context, hash string) error {
	err := c.call(ctx, "torrent-remove", map[string]any{
		"ids":               []string{hash},
		"delete-local-data": false,
	}, nil)
	if err != nil {
		return err
	}
	still, err := c.findTorrent(ctx, hash)
	if err != nil {
		return fmt.Errorf("no se pudo confirmar el retiro: %w", err)
	}
	if still != nil {
		return errors.New("transmission sigue mostrando el torrent después de quitarlo")
	}
	return nil
}
