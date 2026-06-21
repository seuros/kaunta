-- Repair chk_trusted_origin_format for installs that already ran the original
-- 000015 (which is also corrected in place for fresh installs).
--
-- The original regex used an over-escaped label separator '\\.' instead of
-- '\.'. With standard_conforming_strings on (the PostgreSQL default), SQL does
-- not unescape backslashes in ordinary string literals, so the regex engine
-- received '\\.' — "a literal backslash followed by any character" — between
-- domain labels. As a result NO multi-label domain could satisfy the
-- constraint; only single labels such as 'localhost' matched, making
-- `kaunta domain add example.com` and trusted-origins generally unusable.
-- See https://github.com/seuros/kaunta/issues/144
--
-- Drop and recreate the constraint with a correct literal-dot separator.
-- Idempotent: on fresh installs the constraint is already correct, so this is
-- a no-op recreate; on already-migrated installs it replaces the broken one.

ALTER TABLE trusted_origin DROP CONSTRAINT IF EXISTS chk_trusted_origin_format;

ALTER TABLE trusted_origin
ADD CONSTRAINT chk_trusted_origin_format
CHECK (
    domain ~* '^([a-z0-9]([a-z0-9-]*[a-z0-9])?)(\.([a-z0-9]([a-z0-9-]*[a-z0-9])?))*(:[0-9]{1,5})?$'
);
