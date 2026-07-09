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

func (app *application) serveUpload(w http.ResponseWriter, r *http.Request, subdir string, allowedExts []string, download bool) {
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
			app.notFound(w)
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

	http.ServeContent(w, r, fileName, fileInfo.ModTime(), file)
}

var imageExts = []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}

func (app *application) serveCover(w http.ResponseWriter, r *http.Request) {
	app.serveUpload(w, r, "covers", imageExts, false)
}
