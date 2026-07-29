-- Origen del tráfico (referrers): de qué sitios externos llegan las
-- visitas. Dos tablas, igual que el resto de las estadísticas: una
-- agregada por día y host (para el top) y otra con los hits recientes
-- (para la lista de "últimos"), que se poda sola en cada flush.
CREATE TABLE IF NOT EXISTS referrers (
    day date NOT NULL,
    host text NOT NULL,
    count bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (day, host)
);

CREATE TABLE IF NOT EXISTS referrer_hits (
    id bigserial PRIMARY KEY,
    seen_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    host text NOT NULL,
    url text NOT NULL,
    path text NOT NULL
);

CREATE INDEX IF NOT EXISTS referrer_hits_seen_at_idx ON referrer_hits (seen_at DESC);
