-- Add domain constraints and case-insensitive unique index
-- This migration:
-- 1. Adds CHECK constraint for domain length (max 253 chars - DNS standard)
-- 2. Creates unique index on LOWER(domain) for case-insensitive lookups
-- 3. Drops old simple domain index (replaced by new functional index)

-- Add domain length constraint (253 is DNS limit for FQDN)
ALTER TABLE website
ADD CONSTRAINT domain_length_check CHECK (LENGTH(domain) > 0 AND LENGTH(domain) <= 253);

-- Drop the old simple index (we're replacing it with a functional index)
DROP INDEX IF EXISTS website_domain_idx;

-- Create unique index on LOWER(domain) where deleted_at IS NULL
-- This ensures case-insensitive domain uniqueness only for active websites
CREATE UNIQUE INDEX website_domain_lower_idx ON website (LOWER(domain)) WHERE deleted_at IS NULL;

-- Also create a non-unique index for queries that filter by deleted_at explicitly
CREATE INDEX website_domain_lower_deleted_idx ON website (LOWER(domain)) WHERE deleted_at IS NOT NULL;
