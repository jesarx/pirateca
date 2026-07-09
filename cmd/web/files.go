package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// safeFileName limpia el nombre recibido por query param y valida la
// extensión, evitando path traversal (portado de la API vieja).
func safeFileName(name string, allowedExts []string) (string, error) {
	clean := filepath.Base(filepath.Clean(name))
	if clean == "." || clean == "/" || clean == "" {
		return "", fmt.Errorf("invalid filename")
	}
	ext := strings.ToLower(filepath.Ext(clean))
	for _, allowed := range allowedExts {
		if ext == allowed {
			return clean, nil
		}
	}
	return "", fmt.Errorf("file type not allowed")
}

// serveUpload sirve un archivo de uploads/. Si onServe no es nil, se
// llama con el nombre del archivo justo antes de servirlo (solo cuando
// el archivo existe), para conteos.
func (app *application) serveUpload(w http.ResponseWriter, r *http.Request, subdir string, allowedExts []string, download bool, onServe func(fileName string)) {
	rawName := r.URL.Query().Get("file")
	if rawName == "" {
		http.Error(w, "file parameter is required", http.StatusBadRequest)
		return
	}

	fileName, err := safeFileName(rawName, allowedExts)
	if err != nil {
		http.Error(w, "invalid file parameter", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(app.config.uploadsDir, subdir, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if download {
		safe := strings.ReplaceAll(fileName, `"`, `\"`)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safe))
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	if onServe != nil {
		onServe(fileName)
	}

	http.ServeContent(w, r, fileName, fileInfo.ModTime(), file)
}

var imageExts = []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}

func (app *application) serveCover(w http.ResponseWriter, r *http.Request) {
	app.serveUpload(w, r, "covers", imageExts, false, nil)
}

func (app *application) servePdf(w http.ResponseWriter, r *http.Request) {
	// Contar solo la petición inicial de una descarga real: los gestores
	// de descargas y navegadores piden el resto del archivo por rangos
	// (cabecera Range) y cada trozo NO es una descarga nueva. El callback
	// corre únicamente si el archivo existe.
	var onServe func(string)
	if rng := r.Header.Get("Range"); rng == "" || strings.HasPrefix(rng, "bytes=0-") {
		if !looksLikeBot(r.UserAgent()) {
			onServe = func(fileName string) {
				app.downloads.add(strings.TrimSuffix(fileName, ".pdf"))
			}
		}
	}
	app.serveUpload(w, r, "pdfs", []string{".pdf"}, true, onServe)
}

func (app *application) serveEpub(w http.ResponseWriter, r *http.Request) {
	app.serveUpload(w, r, "epubs", []string{".epub"}, true, nil)
}

func (app *application) serveTorrent(w http.ResponseWriter, r *http.Request) {
	app.serveUpload(w, r, "torrents", []string{".torrent"}, true, nil)
}
