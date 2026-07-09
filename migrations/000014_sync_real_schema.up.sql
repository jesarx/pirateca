-- Sincroniza las migraciones con el esquema real del VPS (backup
-- 2026-06-29): year y tags se volvieron nullables con cambios manuales, y
-- dir_dwl se endureció a NOT NULL. En la base real esto es un no-op.
ALTER TABLE books ALTER COLUMN year DROP NOT NULL;
ALTER TABLE books ALTER COLUMN tags DROP NOT NULL;
UPDATE books SET dir_dwl = TRUE WHERE dir_dwl IS NULL;
ALTER TABLE books ALTER COLUMN dir_dwl SET NOT NULL;
