-- Revert domain constraints and case-insensitive unique index

-- Drop the new indexes
DROP INDEX IF EXISTS website_domain_lower_idx;
DROP INDEX IF EXISTS website_domain_lower_deleted_idx;

-- Restore the old simple domain index
CREATE INDEX website_domain_idx ON website(domain);

-- Drop the domain length constraint
ALTER TABLE website
DROP CONSTRAINT IF EXISTS domain_length_check;
