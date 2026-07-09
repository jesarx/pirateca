package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/pirateca/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// ---- Login / logout ----

func (app *application) loginFormHandler(w http.ResponseWriter, r *http.Request) {
	if app.isAuthenticated(r) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	app.render(w, r, http.StatusOK, "login.html", app.newTemplateData(r))
}

func (app *application) loginHandler(w http.ResponseWriter, r *http.Request) {
	if app.store == nil {
		app.serverError(w, r, errNoDatabase)
		return
	}

	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")

	fail := func() {
		time.Sleep(500 * time.Millisecond) // frena fuerza bruta
		data := app.newTemplateData(r)
		data.Form = map[string]string{"Email": email, "Error": "Credenciales inválidas."}
		app.render(w, r, http.StatusUnprocessableEntity, "login.html", data)
	}

	user, err := app.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail()
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	if bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)) != nil {
		fail()
		return
	}

	app.setSessionCookie(w)
	app.logger.Info("admin login", "email", email)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (app *application) logoutHandler(w http.ResponseWriter, r *http.Request) {
	app.clearSessionCookie(w)
	http.Redirect(w, r, "/books", http.StatusSeeOther)
}

// ---- Dashboard: resumen con estadísticas ----

func (app *application) dashboardHomeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	catalog, err := app.store.GetCatalogStats(ctx)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	visits, err := app.store.GetVisitStats(ctx)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	downloads, err := app.store.GetDownloadStats(ctx)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	topDownloads, err := app.store.GetTopDownloads(ctx, 10)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	months, err := app.store.GetBooksPerMonth(ctx)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	tags, err := app.store.ListTags(ctx)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	latest, _, err := app.store.ListBooks(ctx, store.BookFilters{PageSize: 5})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Dash = &dashData{
		Catalog:      catalog,
		Visits:       visits,
		Downloads:    downloads,
		TopDownloads: topDownloads,
		VisitBars:    buildVisitBars(visits.Daily),
		MonthBars:    buildMonthBars(months),
		TagBars:      buildTagBars(tags, 10),
		LatestBooks:  latest,
	}
	app.render(w, r, http.StatusOK, "dashboard-home.html", data)
}

// ---- Dashboard: libros ----

func (app *application) dashboardBooksHandler(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	filters := bookFiltersFromQuery(qs)
	if filters.Sort == "" {
		filters.Sort = "-created_at"
	}

	books, metadata, err := app.store.ListBooks(r.Context(), filters)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Books = books
	data.Metadata = metadata
	data.Filters = filters
	data.Pagination = buildPagination(metadata, r.URL.Path, qs)
	app.render(w, r, http.StatusOK, "dashboard-books.html", data)
}

// bookForm carga los valores del formulario de libro (crear/editar).
type bookForm struct {
	Title          string
	ShortTitle     string
	Year           int
	Tags           []string
	TagsRaw        string
	AuthorName     string
	AuthorLastName string
	Author2Name    string
	Author2LastNm  string
	PublisherName  string
	ISBN           string
	Description    string
	Pages          int
	ExternalLink   string
	DirDwl         bool
	Errors         map[string]string
	// Solo en edición:
	ID       int64
	Version  int
	Filename string
	Slug     string
}

func parseBookForm(r *http.Request) bookForm {
	f := bookForm{
		Title:          strings.TrimSpace(r.PostFormValue("title")),
		ShortTitle:     strings.TrimSpace(r.PostFormValue("short_title")),
		TagsRaw:        strings.TrimSpace(r.PostFormValue("tags")),
		AuthorName:     strings.TrimSpace(r.PostFormValue("author_name")),
		AuthorLastName: strings.TrimSpace(r.PostFormValue("author_last_name")),
		Author2Name:    strings.TrimSpace(r.PostFormValue("author2_name")),
		Author2LastNm:  strings.TrimSpace(r.PostFormValue("author2_last_name")),
		PublisherName:  strings.TrimSpace(r.PostFormValue("publisher_name")),
		ISBN:           strings.TrimSpace(r.PostFormValue("isbn")),
		Description:    strings.TrimSpace(r.PostFormValue("description")),
		ExternalLink:   strings.TrimSpace(r.PostFormValue("external_link")),
		DirDwl:         r.PostFormValue("dir_dwl") == "on",
		Errors:         map[string]string{},
	}
	f.Year, _ = strconv.Atoi(strings.TrimSpace(r.PostFormValue("year")))
	f.Pages, _ = strconv.Atoi(strings.TrimSpace(r.PostFormValue("pages")))

	for _, t := range strings.Split(f.TagsRaw, ",") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			f.Tags = append(f.Tags, t)
		}
	}
	return f
}

