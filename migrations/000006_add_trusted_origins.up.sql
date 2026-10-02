-- Create trusted_origin table for managing allowed dashboard domains
CREATE TABLE IF NOT EXISTS trusted_origin (
    id SERIAL PRIMARY KEY,
    domain VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for efficient active domain lookups
CREATE INDEX idx_trusted_origin_active ON trusted_origin(domain) WHERE is_active = true;

-- Function to validate if an origin is trusted
-- Strips protocol, port, and path from origin URL before checking against database
CREATE OR REPLACE FUNCTION is_trusted_origin(p_origin TEXT)
RETURNS boolean AS $$
DECLARE
    v_domain TEXT;
    v_exists INTEGER;
BEGIN
    -- Handle null/empty origins
    IF p_origin IS NULL OR p_origin = '' OR p_origin = 'null' THEN
        RETURN false;
    END IF;

    -- Strip protocol (http://, https://)
    v_domain := regexp_replace(p_origin, '^https?://', '');

    -- Strip path (everything after first /)
    v_domain := regexp_replace(v_domain, '/.*$', '');

    -- Strip port number
    v_domain := regexp_replace(v_domain, ':\d+$', '');

    -- Strip trailing slash
    v_domain := regexp_replace(v_domain, '/$', '');

    -- Convert to lowercase for case-insensitive comparison
    v_domain := lower(v_domain);

    -- Check if domain exists and is active
    SELECT COUNT(*) INTO v_exists
    FROM trusted_origin
    WHERE lower(domain) = v_domain AND is_active = true;

    RETURN v_exists > 0;
END;
$$ LANGUAGE plpgsql STABLE;

-- Function to get all active trusted origins as an array
-- Used for caching in application layer
CREATE OR REPLACE FUNCTION get_trusted_origins()
RETURNS TEXT[] AS $$
BEGIN
    RETURN ARRAY(
        SELECT domain
        FROM trusted_origin
        WHERE is_active = true
        ORDER BY domain
    );
END;
$$ LANGUAGE plpgsql STABLE;

-- Add trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_trusted_origin_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_trusted_origin_timestamp
    BEFORE UPDATE ON trusted_origin
    FOR EACH ROW
    EXECUTE FUNCTION update_trusted_origin_timestamp();
