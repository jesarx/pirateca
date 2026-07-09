-- Restaura los triggers originales (con el bug de rotación de slugs).

CREATE OR REPLACE FUNCTION update_slug()
RETURNS TRIGGER AS $$
DECLARE
    base_slug text;
BEGIN
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

    NEW.slug := generate_unique_slug(base_slug);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_publisher_slug()
RETURNS TRIGGER AS $$
BEGIN
    NEW.slug := generate_unique_publisher_slug(
        LOWER(REGEXP_REPLACE(UNACCENT(NEW.name), '[^a-zA-Z0-9]+', '-', 'g'))
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_author_slug()
RETURNS TRIGGER AS $$
BEGIN
    -- Versión original, incluido su manejo de NULL.
    NEW.slug := LOWER(
        REGEXP_REPLACE(UNACCENT(NEW.last_name || '-' || NEW.name), '[^a-zA-Z0-9]+', '-', 'g')
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP FUNCTION IF EXISTS books_unique_slug(text, bigint);
DROP FUNCTION IF EXISTS publishers_unique_slug(text, bigint);
