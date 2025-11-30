-- Clean up invalid trusted_origin rows and enforce format
-- Accepts host or host:port, lowercase/uppercase; disallows paths, queries, fragments, wildcards.

-- Delete rows with obvious invalid characters (path/query/fragment/wildcards/whitespace/empty)
DELETE FROM trusted_origin
WHERE domain IS NULL
   OR trim(domain) = ''
   OR domain ~ '[\\s/\\?#*]';

-- Normalize to lowercase for consistent comparisons
UPDATE trusted_origin
SET domain = lower(domain);

-- Remove rows that don't match the allowed host[:port] pattern
DELETE FROM trusted_origin
WHERE domain !~* '^([a-z0-9]([a-z0-9-]*[a-z0-9])?)(\\.([a-z0-9]([a-z0-9-]*[a-z0-9])?))*(:[0-9]{1,5})?$';

-- Add constraint to prevent bad data going forward
ALTER TABLE trusted_origin
ADD CONSTRAINT chk_trusted_origin_format
CHECK (
    domain ~* '^([a-z0-9]([a-z0-9-]*[a-z0-9])?)(\\.([a-z0-9]([a-z0-9-]*[a-z0-9])?))*(:[0-9]{1,5})?$'
);
