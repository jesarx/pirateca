-- Contador de descargas de PDFs: una fila por día y archivo. Igual que
-- visits, la app acumula en memoria y hace flush periódico. Se guarda el
-- nombre base del archivo (books.filename) para poder atribuir cada
-- descarga a su libro.
CREATE TABLE IF NOT EXISTS downloads (
    day date NOT NULL,
    filename text NOT NULL,
    count bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (day, filename)
);
