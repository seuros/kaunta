-- Migration 000010: Add Sorting Support to Analytics Functions
-- This migration adds sort column and direction parameters to analytics functions

-- ============================================================================
-- 1. UPDATE get_top_pages() with sorting support
-- ============================================================================

DROP FUNCTION IF EXISTS get_top_pages(UUID, INTEGER, INTEGER, INTEGER, VARCHAR, VARCHAR, VARCHAR);

CREATE OR REPLACE FUNCTION get_top_pages(
    p_website_id UUID,
    p_days INTEGER DEFAULT 1,
    p_limit INTEGER DEFAULT 10,
    p_offset INTEGER DEFAULT 0,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_sort_by VARCHAR DEFAULT 'views',
    p_sort_order VARCHAR DEFAULT 'desc'
)
RETURNS TABLE (
    path VARCHAR,
    views BIGINT,
    unique_visitors BIGINT,
    avg_engagement_time NUMERIC,
    total_count BIGINT
) AS $$
BEGIN
    RETURN QUERY
    WITH filtered_events AS (
        SELECT e.url_path, e.session_id, e.engagement_time
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id
          AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
          AND e.event_type = 1
          AND e.url_path IS NOT NULL
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
    ),
    page_stats AS (
        SELECT
            fe.url_path,
            COUNT(*)::BIGINT as view_count,
            COUNT(DISTINCT fe.session_id)::BIGINT as unique_visitor_count,
            ROUND(AVG(COALESCE(fe.engagement_time, 0)), 0) as avg_time
        FROM filtered_events fe
        GROUP BY fe.url_path
    ),
    total_count_cte AS (
        SELECT COUNT(*)::BIGINT as total FROM page_stats
    )
    SELECT
        ps.url_path::VARCHAR,
        ps.view_count,
        ps.unique_visitor_count,
        ps.avg_time,
        tc.total as total_count
    FROM page_stats ps
    CROSS JOIN total_count_cte tc
    ORDER BY
        CASE WHEN p_sort_order = 'desc' THEN
            CASE p_sort_by
                WHEN 'views' THEN ps.view_count
                WHEN 'unique_visitors' THEN ps.unique_visitor_count
                WHEN 'avg_engagement_time' THEN ps.avg_time::BIGINT
                ELSE ps.view_count
            END
        END DESC NULLS LAST,
        CASE WHEN p_sort_order = 'asc' THEN
            CASE p_sort_by
                WHEN 'views' THEN ps.view_count
                WHEN 'unique_visitors' THEN ps.unique_visitor_count
                WHEN 'avg_engagement_time' THEN ps.avg_time::BIGINT
                ELSE ps.view_count
            END
        END ASC NULLS LAST,
        CASE WHEN p_sort_by = 'path' AND p_sort_order = 'desc' THEN ps.url_path END DESC NULLS LAST,
        CASE WHEN p_sort_by = 'path' AND p_sort_order = 'asc' THEN ps.url_path END ASC NULLS LAST
    LIMIT p_limit
    OFFSET p_offset;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 2. UPDATE get_breakdown() with sorting support
-- ============================================================================

DROP FUNCTION IF EXISTS get_breakdown(UUID, VARCHAR, INTEGER, INTEGER, INTEGER, VARCHAR, VARCHAR, VARCHAR, VARCHAR);

