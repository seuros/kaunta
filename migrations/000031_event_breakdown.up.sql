CREATE FUNCTION get_event_breakdown(
    p_website_id UUID,
    p_days INTEGER DEFAULT 7,
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
    RETURN QUERY
    WITH breakdown_data AS (
        SELECT COALESCE(e.event_name, 'Unknown')::VARCHAR AS dim_name, COUNT(*)::BIGINT AS dim_count
        FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id
          AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
          AND e.event_type = 2
          AND (p_country IS NULL OR s.country = p_country)
          AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device)
          AND (p_page_path IS NULL OR e.url_path = p_page_path)
        GROUP BY e.event_name
    ),
    total_count_cte AS (
        SELECT COUNT(*)::BIGINT AS total FROM breakdown_data
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
END;
$$ LANGUAGE plpgsql STABLE;
