-- Migration 000007: Enhanced Analytics Functions
-- This migration adds new PostgreSQL functions for analytics and enhances existing ones
-- to reduce SQL complexity in Go handlers and improve performance through query plan caching.

-- ============================================================================
-- 1. ENHANCE get_breakdown() - Add support for cities, regions, and pages
-- ============================================================================

CREATE OR REPLACE FUNCTION get_breakdown(
    p_website_id UUID,
    p_dimension VARCHAR,
    p_days INTEGER DEFAULT 1,
    p_limit INTEGER DEFAULT 10,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_page_path VARCHAR DEFAULT NULL
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
              AND (p_page_path IS NULL OR e.url_path = p_page_path)
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
              AND (p_page_path IS NULL OR e.url_path = p_page_path)
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
              AND (p_page_path IS NULL OR e.url_path = p_page_path)
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
              AND (p_page_path IS NULL OR e.url_path = p_page_path)
            GROUP BY e.referrer_domain
            ORDER BY count DESC
            LIMIT p_limit;

        -- NEW: Cities dimension
        WHEN 'city' THEN
            RETURN QUERY
            SELECT COALESCE(s.city, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1
              AND (p_country IS NULL OR s.country = p_country)
              AND (p_browser IS NULL OR s.browser = p_browser)
              AND (p_device IS NULL OR s.device = p_device)
              AND (p_page_path IS NULL OR e.url_path = p_page_path)
            GROUP BY s.city
            ORDER BY count DESC
            LIMIT p_limit;

        -- NEW: Regions dimension
        WHEN 'region' THEN
            RETURN QUERY
            SELECT COALESCE(s.region, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1
              AND (p_country IS NULL OR s.country = p_country)
              AND (p_browser IS NULL OR s.browser = p_browser)
              AND (p_device IS NULL OR s.device = p_device)
              AND (p_page_path IS NULL OR e.url_path = p_page_path)
            GROUP BY s.region
            ORDER BY count DESC
            LIMIT p_limit;

        -- NEW: Pages dimension
        WHEN 'page' THEN
            RETURN QUERY
            SELECT COALESCE(e.url_path, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1
              AND e.url_path IS NOT NULL
              AND (p_country IS NULL OR s.country = p_country)
              AND (p_browser IS NULL OR s.browser = p_browser)
              AND (p_device IS NULL OR s.device = p_device)
            GROUP BY e.url_path
            ORDER BY count DESC
            LIMIT p_limit;

        ELSE
            RAISE EXCEPTION 'Invalid dimension: %. Must be country, browser, device, referrer, city, region, or page', p_dimension;
    END CASE;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 2. NEW: get_map_data() - Map visualization with percentage calculations
-- ============================================================================

CREATE OR REPLACE FUNCTION get_map_data(
    p_website_id UUID,
    p_days INTEGER DEFAULT 7,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_page_path VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    country VARCHAR,
    visitors BIGINT,
    percentage NUMERIC(5,2)
) AS $$
BEGIN
    RETURN QUERY
    WITH total_visitors AS (
        SELECT COUNT(DISTINCT e.session_id)::BIGINT as total
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id
          AND e.created_at >= NOW() - (p_days || ' days')::INTERVAL
          AND e.event_type = 1
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
          AND (p_page_path IS NULL OR e.url_path = p_page_path)
    ),
    country_breakdown AS (
        SELECT
            COALESCE(s.country, 'Unknown')::VARCHAR as country_code,
            COUNT(DISTINCT e.session_id)::BIGINT as visitor_count
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id
          AND e.created_at >= NOW() - (p_days || ' days')::INTERVAL
          AND e.event_type = 1
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
          AND (p_page_path IS NULL OR e.url_path = p_page_path)
        GROUP BY s.country
    )
    SELECT
        cb.country_code,
        cb.visitor_count,
        CASE
            WHEN tv.total > 0 THEN ROUND((cb.visitor_count::NUMERIC / tv.total::NUMERIC * 100), 2)
            ELSE 0
        END as pct
    FROM country_breakdown cb
    CROSS JOIN total_visitors tv
    ORDER BY cb.visitor_count DESC;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 3. NEW: Session Analytics Functions
-- ============================================================================

-- 3.1 Get duration of a specific session
CREATE OR REPLACE FUNCTION get_session_duration(
    p_website_id UUID,
    p_session_id UUID
)
RETURNS INTERVAL AS $$
DECLARE
    v_duration INTERVAL;
BEGIN
    SELECT MAX(created_at) - MIN(created_at) INTO v_duration
    FROM website_event
    WHERE website_id = p_website_id
      AND session_id = p_session_id
      AND event_type = 1;

    RETURN COALESCE(v_duration, INTERVAL '0 seconds');
END;
$$ LANGUAGE plpgsql STABLE;

-- 3.2 Get average session duration across all sessions with filters
CREATE OR REPLACE FUNCTION get_avg_session_duration(
    p_website_id UUID,
    p_days INTEGER DEFAULT 7,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL
)
RETURNS INTERVAL AS $$
DECLARE
    v_avg_duration INTERVAL;
BEGIN
    WITH session_durations AS (
        SELECT
            e.session_id,
            MAX(e.created_at) - MIN(e.created_at) as duration
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id
          AND e.created_at >= NOW() - (p_days || ' days')::INTERVAL
          AND e.event_type = 1
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
        GROUP BY e.session_id
        HAVING COUNT(*) > 1  -- Exclude single-page sessions from average
    )
    SELECT AVG(duration) INTO v_avg_duration
    FROM session_durations;

    RETURN COALESCE(v_avg_duration, INTERVAL '0 seconds');
END;
$$ LANGUAGE plpgsql STABLE;

-- 3.3 Get all bounced sessions (sessions with exactly 1 pageview)
CREATE OR REPLACE FUNCTION get_bounce_sessions(
    p_website_id UUID,
    p_days INTEGER DEFAULT 7,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    session_id UUID,
    created_at TIMESTAMPTZ,
    country VARCHAR,
    browser VARCHAR,
    device VARCHAR,
    url_path VARCHAR
) AS $$
BEGIN
    RETURN QUERY
    WITH session_pageview_counts AS (
        SELECT
            e.session_id,
            COUNT(*) as pageview_count
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id
          AND e.created_at >= NOW() - (p_days || ' days')::INTERVAL
          AND e.event_type = 1
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
        GROUP BY e.session_id
        HAVING COUNT(*) = 1
    )
    SELECT
        e.session_id,
        e.created_at,
        COALESCE(s.country, 'Unknown')::VARCHAR,
        COALESCE(s.browser, 'Unknown')::VARCHAR,
        COALESCE(s.device, 'Unknown')::VARCHAR,
        COALESCE(e.url_path, 'Unknown')::VARCHAR
    FROM website_event e
    JOIN session s ON e.session_id = s.session_id
    JOIN session_pageview_counts spc ON e.session_id = spc.session_id
    WHERE e.website_id = p_website_id
      AND e.event_type = 1
    ORDER BY e.created_at DESC;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 4. NEW: Get engagement metrics per page
-- ============================================================================

CREATE OR REPLACE FUNCTION get_page_engagement(
    p_website_id UUID,
    p_days INTEGER DEFAULT 7,
    p_limit INTEGER DEFAULT 10,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    page_path VARCHAR,
    pageviews BIGINT,
    unique_visitors BIGINT,
    avg_engagement_time NUMERIC,
    avg_scroll_depth NUMERIC,
    bounce_rate NUMERIC
) AS $$
BEGIN
    RETURN QUERY
    WITH page_stats AS (
        SELECT
            e.url_path,
            COUNT(*)::BIGINT as views,
            COUNT(DISTINCT e.session_id)::BIGINT as visitors,
            AVG(e.engagement_time) as avg_time,
            AVG(e.scroll_depth) as avg_scroll,
            COUNT(DISTINCT CASE
                WHEN (
                    SELECT COUNT(*)
                    FROM website_event e2
                    WHERE e2.session_id = e.session_id
                      AND e2.website_id = p_website_id
                      AND e2.event_type = 1
                ) = 1 THEN e.session_id
            END)::BIGINT as bounce_count
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id
          AND e.created_at >= NOW() - (p_days || ' days')::INTERVAL
          AND e.event_type = 1
          AND e.url_path IS NOT NULL
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
        GROUP BY e.url_path
    )
    SELECT
        COALESCE(url_path, 'Unknown')::VARCHAR,
        views,
        visitors,
        ROUND(COALESCE(avg_time, 0), 0),
        ROUND(COALESCE(avg_scroll, 0), 1),
        CASE
            WHEN visitors > 0 THEN ROUND((bounce_count::NUMERIC / visitors::NUMERIC * 100), 1)
            ELSE 0
        END
    FROM page_stats
    ORDER BY views DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- MIGRATION COMPLETE
-- ============================================================================
-- Functions created:
-- 1. get_breakdown() - Enhanced with city, region, page support
-- 2. get_map_data() - Single query for map visualization with percentages
-- 3. get_session_duration() - Duration of a specific session
-- 4. get_avg_session_duration() - Average session duration with filters
-- 5. get_bounce_sessions() - List all bounced sessions
-- 6. get_page_engagement() - Comprehensive page metrics with bounce rate
