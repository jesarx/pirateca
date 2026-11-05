package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jesarx/pirateca/internal/store"
)

// Borrado de un libro "de todos lados":
//
//   - la fila en la base de datos y su historial de descargas;
//   - sus archivos: PDF, portada, EPUB y .torrent;
//   - el torrent en transmission-daemon (deja de sembrarse).
//
// Todo se ubica por el nombre base EXACTO del libro (books.filename, que
// es único), así que no se toca nada de otro libro. Autores y editorial
// se conservan porque pueden tener otros libros.
//
// El orden importa:
//
//  1. Calcular el info hash del .torrent ANTES de borrar nada.
//  2. Borrar en la base (en una transacción). Si falla, se aborta sin
//     haber tocado ningún archivo.
//  3. Quitar el .torrent de la carpeta vigilada (torrentadded/): si
//     siguiera ahí, transmission podría volver a agregarlo.
//  4. Pedirle a transmission que quite ese torrent (por info hash, sin
//     borrar datos).
//  5. Borrar los archivos del libro.

// bookFile es un archivo que pertenece a un libro.
type bookFile struct {
	Label  string
	Path   string // ruta absoluta o relativa a uploads/
	Rel    string // para mostrar: subcarpeta/archivo
	Exists bool
}

type deletePlan struct {
	Book      *store.Book
	Files     []bookFile // los que se borran al final
	Watched   []bookFile // copias en la carpeta vigilada por transmission
	InfoHash  string
	HashError string
	Torrent   *transmissionTorrent // estado actual en transmission (nil = no cargado)
	TorrentOK bool                 // se pudo consultar transmission
	TorrentNA string               // por qué no se pudo consultar
	Downloads int64
}

type deleteStep struct {
	Label  string
	Detail string
	OK     bool
	Warn   bool
}

type deleteView struct {
	Plan   *deletePlan
	Done   bool
	Steps  []deleteStep
	Manual string // comando para hacer a mano lo que no se pudo
}

func (app *application) planBookDelete(ctx context.Context, book *store.Book) *deletePlan {
	plan := &deletePlan{Book: book}

	if validBase(book.Filename) {
		base := book.Filename
		add := func(list *[]bookFile, label string, parts ...string) {
			path := app.uploadPath(parts...)
			_, err := os.Lstat(path)
			*list = append(*list, bookFile{
				Label:  label,
				Path:   path,
				Rel:    parts[0] + "/" + parts[1],
				Exists: err == nil,
			})
		}
		add(&plan.Files, "PDF", "pdfs", base+".pdf")
		add(&plan.Files, "Portada", "covers", base+".jpg")
		add(&plan.Files, "EPUB", "epubs", base+".epub")
		add(&plan.Files, "Torrent (descarga)", "torrents", base+".pdf.torrent")
		// transmission renombra a .added lo que ya agregó desde la
		// carpeta vigilada; se borran ambas variantes.
		add(&plan.Watched, "Torrent (carpeta vigilada)", "torrentadded", base+".pdf.torrent")
		add(&plan.Watched, "Torrent (carpeta vigilada, ya agregado)", "torrentadded", base+".pdf.torrent.added")

		// El info hash sale del primer .torrent disponible.
		for _, f := range append(append([]bookFile{}, plan.Files[3]), plan.Watched...) {
			if !f.Exists {
				continue
			}
			hash, err := torrentInfoHash(f.Path)
			if err != nil {
				plan.HashError = fmt.Sprintf("%s: %v", f.Rel, err)
				continue
			}
			plan.InfoHash, plan.HashError = hash, ""
			break
		}
	}

	if plan.InfoHash != "" {
		if app.transmission == nil {
			plan.TorrentNA = "la conexión con transmission está desactivada (PIRATECA_TRANSMISSION_URL=off)"
		} else {
			tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			t, err := app.transmission.findTorrent(tctx, plan.InfoHash)
			cancel()
			if err != nil {
				plan.TorrentNA = err.Error()
			} else {
				plan.TorrentOK = true
				plan.Torrent = t
			}
		}
	}

	if book.Filename != "" {
		if n, err := app.store.DownloadsForFile(ctx, book.Filename); err == nil {
			plan.Downloads = n
		}
	}
	return plan
}

