package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Trackers públicos para los torrents (mismos que usaba la API vieja).
var torrentTrackers = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.demonii.com:1337/announce",
	"udp://tracker.torrent.eu.org:451/announce",
}

var nonAlphanumRe = regexp.MustCompile("[^a-zA-Z0-9 ]")

// cleanString normaliza un texto para usarlo en nombres de archivo:
// quita diacríticos, elimina todo lo no alfanumérico y cambia espacios
// por guiones bajos. Idéntico al CleanString del código viejo para que
// los nombres nuevos sigan la misma convención que los ~300 existentes.
func cleanString(str string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, str)
	result := nonAlphanumRe.ReplaceAllString(normalized, "")
	return strings.ReplaceAll(result, " ", "_")
}

// baseFilename arma el nombre base de los archivos de un libro:
// Apellido_Nombre-Titulo_Corto (o Apellido-Titulo_Corto sin nombre).
func baseFilename(authorName, authorLastName, shortTitle string) string {
	if authorName != "" {
		return fmt.Sprintf("%s_%s-%s", cleanString(authorLastName), cleanString(authorName), cleanString(shortTitle))
	}
	return fmt.Sprintf("%s-%s", cleanString(authorLastName), cleanString(shortTitle))
}

// sanitizeMetadataValue quita caracteres de control para evitar inyección
// de argumentos en exiftool/transmission-create.
func sanitizeMetadataValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
}

func runTool(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w, output: %s", name, err, string(out))
	}
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}
	return destFile.Sync()
}

func (app *application) uploadPath(parts ...string) string {
	return filepath.Join(append([]string{app.config.uploadsDir}, parts...)...)
}

func (app *application) ensureUploadDirs() error {
	for _, dir := range []string{"pdfs", "covers", "torrents", "torrentadded"} {
		if err := os.MkdirAll(app.uploadPath(dir), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// saveFormFile guarda el archivo del campo multipart en dst, validando la
// extensión del archivo original. Devuelve os.ErrNotExist si el campo
// viene vacío. El archivo queda CERRADO al regresar, listo para que las
// herramientas externas lo lean.
func saveFormFile(r *http.Request, field, dst string, allowedExts []string) error {
	file, header, err := r.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return os.ErrNotExist
		}
		return err
	}
	defer file.Close()

	if _, err := safeFileName(header.Filename, allowedExts); err != nil {
		return fmt.Errorf("campo %s: %w", field, err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		return fmt.Errorf("failed to save %s: %w", dst, err)
	}
	return out.Close()
}

// processPDF ejecuta el pipeline completo del PDF. EL ORDEN IMPORTA:
//
//  1. Guardar el PDF subido como uploads/pdfs/{base}.pdf (y cerrarlo).
//  2. exiftool -all:all=      → borra TODOS los metadatos originales.
//  3. exiftool -Title/-Author/-Publisher → escribe los metadatos limpios.
//  4. transmission-create     → genera el .torrent. Tiene que ser DESPUÉS
//     de fijar metadatos: el torrent hashea el contenido del PDF y
//     cualquier modificación posterior lo invalidaría.
//  5. Copiar el .torrent a uploads/torrentadded/ (carpeta que vigila el
//     cliente de torrents del VPS para empezar a sembrar).
//
// Devuelve las rutas escritas (para poder limpiarlas si algo posterior
// falla) aunque regrese error.
func (app *application) processPDF(r *http.Request, base, title, authorFull, publisherName string) (written []string, err error) {
	pdfPath := app.uploadPath("pdfs", base+".pdf")
	torrentPath := app.uploadPath("torrents", base+".pdf.torrent")
	torrentAddedPath := app.uploadPath("torrentadded", base+".pdf.torrent")

	// 1. Guardar (saveFormFile cierra el archivo antes de regresar).
	err = saveFormFile(r, "pdf", pdfPath, []string{".pdf"})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // sin PDF: no es error, el campo es opcional
		}
		return nil, err
	}
	written = append(written, pdfPath)

	// 2. Borrar todos los metadatos originales.
	if err := runTool("exiftool", "-overwrite_original", "-all:all=", pdfPath); err != nil {
		return written, err
	}

	// 3. Escribir los metadatos limpios.
	safeTitle := sanitizeMetadataValue(title)
	safeAuthor := sanitizeMetadataValue(authorFull)
	safePublisher := sanitizeMetadataValue(publisherName)
	if err := runTool("exiftool",
		"-overwrite_original",
		"-charset", "exif=UTF8",
		"-Title="+safeTitle,
		"-Author="+safeAuthor,
		"-Publisher="+safePublisher,
		pdfPath,
	); err != nil {
		return written, err
	}

	// 4. Crear el torrent (con el PDF ya en su forma definitiva).
	args := []string{
		"-o", torrentPath,
		"-c", sanitizeMetadataValue(fmt.Sprintf("%s by %s", title, authorFull)),
	}
	for _, tracker := range torrentTrackers {
		args = append(args, "--tracker", tracker)
	}
	args = append(args, pdfPath)
	if err := runTool("transmission-create", args...); err != nil {
		return written, err
	}
	written = append(written, torrentPath)

	// 5. Copiar a la carpeta vigilada para sembrar.
	if err := copyFile(torrentPath, torrentAddedPath); err != nil {
		return written, fmt.Errorf("failed to copy torrent to torrentadded: %w", err)
	}
	written = append(written, torrentAddedPath)

	return written, nil
}

