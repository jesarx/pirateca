package main

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sync"

	"github.com/jesarx/pirateca/internal/store"
)

// coverSize es el tamaño en píxeles de una portada. Declararlo en el
// <img> hace que el navegador reserve el espacio exacto antes de
// descargar la imagen: evita saltos de layout y permite que el masonry
// mida bien las tarjetas desde el primer render.
type coverSize struct {
	Width  int
	Height int
}

type coverCacheEntry struct {
	size    coverSize
	modTime int64
	size64  int64
}

// coverSizes cachea las dimensiones ya leídas. Se revalida con el
// mtime/tamaño del archivo, así que reemplazar una portada desde el
// dashboard se refleja sin reiniciar el servicio.
var coverSizes sync.Map // filename -> coverCacheEntry

func (app *application) coverSize(filename string) (coverSize, bool) {
	if filename == "" {
		return coverSize{}, false
	}

	path := app.uploadPath("covers", filename+".jpg")
	info, err := os.Stat(path)
	if err != nil {
		return coverSize{}, false
	}

	if cached, ok := coverSizes.Load(filename); ok {
		entry := cached.(coverCacheEntry)
		if entry.modTime == info.ModTime().UnixNano() && entry.size64 == info.Size() {
			return entry.size, entry.size.Width > 0
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return coverSize{}, false
	}
	defer file.Close()

	// DecodeConfig lee solo la cabecera, no los píxeles.
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return coverSize{}, false
	}

	size := coverSize{Width: cfg.Width, Height: cfg.Height}
	coverSizes.Store(filename, coverCacheEntry{
		size:    size,
		modTime: info.ModTime().UnixNano(),
		size64:  info.Size(),
	})
	return size, size.Width > 0
}

// coverSizesFor arma el mapa filename -> tamaño para un listado.
func (app *application) coverSizesFor(books []store.Book) map[string]coverSize {
	sizes := make(map[string]coverSize, len(books))
	for _, b := range books {
		if b.Filename == "" {
			continue
		}
		if size, ok := app.coverSize(b.Filename); ok {
			sizes[b.Filename] = size
		}
	}
	return sizes
}
