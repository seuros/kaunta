-- Migration 000011: Rollback UTM Campaign Parameter Tracking
-- Removes UTM columns and restores previous get_breakdown() function

-- ============================================================================
-- 1. DROP UTM INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_website_event_utm_source;
DROP INDEX IF EXISTS idx_website_event_utm_medium;
DROP INDEX IF EXISTS idx_website_event_utm_campaign;

-- ============================================================================
-- 2. DROP UTM COLUMNS FROM website_event
-- ============================================================================

ALTER TABLE website_event DROP COLUMN IF EXISTS utm_source;
ALTER TABLE website_event DROP COLUMN IF EXISTS utm_medium;
ALTER TABLE website_event DROP COLUMN IF EXISTS utm_campaign;
ALTER TABLE website_event DROP COLUMN IF EXISTS utm_term;
ALTER TABLE website_event DROP COLUMN IF EXISTS utm_content;

-- ============================================================================
-- 3. RESTORE get_breakdown() FROM MIGRATION 000010 (without UTM dimensions)
-- ============================================================================

DROP FUNCTION IF EXISTS get_breakdown(UUID, VARCHAR, INTEGER, INTEGER, INTEGER, VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR);

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
-- ROLLBACK COMPLETE
-- ============================================================================
