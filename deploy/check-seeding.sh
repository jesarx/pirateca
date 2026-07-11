#!/bin/bash
# Diagnóstico del sembrado de torrents de Pirateca. Correr EN EL VPS.
#
# La cadena completa que tiene que funcionar:
#   1. La app crea el .torrent y lo copia a uploads/torrentadded/
#   2. transmission-daemon vigila esa carpeta (watch-dir) y agrega el torrent
#   3. transmission busca los DATOS en su download-dir: si ahí está el PDF
#      con el nombre exacto, verifica y SIEMBRA; si no, se queda
#      "descargando" al 0% para siempre (y nadie puede bajar nada)
#   4. Los peers tienen que poder conectarse: puerto de peers abierto
#
# Uso: bash check-seeding.sh [usuario:contraseña de transmission-remote]

AUTH="${1:+-n $1}"
PDFS=/opt/pirateca/uploads/pdfs
ADDED=/opt/pirateca/uploads/torrentadded

echo "=== 1. Configuración de transmission ==="
SETTINGS=""
for f in /etc/transmission-daemon/settings.json \
         /var/lib/transmission-daemon/info/settings.json \
         ~/.config/transmission-daemon/settings.json; do
  [ -f "$f" ] && SETTINGS="$f" && break
done
if [ -z "$SETTINGS" ]; then
  echo "  ✗ No encontré settings.json — ¿transmission-daemon está instalado?"
  echo "    sudo apt install transmission-daemon"
else
  echo "  settings.json: $SETTINGS"
  grep -E '"(download-dir|watch-dir|watch-dir-enabled|peer-port|port-forwarding-enabled)"' "$SETTINGS" | sed 's/^/    /'
  echo
  echo "  Lo que DEBE decir:"
  echo "    \"download-dir\": \"$PDFS\","
  echo "    \"watch-dir\": \"$ADDED\","
  echo "    \"watch-dir-enabled\": true,"
fi

echo
echo "=== 2. Estado de los torrents en transmission ==="
if command -v transmission-remote >/dev/null; then
  transmission-remote $AUTH -l | head -25
  echo "  → Si 'Done' no es 100% o el estado es 'Idle'/'Up & Down' con 0%,"
  echo "    transmission NO encuentra los PDFs (download-dir mal apuntado)."
else
  echo "  ✗ transmission-remote no está instalado"
fi

echo
echo "=== 3. Prueba del puerto de peers ==="
transmission-remote $AUTH -pt 2>/dev/null | sed 's/^/  /'
echo "  → Si dice 'No', abre el puerto en el firewall:"
echo "    sudo ufw allow 51413  (o el peer-port de settings.json, TCP y UDP)"

echo
echo "=== 4. ¿Los archivos existen y son legibles para transmission? ==="
TUSER=$(ps -o user= -C transmission-daemon | head -1)
echo "  transmission corre como: ${TUSER:-no está corriendo}"
ls -la "$PDFS" 2>/dev/null | head -4 | sed 's/^/  /'
if [ -n "$TUSER" ]; then
  PDF1=$(ls "$PDFS"/*.pdf 2>/dev/null | head -1)
  if [ -n "$PDF1" ] && sudo -u "$TUSER" test -r "$PDF1"; then
    echo "  ✓ $TUSER puede leer los PDFs"
  else
    echo "  ✗ $TUSER NO puede leer los PDFs — revisa permisos:"
    echo "    sudo chmod o+rx /opt /opt/pirateca /opt/pirateca/uploads $PDFS"
    echo "    sudo chmod o+r $PDFS/*.pdf"
  fi
fi

echo
echo "=== 5. ¿Cuántos seeders ven los trackers? (verificable desde cualquier máquina) ==="
TORR=$(ls /opt/pirateca/uploads/torrents/*.torrent 2>/dev/null | head -1)
if [ -n "$TORR" ]; then
  transmission-show --scrape "$TORR" 2>/dev/null | sed 's/^/  /'
  echo "  → '0 seeders' en todos los trackers = el VPS no está sembrando o no es alcanzable."
fi

cat <<'EOF'

=== REPARACIÓN (los dos arreglos más comunes) ===

A) download-dir mal apuntado — corrígelo y re-agrega todo:
   sudo systemctl stop transmission-daemon
   # edita settings.json: download-dir → /opt/pirateca/uploads/pdfs
   #                      watch-dir    → /opt/pirateca/uploads/torrentadded
   #                      watch-dir-enabled → true
   sudo systemctl start transmission-daemon
   # borra los torrents "atorados" (SIN borrar datos) y re-agrégalos:
   transmission-remote -t all -r
   for t in /opt/pirateca/uploads/torrents/*.torrent; do
     transmission-remote -a "$t" -w /opt/pirateca/uploads/pdfs
   done
   transmission-remote -t all -v   # verificar datos → deben quedar en Seeding

B) Puerto cerrado — ábrelo:
   sudo ufw allow 51413/tcp && sudo ufw allow 51413/udp
   (usa el peer-port real de settings.json)
EOF
