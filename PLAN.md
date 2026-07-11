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
- [x] **Fase 1 — Núcleo de datos**: `internal/store` contra el esquema
  Postgres existente; listado público `/books` con búsqueda, filtros por
  tag/autor/editorial, ordenamiento y paginación. Adelantado de otras
  fases: `GetBookBySlug` (para Fase 2) y el servido de portadas
  `/images?file=` (para que el listado tenga imágenes). Verificado contra
  un Postgres 16 local con las migraciones reales y datos de prueba.
- [x] **Fase 2 — Catálogo completo**: detalle `/books/{slug}` (portada,
  descripción, meta, botones de descarga/liga externa/copiar enlace),
  `/authors` y `/publishers` con búsqueda + orden + conteo de libros,
  `/authors/{slug}` y `/publishers/{slug}` (listado de libros filtrado),
  `/tags`. Verificado contra el dump real del VPS (2026-06-29).
- [x] **Fase 3 — Archivos**: portadas y descargas PDF/EPUB/torrent con
  Content-Disposition (validación de extensión y path traversal portada
  del código viejo). La *subida* de archivos es parte de la Fase 4.
- [x] **Fase 4 — Admin**: login con sesión (cookie HMAC firmada, bcrypt
  contra la tabla `users` existente), dashboard CRUD de
  libros/autores/editoriales, subida de archivos con el pipeline
  delicado portado tal cual (ver notas), CSRF por chequeo de
  Origin/Referer + SameSite=Lax, autollenado por ISBN vía OpenLibrary.
  Verificado end-to-end con PDF/PNG reales contra la base real.
- [x] **Fase 5 — Pulido y deploy**: manifiesto, contacto y la noticia
  «Mal que dura cien años» portados (el post "volvimos" era un duplicado
  y no se portó); banner de anuncio en la portada del catálogo; página
  404 con layout; dashboard con estadísticas (tiles de catálogo y
  visitas, gráficas de visitas por día, libros por mes y top categorías,
  últimos libros); contador de visitas (migración 000016, acumulado en
  memoria con flush cada 30s y al apagar, ignora bots y solo cuenta
  páginas públicas); apagado ordenado del servidor; `deploy/` con
  `pirateca.service` (systemd), config de nginx y `DEPLOY.md` con el
  instructivo copy-paste.

## Notas / hallazgos

- **SEO/perf (post-lanzamiento)**: meta description por página, canonical,
  Open Graph (+ og:image con la portada y JSON-LD schema.org/Book en el
  detalle), robots.txt, sitemap.xml dinámico, Cache-Control en estáticos
  (7 días, CSS versionado con ?v= por arranque) y portadas (1 día), gzip
  en la config de nginx.
- **Sembrado de torrents**: la cadena es app → torrentadded/ → watch-dir
  de transmission → transmission busca el PDF en su download-dir. Si
  download-dir no apunta a uploads/pdfs, los torrents quedan al 0% y
  nadie puede descargar. Diagnóstico y reparación: `deploy/check-seeding.sh`
  (correr en el VPS).

- **Pipeline de subida de PDF — EL ORDEN IMPORTA** (`cmd/web/process.go`):
  guardar → `exiftool -all:all=` (limpiar metadatos) → `exiftool`
  escribir Título/Autor/Editorial → `transmission-create` (el torrent
  hashea el contenido; debe ir DESPUÉS de los metadatos) → copiar a
  `uploads/torrentadded/` (carpeta vigilada por el cliente de torrents).
  Portada: guardar (convertir a JPG con ImageMagick si hace falta) →
  `exiftool -all:all=`. En edición el PDF es inmutable; solo la portada.
  Requiere `exiftool`, `transmission-create` y `convert` en el sistema.
- Mejoras sobre el flujo viejo: unicidad del filename verificada ANTES de
  escribir archivos; validación de extensiones en la subida; limpieza de
  archivos escritos si el INSERT falla; al borrar, primero la base y
  luego los archivos.
- **Bug corregido (migración 000015)**: los triggers de slug regeneraban
  el slug en cada UPDATE chocando consigo mismos → cada edición añadía
  -1, -2… y rompía enlaces (hay cicatrices en los datos reales, no se
  tocaron). Ahora el slug solo se regenera si cambian los campos de los
  que depende y el chequeo de unicidad excluye la propia fila. También se
  corrigió que el slug de autores sin nombre (NULL) quedara NULL.
  **Esta migración SÍ hay que correrla en el VPS** (a diferencia de la
  000014 que es no-op): `psql -d pirateca -f
  migrations/000015_fix_slug_rotation.up.sql` o vía golang-migrate.
- El login de prueba local es test@pirateca.com / prueba123 (solo existe
  en la base local del sandbox, no en el VPS).
- `schema_migrations` del VPS está en 11 aunque el esquema tiene los
  cambios de la 12 y 13 (más deriva manual). El DEPLOY.md lo sincroniza
  a 16 tras aplicar las migraciones nuevas (15 y 16; la 14 es no-op).
- Para el deploy seguir `deploy/DEPLOY.md`. Migraciones que el VPS
  necesita: 000015 (fix slugs) y 000016 (tabla visits) + GRANT sobre
  visits al usuario del DSN.

- **El esquema real del VPS difiere de las migraciones 1-13** (confirmado
  con el backup `pirateca_20260629.sql`): `books.year` y `books.tags` son
  nullables (cambio manual) y `dir_dwl` es `NOT NULL`. La migración
  `000014_sync_real_schema` documenta esto y es un no-op en la base real
  (verificado). Las queries usan `COALESCE` para year/tags/filename/isbn/
  description/pages/external_link por si acaso. La base real también tiene
  las tablas `temporal` (resto de una importación vieja, se puede tirar
  cuando quieras) y `schema_migrations` (bookkeeping de golang-migrate).
- La base real tiene 302 libros, 227 autores, 99 editoriales y 1 usuario.

- El índice `books_title_idx` (migración 3) usa `to_tsvector('simple',
  title)` pero la búsqueda usa `'spanish'` + `unaccent`, así que ese
  índice no se aprovecha. Irrelevante a esta escala; si algún día molesta,
  crear índice con wrapper IMMUTABLE de unaccent.
- El esquema de la base **no se modifica** (decisión de Fase 1): tags como
  `text[]` con GIN, slugs por trigger y las FK actuales funcionan bien.
- Para probar en local: Postgres + `CREATE EXTENSION citext; CREATE
  EXTENSION unaccent;`, correr `migrations/*.up.sql` en orden (algunos
  archivos no terminan en `;` — agregarlo al concatenar), sembrar datos.

## Cómo retomar el trabajo en una sesión nueva

1. Leer este archivo y `git log --oneline -15`.
2. El código viejo de referencia: `git show main:cmd/api/<archivo>` y el
   repo `qumran-web` (páginas en `app/`, componentes en `components/`,
   queries al API en `lib/data.ts`).
3. Desarrollo: `make run` (el DSN es opcional mientras no haya páginas de
   datos: `PIRATECA_DB_DSN=... make run`). CSS: `make tailwind` una vez,
   luego `make css`.
4. Al terminar: actualizar las casillas de arriba, commit y push.