func (f *bookForm) validate() bool {
	if f.Title == "" {
		f.Errors["title"] = "El título es obligatorio."
	}
	if f.ShortTitle == "" {
		f.Errors["short_title"] = "El título corto es obligatorio."
	}
	if f.Year < 1000 || f.Year > time.Now().Year() {
		f.Errors["year"] = "Año inválido."
	}
	if len(f.Tags) < 1 || len(f.Tags) > 5 {
		f.Errors["tags"] = "Debe haber entre 1 y 5 etiquetas (separadas por coma)."
	} else {
		seen := map[string]bool{}
		for _, t := range f.Tags {
			if seen[t] {
				f.Errors["tags"] = "Hay etiquetas repetidas."
			}
			seen[t] = true
		}
	}
	if f.AuthorLastName == "" {
		f.Errors["author_last_name"] = "El apellido del autor es obligatorio."
	}
	if f.Author2Name != "" && f.Author2LastNm == "" {
		f.Errors["author2_last_name"] = "Falta el apellido del segundo autor."
	}
	if f.PublisherName == "" {
		f.Errors["publisher_name"] = "La editorial es obligatoria."
	}
	return len(f.Errors) == 0
}

func (app *application) newBookFormHandler(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = bookForm{DirDwl: true, Errors: map[string]string{}}
	app.render(w, r, http.StatusOK, "book-form.html", data)
}

func (app *application) createBookHandler(w http.ResponseWriter, r *http.Request) {
	// 32 MB en memoria; el resto se va a archivos temporales.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		app.badRequest(w, "formulario inválido: "+err.Error())
		return
	}

	form := parseBookForm(r)
	renderForm := func(status int) {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, status, "book-form.html", data)
	}

	if !form.validate() {
		renderForm(http.StatusUnprocessableEntity)
		return
	}

	// 1. Resolver autor(es) y editorial (se crean si no existen), porque
	//    el nombre base de los archivos depende del autor principal.
	author, err := app.store.GetOrCreateAuthor(r.Context(), form.AuthorName, form.AuthorLastName)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	var author2ID *int64
	if form.Author2LastNm != "" {
		author2, err := app.store.GetOrCreateAuthor(r.Context(), form.Author2Name, form.Author2LastNm)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		author2ID = &author2.ID
	}

	publisher, err := app.store.GetOrCreatePublisher(r.Context(), form.PublisherName)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// 2. Calcular el nombre base y verificar que esté libre ANTES de
	//    escribir cualquier archivo (el API viejo no lo hacía y dejaba
	//    archivos huérfanos si el INSERT chocaba con el UNIQUE).
	base := baseFilename(author.Name, author.LastName, form.ShortTitle)
	exists, err := app.store.FilenameExists(r.Context(), base)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if exists {
		form.Errors["short_title"] = fmt.Sprintf("Ya existe un libro con el nombre de archivo %q (mismo autor y título corto).", base)
		renderForm(http.StatusUnprocessableEntity)
		return
	}

	if err := app.ensureUploadDirs(); err != nil {
		app.serverError(w, r, err)
		return
	}

	// 3. Pipeline del PDF (guardar → limpiar metadatos → escribir
	//    metadatos → torrent → torrentadded). Ver processPDF.
	var written []string
	pdfWritten, err := app.processPDF(r, base, form.ShortTitle, author.FullName(), publisher.Name)
	written = append(written, pdfWritten...)
	if err != nil {
		app.removeFiles(written)
		app.serverError(w, r, err)
		return
	}

	// 4. Pipeline de la portada (guardar/convertir a JPG → limpiar
	//    metadatos).
	coverWritten, err := app.processCover(r, base)
	written = append(written, coverWritten...)
	if err != nil {
		app.removeFiles(written)
		app.serverError(w, r, err)
		return
	}

	// 5. Insertar en la base; si falla, limpiar los archivos escritos.
	_, slug, err := app.store.InsertBook(r.Context(), store.BookInput{
		Title:        form.Title,
		ShortTitle:   form.ShortTitle,
		Year:         form.Year,
		Tags:         form.Tags,
		AuthorID:     author.ID,
		Author2ID:    author2ID,
		PublisherID:  publisher.ID,
		Filename:     base,
		ISBN:         form.ISBN,
		Description:  form.Description,
		Pages:        form.Pages,
		ExternalLink: form.ExternalLink,
		DirDwl:       form.DirDwl,
	})
	if err != nil {
		app.removeFiles(written)
		app.serverError(w, r, err)
		return
	}

	app.logger.Info("book created", "slug", slug, "filename", base)
	http.Redirect(w, r, "/books/"+slug, http.StatusSeeOther)
}

