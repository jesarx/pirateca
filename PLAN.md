# Plan de reescritura: Pirateca

Reescritura de qumran-api (API Go estilo *Let's Go Further*) + qumran-web
(Next.js) como **un solo servicio en Go** con plantillas server-rendered.
Este archivo es la fuente de verdad del avance: cada sesión de trabajo lo
actualiza al terminar.

## Decisiones tomadas

- **Un solo binario Go** sirve HTML, estáticos y descargas. Ya no hay API
  JSON ni frontend separado.
- **PostgreSQL se mantiene** y se usa la base de datos existente **tal
  cual, sin migrar datos**. Las migraciones en `migrations/` describen el
  esquema actual y se conservan como historial; el código nuevo consulta
  las tablas existentes (books, authors, publishers, users, etc.).
- **URLs públicas se conservan** (`/books`, `/books/{slug}`, `/authors`,
  `/publishers`, `/tags`, `/manifest`, `/contact`) para no romper enlaces.
- **Tailwind CSS** vía binario standalone (sin Node). El `app.css`
  generado se commitea; `make tailwind && make css` lo regenera.
- **Módulo Go**: `github.com/jesarx/pirateca`. Este repo se renombrará de
  `qumran-api` a `pirateca` en GitHub cuando la reescritura esté lista.
- **Se elimina** del código viejo: registro/activación de usuarios,
  mailer, tokens de API, tabla de permisos, expvar, CORS. El admin usará
  sesión con cookie (un solo usuario, bcrypt contra la tabla `users`).
- **Referencia**: el código viejo de la API vive en la rama `main` de este
  repo; el frontend viejo en el repo `qumran-web`.
- Interactividad con JS vanilla mínimo (debounce de búsqueda, copiar URL);
  sin frameworks de frontend.

## Fases

- [x] **Fase 0 — Esqueleto**: estructura `cmd/web` + `ui/` + `internal/`,
  servidor con logging/recover/security headers, cache de plantillas
  embebidas (`embed`), layout base + nav + footer con Tailwind, Makefile,
  este plan.
- [ ] **Fase 1 — Núcleo de datos**: `internal/store` contra el esquema
  Postgres existente; listado público `/books` con paginación.
- [ ] **Fase 2 — Catálogo completo**: detalle `/books/{slug}`, autores,
  editoriales, categorías, búsqueda y ordenamiento (query params, como el
  sitio actual).
- [ ] **Fase 3 — Archivos**: portadas, descargas PDF/EPUB/torrent
  (portar `serveFile` de `main:cmd/api/files.go`, que ya valida
  extensiones y paths), respetando `dir_dwl` y `external_link`.
- [ ] **Fase 4 — Admin**: login con sesión (cookie, bcrypt contra tabla
  `users` existente), dashboard CRUD de libros/autores/editoriales,
  subida de archivos, protección CSRF.
- [ ] **Fase 5 — Pulido y deploy**: páginas estáticas (manifiesto,
  contacto, noticias), pendientes del frontend viejo, systemd unit,
  instrucciones de deploy en VPS detrás de Caddy/nginx, redirects si
  hiciera falta.

## Cómo retomar el trabajo en una sesión nueva

1. Leer este archivo y `git log --oneline -15`.
2. El código viejo de referencia: `git show main:cmd/api/<archivo>` y el
   repo `qumran-web` (páginas en `app/`, componentes en `components/`,
   queries al API en `lib/data.ts`).
3. Desarrollo: `make run` (el DSN es opcional mientras no haya páginas de
   datos: `PIRATECA_DB_DSN=... make run`). CSS: `make tailwind` una vez,
   luego `make css`.
4. Al terminar: actualizar las casillas de arriba, commit y push.
