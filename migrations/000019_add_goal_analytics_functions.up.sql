-- Migration 000019: Goal Analytics Functions
-- Adds PostgreSQL functions for goal completion analytics

-- ============================================================================
-- 1. get_goal_analytics - Basic conversion metrics
-- ============================================================================

CREATE OR REPLACE FUNCTION get_goal_analytics(
    p_goal_id UUID,
    p_days INTEGER DEFAULT 7,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_page_path VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    completions BIGINT,
    unique_sessions BIGINT,
    conversion_rate NUMERIC,
    total_sessions BIGINT
) AS $$
DECLARE
    v_website_id UUID;
BEGIN
    -- Get website_id from goal
    SELECT website_id INTO v_website_id FROM goals WHERE id = p_goal_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'Goal not found: %', p_goal_id;
    END IF;

    RETURN QUERY
    WITH filtered_completions AS (
        SELECT DISTINCT gc.session_id
        FROM goal_completions gc
        JOIN session s ON gc.session_id = s.session_id
        WHERE gc.goal_id = p_goal_id
          AND gc.completed_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
    ),
    filtered_sessions AS (
        SELECT DISTINCT e.session_id
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = v_website_id
          AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
          AND e.event_type = 1
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
          AND (p_page_path IS NULL OR e.url_path = p_page_path)
    )
    SELECT
        (SELECT COUNT(*) FROM goal_completions WHERE goal_id = p_goal_id
         AND completed_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL)::BIGINT as completions,
        (SELECT COUNT(*) FROM filtered_completions)::BIGINT as unique_sessions,
        CASE
            WHEN (SELECT COUNT(*) FROM filtered_sessions) > 0 THEN
                ROUND((SELECT COUNT(*) FROM filtered_completions)::NUMERIC /
                      (SELECT COUNT(*) FROM filtered_sessions)::NUMERIC * 100, 2)
            ELSE 0
        END as conversion_rate,
        (SELECT COUNT(*) FROM filtered_sessions)::BIGINT as total_sessions;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 2. get_goal_timeseries - Completions over time
-- ============================================================================

CREATE OR REPLACE FUNCTION get_goal_timeseries(
    p_goal_id UUID,
    p_days INTEGER DEFAULT 7,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_page_path VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    time_bucket TIMESTAMPTZ,
    completions BIGINT
) AS $$
DECLARE
    v_interval TEXT;
