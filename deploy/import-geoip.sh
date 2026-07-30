#!/bin/bash
# Carga la base IP → país en Postgres para el panel de países del
# dashboard. Correr EN EL VPS:
#
#   sudo bash deploy/import-geoip.sh              # descarga el mes actual
#   sudo bash deploy/import-geoip.sh 2026-06      # descarga otro mes
#   sudo bash deploy/import-geoip.sh archivo.csv  # usa un CSV ya bajado
#
# Usa la base gratuita "IP to Country Lite" de DB-IP (licencia CC-BY 4.0,
# descarga directa sin registro). Se actualiza cada mes; volver a correr
# este script reemplaza los datos. Con correrlo una o dos veces al año
# basta para tener países razonablemente correctos.
set -euo pipefail

DB="${PIRATECA_DB:-pirateca}"
ARG="${1:-$(date +%Y-%m)}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

if [ -f "$ARG" ]; then
	echo "==> Usando el archivo local $ARG"
	case "$ARG" in
	*.gz) gunzip -c "$ARG" > "$TMP/dbip.csv" ;;
	*) cp "$ARG" "$TMP/dbip.csv" ;;
	esac
else
	URL="https://download.db-ip.com/free/dbip-country-lite-${ARG}.csv.gz"
	echo "==> Descargando $URL"
	if ! curl -fsSL "$URL" -o "$TMP/dbip.csv.gz"; then
		PREV=$(date -d '1 month ago' +%Y-%m 2>/dev/null || echo "el mes anterior")
		echo "  ✗ No se pudo descargar. Si el mes actual aún no está publicado, prueba:"
		echo "     sudo bash deploy/import-geoip.sh $PREV"
		exit 1
	fi
	echo "==> Descomprimiendo"
	gunzip -c "$TMP/dbip.csv.gz" > "$TMP/dbip.csv"
fi

wc -l < "$TMP/dbip.csv" | sed 's/^/    rangos: /'

echo "==> Cargando en Postgres"
# El CSV se manda por stdin (COPY ... FROM STDIN) en vez de leerlo desde
# disco: psql corre como el usuario postgres y no podría entrar al
# directorio temporal que crea mktemp (permisos 700 del usuario actual).
{
	cat <<'SQL'
CREATE TEMP TABLE dbip_import (start_ip text, end_ip text, country text);
COPY dbip_import FROM STDIN WITH (FORMAT csv);
SQL
	cat "$TMP/dbip.csv"
	printf '\\.\n'
	cat <<'SQL'
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
} | sudo -u postgres psql -d "$DB" -v ON_ERROR_STOP=1 --quiet

echo "==> Listo. El panel de países del dashboard ya puede resolver visitas."
echo "    (Las visitas anteriores a esta carga quedan como «Sin identificar».)"
