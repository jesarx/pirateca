#!/bin/bash
# Actualiza Pirateca en el VPS: trae los cambios, recompila y reinicia.
# Correr EN EL VPS desde el clon del repo:
#
#   cd /opt/pirateca-src && sudo bash deploy/update.sh
#
# Si aparecen migraciones nuevas, las lista y se detiene para que las
# apliques a mano (nunca toca la base de datos por su cuenta).
set -euo pipefail

SRC="$(cd "$(dirname "$0")/.." && pwd)"
DEST=/opt/pirateca/pirateca
SERVICE=pirateca

cd "$SRC"

echo "==> Migraciones aplicadas antes de actualizar"
BEFORE=$(ls migrations/*.up.sql 2>/dev/null | wc -l)

echo "==> Trayendo cambios"
git pull

AFTER=$(ls migrations/*.up.sql 2>/dev/null | wc -l)
if [ "$AFTER" -gt "$BEFORE" ]; then
  echo
  echo "  ⚠  Hay $((AFTER - BEFORE)) migración(es) nueva(s):"
  ls -1 migrations/*.up.sql | tail -n $((AFTER - BEFORE)) | sed 's/^/     /'
  echo
  echo "  Aplícalas antes de continuar, por ejemplo:"
  ls -1 migrations/*.up.sql | tail -n $((AFTER - BEFORE)) | sed 's|^|     sudo -u postgres psql -d pirateca -f |'
  echo "     sudo -u postgres psql -d pirateca -c \"UPDATE schema_migrations SET version = N, dirty = false;\""
  echo
  read -r -p "  ¿Ya las aplicaste? [s/N] " ok
  [ "$ok" = "s" ] || [ "$ok" = "S" ] || { echo "Cancelado."; exit 1; }
fi

echo "==> Compilando"
[ -x ./bin/tailwindcss ] || make tailwind
make build

echo "==> Reiniciando el servicio"
systemctl stop "$SERVICE"
cp bin/pirateca "$DEST"
chown pirateca:pirateca "$DEST"
systemctl start "$SERVICE"

sleep 1
if curl -fsS http://127.0.0.1:4000/health >/dev/null; then
  echo "==> Listo: el servicio responde correctamente."
else
  echo "==> ⚠ El servicio no responde. Revisa: journalctl -u $SERVICE -n 30"
  exit 1
fi
