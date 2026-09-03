# Pirateca en la red Tor

Guía copy-paste para publicar el sitio como **servicio onion**, además
del sitio normal. Es el mismo binario y la misma base de datos: solo se
añade una puerta de entrada más.

Qué se gana: quien nos lee por Tor no revela a su proveedor de internet
qué consulta ni qué descarga, y el sitio sigue alcanzable donde esté
bloqueado por DNS o por IP.

## Cómo queda el flujo

```
Internet  →  nginx :443  ─┐
                          ├─→  pirateca (127.0.0.1:4000)  →  PostgreSQL
red Tor   →  tor  →  nginx 127.0.0.1:8080  ─┘
```

El servicio de Tor entrega el tráfico a un nginx que escucha **solo en
loopback**: desde internet nadie puede tocar ese puerto.

## 1. Instalar Tor

```sh
sudo apt update
sudo apt install -y tor
```

## 2. Declarar el servicio onion

```sh
sudo tee -a /etc/tor/torrc > /dev/null <<'EOF'

# Servicio onion de Pirateca
HiddenServiceDir /var/lib/tor/pirateca/
HiddenServicePort 80 127.0.0.1:8080
EOF

sudo systemctl restart tor
```

Tor genera la clave y la dirección en el primer arranque. Léela:

```sh
sudo cat /var/lib/tor/pirateca/hostname
```

Sale algo como `abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrstuvwxyz.onion`.
**Cópiala**: la necesitas en los dos pasos siguientes.

> ⚠ **Respalda la clave.** El contenido de `/var/lib/tor/pirateca/` es la
> identidad del sitio en Tor. Si se pierde, la dirección se pierde para
> siempre y hay que anunciar una nueva; si alguien la copia, puede
> suplantar el sitio. Guárdala cifrada y fuera del VPS:
>
> ```sh
> sudo tar czf - -C /var/lib/tor pirateca | gpg -c > pirateca-onion-key.tar.gz.gpg
> ```

## 3. nginx para el servicio onion

```sh
cd /opt/pirateca-src
sudo cp deploy/nginx-pirateca-onion.conf /etc/nginx/sites-available/pirateca-onion

# Pon la dirección real en server_name:
ONION=$(sudo cat /var/lib/tor/pirateca/hostname)
sudo sed -i "s/TU-DIRECCION.onion/$ONION/" /etc/nginx/sites-available/pirateca-onion

sudo ln -s /etc/nginx/sites-available/pirateca-onion /etc/nginx/sites-enabled/pirateca-onion
sudo nginx -t && sudo systemctl reload nginx
```

## 4. Decirle la dirección a la app

Con esto la app anuncia el servicio onion a Tor Browser (cabecera
`Onion-Location`) y muestra la dirección en el pie y en Contacto:

```sh
ONION=$(sudo cat /var/lib/tor/pirateca/hostname)
echo "PIRATECA_ONION_ADDRESS=$ONION" | sudo tee -a /etc/pirateca.env

sudo systemctl restart pirateca
```

## 5. Comprobar

Desde el propio VPS, simulando la entrada por Tor:

```sh
ONION=$(sudo cat /var/lib/tor/pirateca/hostname)

# El sitio responde por el puerto del servicio onion
curl -s -o /dev/null -w "onion: %{http_code}\n" -H "Host: $ONION" http://127.0.0.1:8080/books

# Y NO genera enlaces al clearnet (el canonical debe ser la .onion)
curl -s -H "Host: $ONION" http://127.0.0.1:8080/books | grep -o '<link rel="canonical"[^>]*>'

# El sitio normal anuncia la versión onion
curl -sI https://pirateca.com/books | grep -i onion-location
```

Y desde el **Navegador Tor**, abre `http://TU-DIRECCION.onion`. Prueba:
catálogo, portadas, buscador, descarga de un PDF y de un torrent.

Al entrar a `pirateca.com` con el Navegador Tor debe aparecer un botón
morado **«.onion disponible»** en la barra de direcciones.

## Notas

**La dirección tarda unos minutos** en ser alcanzable la primera vez,
mientras Tor publica el descriptor del servicio.

**Las estadísticas del dashboard** cuentan las visitas por Tor como un
país aparte, «🧅 Red Tor». No se resuelve ninguna IP: por diseño, nginx
del onion **no manda** `X-Forwarded-For` ni `X-Real-IP`, así que la app
nunca ve de dónde viene esa gente. Es lo correcto para un servicio onion.

**Los enlaces se quedan dentro de Tor**: cuando la visita entra por la
dirección `.onion`, el canonical, el Open Graph, el sitemap y el
`robots.txt` usan esa misma dirección, no `pirateca.com`. Así nadie sale
de la red sin querer.

**Los torrents siguen funcionando igual**, pero ojo: descargar por
torrent revela la IP a los demás pares aunque el `.torrent` se haya
bajado por Tor. Quien necesite anonimato completo debe usar la descarga
directa del PDF.

**Actualizaciones**: nada especial. `deploy/update.sh` recompila y
reinicia; el servicio onion sigue funcionando sin tocar nada.
