-- Migration 000013: Rollback OS Breakdown Dimension
-- Reverts to previous get_breakdown() function without OS dimension

DROP FUNCTION IF EXISTS get_breakdown(UUID, VARCHAR, INTEGER, INTEGER, INTEGER, VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR);

-- Restore previous version (from migration 000012)
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

        ELSE
            RAISE EXCEPTION 'Invalid dimension: %', p_dimension;
    END CASE;
END;
$$ LANGUAGE plpgsql STABLE;
