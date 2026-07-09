# Pirateca

Los libros no se roban, ¡se expropian!

Biblioteca digital escrita en Go: un solo binario que sirve el catálogo
(HTML server-rendered con `html/template` + Tailwind), las descargas de
archivos y el panel de administración, respaldado por PostgreSQL.

Este repo está en proceso de reescritura — antes era una API JSON
(`qumran-api`) con un frontend Next.js aparte (`qumran-web`). El avance y
las decisiones están documentados en [PLAN.md](PLAN.md).

## Dependencias externas

La subida de libros usa tres herramientas del sistema (en Debian/Ubuntu):

```sh
apt install libimage-exiftool-perl transmission-cli imagemagick
```

- **exiftool**: limpia los metadatos del PDF/portada y escribe los nuevos.
- **transmission-create**: genera el `.torrent` del PDF.
- **convert** (ImageMagick): convierte portadas a JPG.

## Desarrollo

```sh
make tailwind   # una sola vez: descarga el binario standalone de Tailwind
make css        # regenera ui/static/css/app.css
make run        # levanta el servidor en :4000
```

Configuración por flags o entorno:

| Flag | Entorno | Default |
|---|---|---|
| `-addr` | — | `:4000` |
| `-db-dsn` | `PIRATECA_DB_DSN` | — |
| `-env` | — | `development` |
| `-uploads-dir` | — | `./uploads` |
| `-session-secret` | `PIRATECA_SESSION_SECRET` | efímero (mín. 32 caracteres) |

## Build de producción

```sh
make build      # genera ./bin/pirateca (CSS incluido, plantillas embebidas)
```
