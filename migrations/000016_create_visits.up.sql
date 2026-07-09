-- Contador de visitas simple: una fila por día. La app acumula en
-- memoria y hace flush periódico, así que el tráfico normal no genera
-- escrituras por request.
CREATE TABLE IF NOT EXISTS visits (
    day date PRIMARY KEY,
    count bigint NOT NULL DEFAULT 0
);