// deleteBookConfirmHandler muestra exactamente qué se va a borrar.
func (app *application) deleteBookConfirmHandler(w http.ResponseWriter, r *http.Request) {
	book, ok := app.bookFromPath(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Form = deleteView{Plan: app.planBookDelete(r.Context(), book)}
	app.render(w, r, http.StatusOK, "book-delete.html", data)
}

func (app *application) deleteBookHandler(w http.ResponseWriter, r *http.Request) {
	book, ok := app.bookFromPath(w, r)
	if !ok {
		return
	}
	plan := app.planBookDelete(r.Context(), book)
	view := deleteView{Plan: plan, Done: true}
	step := func(label, detail string, ok, warn bool) {
		view.Steps = append(view.Steps, deleteStep{Label: label, Detail: detail, OK: ok, Warn: warn})
	}

	// 2. Base de datos. Si falla, no se ha tocado ningún archivo.
	if err := app.store.DeleteBook(r.Context(), book.ID, book.Filename); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}
	detail := "Se borró el registro del libro."
	if plan.Downloads > 0 {
		detail = fmt.Sprintf("Se borró el registro del libro y su historial de %d descargas.", plan.Downloads)
	}
	step("Base de datos", detail, true, false)
	if book.Filename != "" {
		app.downloads.forget(book.Filename)
		coverSizes.Delete(book.Filename)
	}

	// 3. Carpeta vigilada primero, para que transmission no lo re-agregue.
	removeStep := func(f bookFile) {
		if !f.Exists {
			return
		}
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			app.logger.Error("failed to remove file", "path", f.Path, "error", err.Error())
			step(f.Label, fmt.Sprintf("No se pudo borrar %s: %v", f.Rel, err), false, true)
			return
		}
		step(f.Label, "Borrado: "+f.Rel, true, false)
	}
	for _, f := range plan.Watched {
		removeStep(f)
	}

	// 4. Transmission.
	switch {
	case plan.InfoHash == "":
		if plan.HashError != "" {
			step("Transmission", "No se pudo leer el .torrent ("+plan.HashError+"); si seguía sembrando, al borrar el PDF dejará de hacerlo.", false, true)
		} else {
			step("Transmission", "El libro no tenía .torrent: no había nada que dejar de sembrar.", true, false)
		}
	case app.transmission == nil:
		step("Transmission", "Conexión desactivada: quita el torrent a mano (comando abajo).", false, true)
		view.Manual = "transmission-remote -t " + plan.InfoHash + " -r"
	default:
		tctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		t, err := app.transmission.findTorrent(tctx, plan.InfoHash)
		if err == nil && t == nil {
			step("Transmission", "El torrent no estaba cargado en transmission (no se estaba sembrando).", true, false)
		} else if err == nil {
			err = app.transmission.removeTorrent(tctx, plan.InfoHash)
			if err == nil {
				step("Transmission", "Se dejó de sembrar «"+t.Name+"» (info hash "+plan.InfoHash+").", true, false)
			}
		}
		cancel()
		if err != nil {
			app.logger.Error("transmission remove failed", "hash", plan.InfoHash, "error", err.Error())
			step("Transmission", "No se pudo quitar el torrent: "+err.Error()+". Al no existir el PDF ya no puede sembrarse, pero conviene quitarlo a mano (comando abajo).", false, true)
			view.Manual = "transmission-remote -t " + plan.InfoHash + " -r"
		}
	}

	// 5. Archivos del libro.
	for _, f := range plan.Files {
		removeStep(f)
	}

	app.logger.Info("book deleted", "id", book.ID, "slug", book.Slug, "filename", book.Filename, "infohash", plan.InfoHash)

	data := app.newTemplateData(r)
	data.Form = view
	app.render(w, r, http.StatusOK, "book-delete.html", data)
}