BEGIN
    -- Use hourly for <= 7 days, daily for > 7 days
    v_interval := CASE WHEN p_days <= 7 THEN 'hour' ELSE 'day' END;

    RETURN QUERY EXECUTE format('
        SELECT
            DATE_TRUNC(%L, gc.completed_at) as bucket,
            COUNT(*)::BIGINT as count
        FROM goal_completions gc
        JOIN session s ON gc.session_id = s.session_id
        WHERE gc.goal_id = $1
          AND gc.completed_at >= CURRENT_DATE - ($2 || '' days'')::INTERVAL
          AND ($3 IS NULL OR s.country = $3)
          AND ($4 IS NULL OR s.browser = $4)
          AND ($5 IS NULL OR s.device = $5)
        GROUP BY bucket
        ORDER BY bucket ASC
    ', v_interval)
    USING p_goal_id, p_days, p_country, p_browser, p_device;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 3. get_goal_breakdown - Breakdown by dimension
-- ============================================================================

CREATE OR REPLACE FUNCTION get_goal_breakdown(
    p_goal_id UUID,
    p_dimension VARCHAR,
    p_days INTEGER DEFAULT 7,
    p_limit INTEGER DEFAULT 10,
    p_offset INTEGER DEFAULT 0,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_page_path VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    name VARCHAR,
    count BIGINT,
    total_count BIGINT
) AS $$
DECLARE
    v_column TEXT;
BEGIN
    -- Map dimension to column name
    v_column := CASE p_dimension
        WHEN 'referrer' THEN 'e.referrer_domain'
        WHEN 'country' THEN 's.country'
        WHEN 'browser' THEN 's.browser'
        WHEN 'device' THEN 's.device'
        WHEN 'os' THEN 's.os'
        WHEN 'page' THEN 'e.url_path'
        ELSE NULL
    END;

    IF v_column IS NULL THEN
        RAISE EXCEPTION 'Invalid dimension: %. Must be referrer, country, browser, device, os, or page', p_dimension;
    END IF;

    RETURN QUERY EXECUTE format('
        WITH base_data AS (
            SELECT %s as dim_value
            FROM goal_completions gc
            JOIN session s ON gc.session_id = s.session_id
            LEFT JOIN website_event e ON gc.event_id = e.event_id
            WHERE gc.goal_id = $1
              AND gc.completed_at >= CURRENT_DATE - ($2 || '' days'')::INTERVAL
              AND ($3 IS NULL OR s.country = $3)
              AND ($4 IS NULL OR s.browser = $4)
              AND ($5 IS NULL OR s.device = $5)
              AND ($6 IS NULL OR e.url_path = $6)
        ),
        aggregated AS (
            SELECT
                COALESCE(dim_value, ''Unknown'')::VARCHAR as dim_name,
                COUNT(*)::BIGINT as dim_count
            FROM base_data
            GROUP BY dim_value
        ),
        total AS (
            SELECT SUM(dim_count)::BIGINT as total_rows FROM aggregated
        )
        SELECT
            a.dim_name,
            a.dim_count,
            t.total_rows
        FROM aggregated a
        CROSS JOIN total t
        ORDER BY a.dim_count DESC
        LIMIT $7 OFFSET $8
    ', v_column)
    USING p_goal_id, p_days, p_country, p_browser, p_device, p_page_path, p_limit, p_offset;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 4. get_goal_converting_pages - Pages visited before conversion
-- ============================================================================

CREATE OR REPLACE FUNCTION get_goal_converting_pages(
    p_goal_id UUID,
    p_days INTEGER DEFAULT 7,
    p_limit INTEGER DEFAULT 10,
    p_offset INTEGER DEFAULT 0,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    page_path VARCHAR,
    conversions BIGINT,
    total_count BIGINT
) AS $$
DECLARE
    v_website_id UUID;
    v_target_url TEXT;
    v_target_event TEXT;
BEGIN
    -- Get goal details
    SELECT website_id, target_url, target_event
    INTO v_website_id, v_target_url, v_target_event
    FROM goals WHERE id = p_goal_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'Goal not found: %', p_goal_id;
    END IF;

    RETURN QUERY
    WITH converting_sessions AS (
        SELECT DISTINCT gc.session_id, gc.completed_at
        FROM goal_completions gc
        JOIN session s ON gc.session_id = s.session_id
        WHERE gc.goal_id = p_goal_id
          AND gc.completed_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
    ),
    pages_before_conversion AS (
        SELECT
            e.url_path,
            cs.session_id
        FROM converting_sessions cs
        JOIN website_event e ON e.session_id = cs.session_id
        WHERE e.website_id = v_website_id
          AND e.event_type = 1
          AND e.url_path IS NOT NULL
          AND e.created_at <= cs.completed_at
          AND e.url_path != COALESCE(v_target_url, '')
    ),
    page_counts AS (
        SELECT
            url_path::VARCHAR as path,
            COUNT(DISTINCT session_id)::BIGINT as conversion_count
        FROM pages_before_conversion
        GROUP BY url_path
    ),
    total AS (
        SELECT SUM(conversion_count)::BIGINT as total_rows FROM page_counts
    )
    SELECT
        pc.path,
        pc.conversion_count,
        t.total_rows
    FROM page_counts pc
    CROSS JOIN total t
    ORDER BY pc.conversion_count DESC
    LIMIT p_limit OFFSET p_offset;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- MIGRATION COMPLETE
-- ============================================================================
