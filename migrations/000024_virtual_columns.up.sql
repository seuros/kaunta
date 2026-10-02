-- PostgreSQL 18: Add virtual generated columns
-- Virtual columns compute values on-read with zero storage overhead
-- Note: TIMESTAMPTZ expressions must use AT TIME ZONE 'UTC' to be immutable

-- website_event: derived time fields (UTC-based for immutability)
ALTER TABLE website_event ADD COLUMN IF NOT EXISTS
    event_date DATE GENERATED ALWAYS AS (DATE(created_at AT TIME ZONE 'UTC')) VIRTUAL;

ALTER TABLE website_event ADD COLUMN IF NOT EXISTS
    event_hour SMALLINT GENERATED ALWAYS AS (EXTRACT(HOUR FROM created_at AT TIME ZONE 'UTC')::SMALLINT) VIRTUAL;

ALTER TABLE website_event ADD COLUMN IF NOT EXISTS
    is_custom_event BOOLEAN GENERATED ALWAYS AS (event_type = 2) VIRTUAL;

-- session: device type flags
ALTER TABLE session ADD COLUMN IF NOT EXISTS
    is_mobile BOOLEAN GENERATED ALWAYS AS (device = 'mobile') VIRTUAL;

ALTER TABLE session ADD COLUMN IF NOT EXISTS
    is_desktop BOOLEAN GENERATED ALWAYS AS (device = 'desktop') VIRTUAL;

ALTER TABLE session ADD COLUMN IF NOT EXISTS
    is_tablet BOOLEAN GENERATED ALWAYS AS (device = 'tablet') VIRTUAL;

ALTER TABLE session ADD COLUMN IF NOT EXISTS
    has_location BOOLEAN GENERATED ALWAYS AS (country IS NOT NULL) VIRTUAL;

-- ip_metadata: bot classification
ALTER TABLE ip_metadata ADD COLUMN IF NOT EXISTS
    is_high_confidence_bot BOOLEAN GENERATED ALWAYS AS (is_bot AND confidence >= 80) VIRTUAL;

-- Comments
COMMENT ON COLUMN website_event.event_date IS 'Virtual: DATE(created_at AT TIME ZONE UTC) - zero storage';
COMMENT ON COLUMN website_event.event_hour IS 'Virtual: hour of day in UTC (0-23) - zero storage';
COMMENT ON COLUMN website_event.is_custom_event IS 'Virtual: true if event_type = 2 - zero storage';
COMMENT ON COLUMN session.is_mobile IS 'Virtual: device = mobile - zero storage';
COMMENT ON COLUMN session.is_desktop IS 'Virtual: device = desktop - zero storage';
COMMENT ON COLUMN session.is_tablet IS 'Virtual: device = tablet - zero storage';
COMMENT ON COLUMN session.has_location IS 'Virtual: country IS NOT NULL - zero storage';
COMMENT ON COLUMN ip_metadata.is_high_confidence_bot IS 'Virtual: is_bot AND confidence >= 80 - zero storage';
