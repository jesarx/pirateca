-- Países de los visitantes.
--
-- visitor_countries guarda solo el agregado por día y país: las IPs NO
-- se almacenan nunca, se resuelven en memoria y se descartan.
CREATE TABLE IF NOT EXISTS visitor_countries (
    day date NOT NULL,
    country text NOT NULL,   -- ISO 3166-1 alpha-2, o '??' si no se pudo resolver
    count bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (day, country)
);

-- Tabla de rangos IP → país. Se llena con la base gratuita de DB-IP
-- (deploy/import-geoip.sh). Si está vacía, la app simplemente no
-- registra países y todo lo demás sigue funcionando igual.
CREATE TABLE IF NOT EXISTS ip_country_ranges (
    start_ip inet NOT NULL,
    end_ip inet NOT NULL,
    country text NOT NULL,
    PRIMARY KEY (start_ip, end_ip)
);

CREATE INDEX IF NOT EXISTS ip_country_ranges_lookup_idx
    ON ip_country_ranges (start_ip DESC);