// processCover guarda la portada como uploads/covers/{base}.jpg:
// si no viene en JPG la convierte con ImageMagick, y al final borra los
// metadatos con exiftool. Es el mismo pipeline en creación y edición.
func (app *application) processCover(r *http.Request, base string) (written []string, err error) {
	coverPath := app.uploadPath("covers", base+".jpg")

	file, header, err := r.FormFile("image")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, nil // sin portada: el campo es opcional
		}
		return nil, err
	}
	defer file.Close()

	cleanName, err := safeFileName(header.Filename, imageExts)
	if err != nil {
		return nil, fmt.Errorf("campo image: %w", err)
	}
	origExt := strings.ToLower(filepath.Ext(cleanName))

	if origExt == ".jpg" || origExt == ".jpeg" {
		out, err := os.Create(coverPath)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(out, file); err != nil {
			out.Close()
			return nil, fmt.Errorf("failed to save image: %w", err)
		}
		if err := out.Close(); err != nil {
			return nil, err
		}
	} else {
		// Guardar temporal con la extensión original y convertir a JPG.
		tempFile, err := os.CreateTemp("", "cover-*"+origExt)
		if err != nil {
			return nil, err
		}
		tempPath := tempFile.Name()
		defer os.Remove(tempPath)

		if _, err := io.Copy(tempFile, file); err != nil {
			tempFile.Close()
			return nil, fmt.Errorf("failed to save temp image: %w", err)
		}
		if err := tempFile.Close(); err != nil {
			return nil, err
		}

		if err := runTool("convert", tempPath, coverPath); err != nil {
			return nil, err
		}
	}
	written = append(written, coverPath)

	// Borrar metadatos de la imagen final.
	if err := runTool("exiftool", "-overwrite_original", "-all:all=", coverPath); err != nil {
		return written, err
	}

	return written, nil
}

// removeFiles borra rutas en best-effort (para limpiar archivos huérfanos
// cuando falla un paso posterior, o al borrar un libro).
func (app *application) removeFiles(paths []string) {
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			app.logger.Error("failed to remove file", "path", p, "error", err.Error())
		}
	}
}

// bookFilePaths devuelve las rutas de todos los archivos asociados a un
// nombre base (las mismas que borraba el API viejo al eliminar un libro).
func (app *application) bookFilePaths(base string) []string {
	return []string{
		app.uploadPath("pdfs", base+".pdf"),
		app.uploadPath("covers", base+".jpg"),
		app.uploadPath("torrents", base+".pdf.torrent"),
		app.uploadPath("torrentadded", base+".pdf.torrent"),
	}
}