func (app *application) editBookFormHandler(w http.ResponseWriter, r *http.Request) {
	book, ok := app.bookFromPath(w, r)
	if !ok {
		return
	}

	form := bookForm{
		Title:          book.Title,
		ShortTitle:     book.ShortTitle,
		Year:           book.Year,
		Tags:           book.Tags,
		TagsRaw:        strings.Join(book.Tags, ", "),
		AuthorName:     book.AuthorName,
		AuthorLastName: book.AuthorLastName,
		PublisherName:  book.PublisherName,
		ISBN:           book.ISBN,
		Description:    book.Description,
		Pages:          book.Pages,
		ExternalLink:   book.ExternalLink,
		DirDwl:         book.DirDwl,
		Errors:         map[string]string{},
		ID:             book.ID,
		Version:        book.Version,
		Filename:       book.Filename,
		Slug:           book.Slug,
	}
	if book.Author2Name != nil {
		form.Author2Name = *book.Author2Name
	}
	if book.Author2LastName != nil {
		form.Author2LastNm = *book.Author2LastName
	}

	data := app.newTemplateData(r)
	data.Form = form
	app.render(w, r, http.StatusOK, "book-form.html", data)
}

func (app *application) updateBookHandler(w http.ResponseWriter, r *http.Request) {
	book, ok := app.bookFromPath(w, r)
	if !ok {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		app.badRequest(w, "formulario inválido: "+err.Error())
		return
	}

	form := parseBookForm(r)
	form.ID = book.ID
	form.Version = book.Version
	form.Filename = book.Filename
	form.Slug = book.Slug

	renderForm := func(status int) {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, status, "book-form.html", data)
	}

	if !form.validate() {
		renderForm(http.StatusUnprocessableEntity)
		return
	}

	author, err := app.store.GetOrCreateAuthor(r.Context(), form.AuthorName, form.AuthorLastName)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	var author2ID *int64
	if form.Author2LastNm != "" {
		author2, err := app.store.GetOrCreateAuthor(r.Context(), form.Author2Name, form.Author2LastNm)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		author2ID = &author2.ID
	}

	publisher, err := app.store.GetOrCreatePublisher(r.Context(), form.PublisherName)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// En edición el PDF es inmutable (su torrent ya circula); solo se
	// puede reemplazar la portada, conservando el nombre base.
	if book.Filename != "" {
		if _, err := app.processCover(r, book.Filename); err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	slug, err := app.store.UpdateBook(r.Context(), book.ID, book.Version, store.BookInput{
		Title:        form.Title,
		ShortTitle:   form.ShortTitle,
		Year:         form.Year,
		Tags:         form.Tags,
		AuthorID:     author.ID,
		Author2ID:    author2ID,
		PublisherID:  publisher.ID,
		ISBN:         form.ISBN,
		Description:  form.Description,
		Pages:        form.Pages,
		ExternalLink: form.ExternalLink,
		DirDwl:       form.DirDwl,
	})
	if err != nil {
		if errors.Is(err, store.ErrEditConflict) {
			form.Errors["general"] = "El libro fue modificado por otra sesión; recarga la página."
			renderForm(http.StatusConflict)
			return
		}
		app.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, "/books/"+slug, http.StatusSeeOther)
}

func (app *application) deleteBookHandler(w http.ResponseWriter, r *http.Request) {
	book, ok := app.bookFromPath(w, r)
	if !ok {
		return
	}

	// Primero la base de datos (si falla, no se pierde ningún archivo) y
	// después los archivos, en best-effort.
	if err := app.store.DeleteBook(r.Context(), book.ID); err != nil {
		app.serverError(w, r, err)
		return
	}
	if book.Filename != "" {
		app.removeFiles(app.bookFilePaths(book.Filename))
	}

	app.logger.Info("book deleted", "slug", book.Slug, "filename", book.Filename)
	http.Redirect(w, r, "/dashboard/books", http.StatusSeeOther)
}

func (app *application) bookFromPath(w http.ResponseWriter, r *http.Request) (*store.Book, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return nil, false
	}
	book, err := app.store.GetBookByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return nil, false
	}
	return book, true
}
