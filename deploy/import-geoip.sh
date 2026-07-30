#!/bin/bash
# Carga la base IP → país en Postgres para el panel de países del
# dashboard. Correr EN EL VPS:
#
#   sudo bash deploy/import-geoip.sh
#
# Usa la base gratuita "IP to Country Lite" de DB-IP (licencia CC-BY 4.0,
# descarga directa sin registro). Se actualiza cada mes; volver a correr
# este script reemplaza los datos. Con correrlo una o dos veces al año
# basta para tener países razonablemente correctos.
set -euo pipefail

DB="${PIRATECA_DB:-pirateca}"
MONTH="${1:-$(date +%Y-%m)}"
URL="https://download.db-ip.com/free/dbip-country-lite-${MONTH}.csv.gz"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "==> Descargando $URL"
if ! curl -fsSL "$URL" -o "$TMP/dbip.csv.gz"; then
  echo "  ✗ No se pudo descargar. Si el mes actual aún no está publicado, prueba el anterior:"
  echo "     sudo bash deploy/import-geoip.sh $(date -d '1 month ago' +%Y-%m 2>/dev/null || date +%Y-%m)"
  exit 1
fi

echo "==> Descomprimiendo"
gunzip -c "$TMP/dbip.csv.gz" > "$TMP/dbip.csv"
wc -l < "$TMP/dbip.csv" | sed 's/^/    rangos: /'

echo "==> Cargando en Postgres (tabla temporal)"
sudo -u postgres psql -d "$DB" -v ON_ERROR_STOP=1 <<SQL
CREATE TEMP TABLE dbip_import (start_ip text, end_ip text, country text);
\copy dbip_import FROM '$TMP/dbip.csv' WITH (FORMAT csv)

BEGIN;
TRUNCATE ip_country_ranges;
-- Se descartan filas inválidas y se normaliza el código de país.
INSERT INTO ip_country_ranges (start_ip, end_ip, country)
SELECT start_ip::inet, end_ip::inet, upper(country)
FROM dbip_import
WHERE length(country) = 2
  AND family(start_ip::inet) = family(end_ip::inet)
ON CONFLICT (start_ip, end_ip) DO NOTHING;
COMMIT;

ANALYZE ip_country_ranges;
SELECT count(*) AS rangos_cargados FROM ip_country_ranges;
SQL

echo "==> Listo. El panel de países del dashboard ya puede resolver visitas."
echo "    (Las visitas anteriores a esta carga quedan como «Sin identificar».)"