CREATE OR REPLACE FUNCTION get_breakdown(
    p_website_id UUID,
    p_dimension VARCHAR,
    p_days INTEGER DEFAULT 1,
    p_limit INTEGER DEFAULT 10,
    p_offset INTEGER DEFAULT 0,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_page_path VARCHAR DEFAULT NULL,
    p_sort_by VARCHAR DEFAULT 'count',
    p_sort_order VARCHAR DEFAULT 'desc'
)
RETURNS TABLE (name VARCHAR, count BIGINT, total_count BIGINT) AS $$
BEGIN
    CASE p_dimension
        WHEN 'country' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.country, 'Unknown')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY s.country
            ),
            total_count_cte AS (
                SELECT COUNT(*)::BIGINT as total FROM breakdown_data
            )
            SELECT bd.dim_name, bd.dim_count, tc.total
            FROM breakdown_data bd
            CROSS JOIN total_count_cte tc
            ORDER BY
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'desc' THEN bd.dim_count END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'asc' THEN bd.dim_count END ASC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'desc' THEN bd.dim_name END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'asc' THEN bd.dim_name END ASC NULLS LAST
            LIMIT p_limit
            OFFSET p_offset;

        WHEN 'browser' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.browser, 'Unknown')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY s.browser
            ),
            total_count_cte AS (
                SELECT COUNT(*)::BIGINT as total FROM breakdown_data
            )
            SELECT bd.dim_name, bd.dim_count, tc.total
            FROM breakdown_data bd
            CROSS JOIN total_count_cte tc
            ORDER BY
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'desc' THEN bd.dim_count END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'asc' THEN bd.dim_count END ASC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'desc' THEN bd.dim_name END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'asc' THEN bd.dim_name END ASC NULLS LAST
            LIMIT p_limit
            OFFSET p_offset;

        WHEN 'device' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.device, 'Unknown')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY s.device
            ),
            total_count_cte AS (
                SELECT COUNT(*)::BIGINT as total FROM breakdown_data
            )
            SELECT bd.dim_name, bd.dim_count, tc.total
            FROM breakdown_data bd
            CROSS JOIN total_count_cte tc
            ORDER BY
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'desc' THEN bd.dim_count END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'asc' THEN bd.dim_count END ASC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'desc' THEN bd.dim_name END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'asc' THEN bd.dim_name END ASC NULLS LAST
            LIMIT p_limit
            OFFSET p_offset;

        WHEN 'referrer' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(e.referrer_domain, 'Direct / None')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
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
            ),
            total_count_cte AS (
                SELECT COUNT(*)::BIGINT as total FROM breakdown_data
            )
            SELECT bd.dim_name, bd.dim_count, tc.total
            FROM breakdown_data bd
            CROSS JOIN total_count_cte tc
            ORDER BY
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'desc' THEN bd.dim_count END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'asc' THEN bd.dim_count END ASC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'desc' THEN bd.dim_name END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'asc' THEN bd.dim_name END ASC NULLS LAST
            LIMIT p_limit
            OFFSET p_offset;

        WHEN 'city' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.city, 'Unknown')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
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
            ),
            total_count_cte AS (
                SELECT COUNT(*)::BIGINT as total FROM breakdown_data
            )
            SELECT bd.dim_name, bd.dim_count, tc.total
            FROM breakdown_data bd
            CROSS JOIN total_count_cte tc
            ORDER BY
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'desc' THEN bd.dim_count END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'asc' THEN bd.dim_count END ASC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'desc' THEN bd.dim_name END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'asc' THEN bd.dim_name END ASC NULLS LAST
            LIMIT p_limit
            OFFSET p_offset;

        WHEN 'region' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.region, 'Unknown')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
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
            ),
            total_count_cte AS (
                SELECT COUNT(*)::BIGINT as total FROM breakdown_data
            )
            SELECT bd.dim_name, bd.dim_count, tc.total
            FROM breakdown_data bd
            CROSS JOIN total_count_cte tc
            ORDER BY
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'desc' THEN bd.dim_count END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'asc' THEN bd.dim_count END ASC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'desc' THEN bd.dim_name END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'asc' THEN bd.dim_name END ASC NULLS LAST
            LIMIT p_limit
            OFFSET p_offset;

        WHEN 'page' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(e.url_path, 'Unknown')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
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
            ),
            total_count_cte AS (
                SELECT COUNT(*)::BIGINT as total FROM breakdown_data
            )
            SELECT bd.dim_name, bd.dim_count, tc.total
            FROM breakdown_data bd
            CROSS JOIN total_count_cte tc
            ORDER BY
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'desc' THEN bd.dim_count END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'count' AND p_sort_order = 'asc' THEN bd.dim_count END ASC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'desc' THEN bd.dim_name END DESC NULLS LAST,
                CASE WHEN p_sort_by = 'name' AND p_sort_order = 'asc' THEN bd.dim_name END ASC NULLS LAST
            LIMIT p_limit
            OFFSET p_offset;

        ELSE
            RAISE EXCEPTION 'Invalid dimension: %. Must be country, browser, device, referrer, city, region, or page', p_dimension;
    END CASE;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- MIGRATION COMPLETE
-- ============================================================================
