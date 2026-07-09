# Deploy de Pirateca en el VPS

Guía copy-paste. Supuestos: Debian/Ubuntu, nginx y PostgreSQL ya
instalados, la base `pirateca` existente con los datos actuales, y los
archivos de `uploads/` del servicio viejo disponibles. Ajusta rutas,
usuario de Postgres y dominio donde haga falta.

## 1. Paquetes del sistema

```sh
sudo apt update
sudo apt install -y golang libimage-exiftool-perl transmission-cli imagemagick
```

(Si el Go de los repos es viejo, instala desde https://go.dev/dl/ — se
necesita Go 1.24+.)

## 2. Usuario y directorios

```sh
sudo useradd --system --home /opt/pirateca --shell /usr/sbin/nologin pirateca
sudo mkdir -p /opt/pirateca/uploads/{covers,pdfs,epubs,torrents,torrentadded}
```

Copia los archivos existentes del servicio viejo (ajusta el origen):

```sh
sudo rsync -a /ruta/del/viejo/uploads/ /opt/pirateca/uploads/
sudo chown -R pirateca:pirateca /opt/pirateca
```

## 3. Compilar y colocar el binario

En el VPS (o compila en tu máquina con `GOOS=linux GOARCH=amd64 make build`
y súbelo con scp):

```sh
git clone https://github.com/jesarx/pirateca.git /tmp/pirateca-src
cd /tmp/pirateca-src
make tailwind && make build
sudo cp bin/pirateca /opt/pirateca/pirateca
sudo chown pirateca:pirateca /opt/pirateca/pirateca
```

## 4. Migraciones de base de datos

Solo hay que aplicar la 15 (corrige la rotación de slugs) y la 16 (tabla
de visitas). La 14 es un no-op documental, pero es seguro correrla.
`schema_migrations` del VPS está en 11 con cambios manuales aplicados;
el último comando lo sincroniza a 16 por si algún día usas golang-migrate.

```sh
cd /tmp/pirateca-src
sudo -u postgres psql -d pirateca -f migrations/000014_sync_real_schema.up.sql
sudo -u postgres psql -d pirateca -f migrations/000015_fix_slug_rotation.up.sql
sudo -u postgres psql -d pirateca -f migrations/000016_create_visits.up.sql
sudo -u postgres psql -d pirateca -c "UPDATE schema_migrations SET version = 16, dirty = false;"
```

Dale permisos al usuario de la app sobre la tabla nueva (usa el usuario
de Postgres que tengas en el DSN):

```sh
sudo -u postgres psql -d pirateca -c "GRANT ALL ON visits TO TU_USUARIO_DE_DB;"
```

## 5. Configuración

```sh
sudo tee /etc/pirateca.env > /dev/null <<'EOF'
PIRATECA_DB_DSN=postgres://TU_USUARIO_DE_DB:TU_PASSWORD@localhost/pirateca?sslmode=disable
PIRATECA_SESSION_SECRET=CAMBIA-ESTO-POR-32+CARACTERES-ALEATORIOS
EOF
sudo chmod 600 /etc/pirateca.env
```

Genera un secret decente con: `openssl rand -base64 48`

## 6. Servicio systemd

```sh
sudo cp deploy/pirateca.service /etc/systemd/system/pirateca.service
sudo systemctl daemon-reload
sudo systemctl enable --now pirateca
systemctl status pirateca
curl -s http://127.0.0.1:4000/health   # debe responder: ok
```

## 7. nginx

```sh
sudo cp deploy/nginx-pirateca.conf /etc/nginx/sites-available/pirateca
sudo ln -s /etc/nginx/sites-available/pirateca /etc/nginx/sites-enabled/pirateca
# Desactiva los sitios del stack viejo (frontend Next + api) cuando
# confirmes que todo funciona:
# sudo rm /etc/nginx/sites-enabled/EL-SITIO-VIEJO
sudo nginx -t && sudo systemctl reload nginx
```

TLS con certbot (si el dominio ya apuntaba aquí, reutiliza el cert):

```sh
sudo certbot --nginx -d pirateca.com -d www.pirateca.com
```

## 8. Verificación y limpieza

```sh
curl -sI https://pirateca.com/books | head -1        # 200
curl -sI https://pirateca.com/ | head -1             # 301 → /books
```

Entra a https://pirateca.com/admin/login con tu email y contraseña de
siempre (los de la tabla users). Sube un libro de prueba y bórralo.

Cuando todo esté verificado, apaga y deshabilita los servicios viejos
(la API de Go vieja y el Next.js con su Node), y borra sus directorios.
El cliente de torrents debe seguir vigilando
`/opt/pirateca/uploads/torrentadded/` (ajusta su watch-dir si cambió la
ruta).

## Actualizaciones futuras

```sh
cd /tmp/pirateca-src && git pull
make css && make build
sudo systemctl stop pirateca
sudo cp bin/pirateca /opt/pirateca/pirateca
sudo systemctl start pirateca
```
