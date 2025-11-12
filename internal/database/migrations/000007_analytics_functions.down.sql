-- Migration 000007 Down: Revert Enhanced Analytics Functions
-- This rollback removes new functions and reverts get_breakdown() to its original state

-- ============================================================================
-- 1. DROP NEW FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS get_map_data(UUID, INTEGER, VARCHAR, VARCHAR, VARCHAR, VARCHAR);
DROP FUNCTION IF EXISTS get_session_duration(UUID, UUID);
DROP FUNCTION IF EXISTS get_avg_session_duration(UUID, INTEGER, VARCHAR, VARCHAR, VARCHAR);
DROP FUNCTION IF EXISTS get_bounce_sessions(UUID, INTEGER, VARCHAR, VARCHAR, VARCHAR);
DROP FUNCTION IF EXISTS get_page_engagement(UUID, INTEGER, INTEGER, VARCHAR, VARCHAR, VARCHAR);

-- ============================================================================
-- 2. REVERT get_breakdown() to original version (4 dimensions only)
-- ============================================================================

CREATE OR REPLACE FUNCTION get_breakdown(
    p_website_id UUID,
    p_dimension VARCHAR,
    p_days INTEGER DEFAULT 1,
    p_limit INTEGER DEFAULT 10,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL
)
RETURNS TABLE (name VARCHAR, count BIGINT) AS $$
BEGIN
    CASE p_dimension
        WHEN 'country' THEN
            RETURN QUERY
            SELECT COALESCE(s.country, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1
              AND (p_browser IS NULL OR s.browser = p_browser)
              AND (p_device IS NULL OR s.device = p_device)
            GROUP BY s.country
            ORDER BY count DESC
            LIMIT p_limit;

        WHEN 'browser' THEN
            RETURN QUERY
            SELECT COALESCE(s.browser, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1
              AND (p_country IS NULL OR s.country = p_country)
              AND (p_device IS NULL OR s.device = p_device)
            GROUP BY s.browser
            ORDER BY count DESC
            LIMIT p_limit;

        WHEN 'device' THEN
            RETURN QUERY
            SELECT COALESCE(s.device, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1
              AND (p_country IS NULL OR s.country = p_country)
              AND (p_browser IS NULL OR s.browser = p_browser)
            GROUP BY s.device
            ORDER BY count DESC
            LIMIT p_limit;

        WHEN 'referrer' THEN
            RETURN QUERY
            SELECT COALESCE(e.referrer_domain, 'Direct / None')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1
              AND (p_country IS NULL OR s.country = p_country)
              AND (p_browser IS NULL OR s.browser = p_browser)
              AND (p_device IS NULL OR s.device = p_device)
            GROUP BY e.referrer_domain
            ORDER BY count DESC
            LIMIT p_limit;

        ELSE
            RAISE EXCEPTION 'Invalid dimension: %. Must be country, browser, device, or referrer', p_dimension;
    END CASE;
END;
$$ LANGUAGE plpgsql STABLE;
