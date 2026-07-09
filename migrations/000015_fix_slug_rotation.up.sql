-- Corrige la rotación de slugs al editar: el trigger regeneraba el slug
-- en cada UPDATE y el chequeo de unicidad chocaba con la propia fila,
-- añadiendo -1, -2, ... en cada edición (hay cicatrices de esto en los
-- datos reales). Ahora: (1) el slug solo se regenera si cambian los
-- campos de los que depende, y (2) el chequeo de unicidad excluye a la
-- propia fila. Los slugs existentes NO se tocan para no romper enlaces.

CREATE OR REPLACE FUNCTION books_unique_slug(base_slug text, self_id bigint)
RETURNS text AS $$
DECLARE
    unique_slug text := base_slug;
    counter integer := 1;
BEGIN
    LOOP
        PERFORM FROM books WHERE slug = unique_slug AND id IS DISTINCT FROM self_id;
        IF NOT FOUND THEN
            RETURN unique_slug;
        END IF;
        unique_slug := base_slug || '-' || counter;
        counter := counter + 1;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_slug()
RETURNS TRIGGER AS $$
DECLARE
    base_slug text;
BEGIN
    -- En updates que no cambian autor ni título corto, conservar el slug.
    IF TG_OP = 'UPDATE' AND NEW.slug IS NOT NULL
       AND NEW.short_title = OLD.short_title
       AND NEW.auth_id = OLD.auth_id THEN
        RETURN NEW;
    END IF;

    SELECT
        LOWER(
            REGEXP_REPLACE(UNACCENT(COALESCE(a.last_name, '')), '[^a-zA-Z0-9]+', '-', 'g') || '-' ||
            REGEXP_REPLACE(UNACCENT(COALESCE(a.name, '')), '[^a-zA-Z0-9]+', '-', 'g') || '-' ||
            REGEXP_REPLACE(UNACCENT(NEW.short_title), '[^a-zA-Z0-9]+', '-', 'g')
        ) INTO base_slug
    FROM authors a
    WHERE a.id = NEW.auth_id;

    IF base_slug IS NULL THEN
        base_slug := LOWER(
            REGEXP_REPLACE(UNACCENT(NEW.short_title), '[^a-zA-Z0-9]+', '-', 'g')
        );
    END IF;

    NEW.slug := books_unique_slug(base_slug, NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION publishers_unique_slug(base_slug text, self_id bigint)
RETURNS text AS $$
DECLARE
    unique_slug text := base_slug;
    counter integer := 1;
BEGIN
    LOOP
        PERFORM FROM publishers WHERE slug = unique_slug AND id IS DISTINCT FROM self_id;
        IF NOT FOUND THEN
            RETURN unique_slug;
        END IF;
        unique_slug := base_slug || '-' || counter;
        counter := counter + 1;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_publisher_slug()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.slug IS NOT NULL AND NEW.name = OLD.name THEN
        RETURN NEW;
    END IF;

    NEW.slug := publishers_unique_slug(
        LOWER(REGEXP_REPLACE(UNACCENT(NEW.name), '[^a-zA-Z0-9]+', '-', 'g')),
        NEW.id
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- El trigger de autores no numera slugs (sobrescribe directo), pero
-- también conviene no regenerarlo cuando el nombre no cambió. Además se
-- corrige otro bug del original: con name NULL, last_name || '-' || name
-- es NULL y el slug quedaba NULL; ahora se usa COALESCE (los autores sin
-- nombre quedan como 'apellido-', igual que los existentes, ej. 'vv-aa-').
CREATE OR REPLACE FUNCTION update_author_slug()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.slug IS NOT NULL
       AND NEW.name IS NOT DISTINCT FROM OLD.name
       AND NEW.last_name = OLD.last_name THEN
        RETURN NEW;
    END IF;

    NEW.slug := LOWER(
        REGEXP_REPLACE(UNACCENT(NEW.last_name || '-' || COALESCE(NEW.name, '')), '[^a-zA-Z0-9]+', '-', 'g')
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
