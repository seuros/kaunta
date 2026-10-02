-- Migration 000016: Referrer with full path
-- Changes referrer breakdown to show domain + path instead of just domain
-- Example: reddit.com/r/programming/comments/xyz instead of reddit.com

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

        WHEN 'os' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.os, 'Unknown')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY s.os
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

        -- ====================================================================
        -- REFERRER DIMENSION (MODIFIED)
        -- ====================================================================
        WHEN 'referrer' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT
                    COALESCE(
                        CASE
                            WHEN e.referrer_domain IS NOT NULL THEN
                                e.referrer_domain || COALESCE(e.referrer_path, '')
                            ELSE 'Direct / None'
                        END,
                        'Direct / None'
                    )::VARCHAR as dim_name,
                    COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY e.referrer_domain, e.referrer_path
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

        WHEN 'utm_source' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(e.utm_source, 'Direct / None')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY e.utm_source
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

        WHEN 'utm_medium' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(e.utm_medium, 'Direct / None')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY e.utm_medium
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

        WHEN 'utm_campaign' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(e.utm_campaign, 'Direct / None')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY e.utm_campaign
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

        WHEN 'utm_term' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(e.utm_term, 'Direct / None')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY e.utm_term
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

        WHEN 'utm_content' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(e.utm_content, 'Direct / None')::VARCHAR as dim_name, COUNT(*)::BIGINT as dim_count
                FROM website_event e
                JOIN session s ON e.session_id = s.session_id
                WHERE e.website_id = p_website_id
                  AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND e.event_type = 1
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                  AND (p_page_path IS NULL OR e.url_path = p_page_path)
                GROUP BY e.utm_content
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

        WHEN 'entry_page' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.entry_page, 'Unknown')::VARCHAR as dim_name, COUNT(DISTINCT s.session_id)::BIGINT as dim_count
                FROM session s
                WHERE s.website_id = p_website_id
                  AND s.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND s.entry_page IS NOT NULL
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                GROUP BY s.entry_page
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

        WHEN 'exit_page' THEN
            RETURN QUERY
            WITH breakdown_data AS (
                SELECT COALESCE(s.exit_page, 'Unknown')::VARCHAR as dim_name, COUNT(DISTINCT s.session_id)::BIGINT as dim_count
                FROM session s
                WHERE s.website_id = p_website_id
                  AND s.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
                  AND s.exit_page IS NOT NULL
                  AND (p_country IS NULL OR s.country = p_country)
                  AND (p_browser IS NULL OR s.browser = p_browser)
                  AND (p_device IS NULL OR s.device = p_device)
                GROUP BY s.exit_page
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
            RAISE EXCEPTION 'Invalid dimension: %. Must be country, browser, device, os, referrer, city, region, page, utm_source, utm_medium, utm_campaign, utm_term, utm_content, entry_page, or exit_page', p_dimension;
    END CASE;
END;
$$ LANGUAGE plpgsql STABLE;
