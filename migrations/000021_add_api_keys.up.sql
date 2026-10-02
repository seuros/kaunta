-- API Keys for Server-Side Ingest API
-- Migration 000021

-- ============================================================
-- API Keys Table
-- ============================================================

CREATE TABLE api_keys (
    key_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    website_id UUID NOT NULL REFERENCES website(website_id) ON DELETE CASCADE,
    created_by UUID REFERENCES users(user_id),
    key_hash VARCHAR(64) NOT NULL,
    key_prefix VARCHAR(16) NOT NULL,
    name VARCHAR(100),
    scopes TEXT[] DEFAULT ARRAY['ingest'],
    rate_limit_per_minute INT DEFAULT 1000,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ
);

-- Hash lookup index (only active keys)
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash) WHERE revoked_at IS NULL;
CREATE INDEX idx_api_keys_website ON api_keys(website_id);
CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);

COMMENT ON TABLE api_keys IS 'API keys for server-side event ingestion. Keys are SHA256 hashed.';
COMMENT ON COLUMN api_keys.key_hash IS 'SHA256 hash of the full API key. Never store plaintext.';
COMMENT ON COLUMN api_keys.key_prefix IS 'First 16 chars of key for display (e.g., kaunta_live_abc)';
COMMENT ON COLUMN api_keys.scopes IS 'Permissions: ingest (default), read, admin';
COMMENT ON COLUMN api_keys.rate_limit_per_minute IS 'Per-key rate limit. Default 1000 req/min.';

-- ============================================================
-- Per-Website Rate Limit Column
-- ============================================================

ALTER TABLE website ADD COLUMN IF NOT EXISTS api_rate_limit_per_minute INT DEFAULT 5000;

COMMENT ON COLUMN website.api_rate_limit_per_minute IS 'Aggregate rate limit for all API keys on this website';

-- ============================================================
-- Event Idempotency Table (Partitioned)
-- ============================================================

CREATE TABLE event_idempotency (
    event_id UUID NOT NULL,
    website_id UUID NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    PRIMARY KEY (event_id, created_at)
) PARTITION BY RANGE (created_at);

-- Create 7-day rolling partitions
DO $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date TEXT;
    end_date TEXT;
BEGIN
    FOR i IN 0..7 LOOP
        partition_date := CURRENT_DATE + i;
        partition_name := 'event_idempotency_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
        start_date := TO_CHAR(partition_date, 'YYYY-MM-DD');
        end_date := TO_CHAR(partition_date + 1, 'YYYY-MM-DD');

        EXECUTE format('
            CREATE TABLE IF NOT EXISTS %I
            PARTITION OF event_idempotency
            FOR VALUES FROM (%L) TO (%L)
        ', partition_name, start_date, end_date);
    END LOOP;
END $$;

CREATE INDEX idx_event_idempotency_website ON event_idempotency(website_id, event_id);

COMMENT ON TABLE event_idempotency IS 'Deduplication table for idempotent event ingestion. Auto-partitioned, 7-day retention.';

-- ============================================================
-- Rate Limit Storage (UNLOGGED for Performance)
-- ============================================================

CREATE UNLOGGED TABLE rate_limit_storage (
    k  VARCHAR(64) PRIMARY KEY,
    v  BYTEA NOT NULL,
    e  BIGINT DEFAULT 0
);

CREATE INDEX idx_rate_limit_exp ON rate_limit_storage(e) WHERE e > 0;

COMMENT ON TABLE rate_limit_storage IS 'UNLOGGED: Rate limit counters for API keys. Ephemeral - loss on crash resets limits. No WAL for 2-10x performance.';
COMMENT ON COLUMN rate_limit_storage.k IS 'Key: "key:<key_id>" or "website:<website_id>"';
COMMENT ON COLUMN rate_limit_storage.v IS 'Counter value (Fiber storage format)';
COMMENT ON COLUMN rate_limit_storage.e IS 'Expiration unix timestamp';

-- ============================================================
-- API Key Validation Function
-- ============================================================

CREATE OR REPLACE FUNCTION validate_api_key(p_key_hash VARCHAR(64))
RETURNS TABLE (
    key_id UUID,
    website_id UUID,
    name VARCHAR(100),
    scopes TEXT[],
    rate_limit_per_minute INT,
    website_rate_limit INT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        ak.key_id,
        ak.website_id,
        ak.name,
        ak.scopes,
        ak.rate_limit_per_minute,
        w.api_rate_limit_per_minute
    FROM api_keys ak
    JOIN website w ON ak.website_id = w.website_id
    WHERE ak.key_hash = p_key_hash
      AND ak.revoked_at IS NULL
      AND w.deleted_at IS NULL
      AND (ak.expires_at IS NULL OR ak.expires_at > NOW());

    -- Update last_used_at asynchronously (don't block)
    -- This is handled in Go code instead
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION validate_api_key IS 'Validates API key hash and returns key + website info. Returns empty if invalid/revoked/expired.';

-- ============================================================
-- Cleanup Functions
-- ============================================================

CREATE OR REPLACE FUNCTION cleanup_event_idempotency(retention_days INTEGER DEFAULT 7)
RETURNS INTEGER AS $$
DECLARE
    cutoff_date DATE := CURRENT_DATE - retention_days;
    partition_name TEXT;
    dropped_count INTEGER := 0;
BEGIN
    FOR partition_name IN
        SELECT tablename FROM pg_tables
        WHERE schemaname = 'public'
          AND tablename LIKE 'event_idempotency_%'
          AND tablename < 'event_idempotency_' || TO_CHAR(cutoff_date, 'YYYY_MM_DD')
        ORDER BY tablename
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I', partition_name);
        dropped_count := dropped_count + 1;
        RAISE NOTICE 'Dropped old idempotency partition: %', partition_name;
    END LOOP;
    RETURN dropped_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_event_idempotency IS 'Removes idempotency partitions older than retention_days. Called by scheduler.';

CREATE OR REPLACE FUNCTION cleanup_rate_limit_storage()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM rate_limit_storage WHERE e > 0 AND e < EXTRACT(EPOCH FROM NOW());
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_rate_limit_storage IS 'Removes expired rate limit entries. Called by scheduler.';

-- ============================================================
-- Create Future Idempotency Partitions Function
-- ============================================================

CREATE OR REPLACE FUNCTION create_event_idempotency_partitions(days_ahead INTEGER DEFAULT 7)
RETURNS INTEGER AS $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date TEXT;
    end_date TEXT;
    created_count INTEGER := 0;
BEGIN
    FOR i IN 0..days_ahead LOOP
        partition_date := CURRENT_DATE + i;
        partition_name := 'event_idempotency_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
        start_date := TO_CHAR(partition_date, 'YYYY-MM-DD');
        end_date := TO_CHAR(partition_date + 1, 'YYYY-MM-DD');

        BEGIN
            EXECUTE format('
                CREATE TABLE IF NOT EXISTS %I
                PARTITION OF event_idempotency
                FOR VALUES FROM (%L) TO (%L)
            ', partition_name, start_date, end_date);
            created_count := created_count + 1;
        EXCEPTION WHEN duplicate_table THEN
            -- Partition already exists, skip
            NULL;
        END;
    END LOOP;
    RETURN created_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION create_event_idempotency_partitions IS 'Creates future partitions for event_idempotency table. Called by scheduler.';
