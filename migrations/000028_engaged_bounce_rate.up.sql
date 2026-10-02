CREATE OR REPLACE FUNCTION get_dashboard_stats(
    p_website_id UUID,
    p_days INTEGER DEFAULT 1,
    p_country VARCHAR DEFAULT NULL,
    p_browser VARCHAR DEFAULT NULL,
    p_device VARCHAR DEFAULT NULL,
    p_page_path VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    current_visitors BIGINT,
    today_pageviews BIGINT,
    today_visitors BIGINT,
    bounce_rate NUMERIC(5,2)
) AS $$
DECLARE
    v_current_visitors BIGINT;
    v_today_pageviews BIGINT;
    v_today_visitors BIGINT;
    v_bounce_rate NUMERIC(5,2);
    v_bounces BIGINT;
BEGIN
    SELECT COUNT(DISTINCT e.session_id) INTO v_current_visitors
    FROM website_event e
    JOIN session s ON e.session_id = s.session_id
    WHERE e.website_id = p_website_id
      AND e.created_at >= NOW() - INTERVAL '5 minutes'
      AND e.event_type = 1
      AND (p_country IS NULL OR s.country = p_country)
      AND (p_browser IS NULL OR s.browser = p_browser)
      AND (p_device IS NULL OR s.device = p_device)
      AND (p_page_path IS NULL OR e.url_path = p_page_path);

    SELECT COUNT(*) INTO v_today_pageviews
    FROM website_event e
    JOIN session s ON e.session_id = s.session_id
    WHERE e.website_id = p_website_id
      AND e.created_at >= CURRENT_DATE
      AND e.event_type = 1
      AND (p_country IS NULL OR s.country = p_country)
      AND (p_browser IS NULL OR s.browser = p_browser)
      AND (p_device IS NULL OR s.device = p_device)
      AND (p_page_path IS NULL OR e.url_path = p_page_path);

    SELECT COUNT(DISTINCT e.session_id) INTO v_today_visitors
    FROM website_event e
    JOIN session s ON e.session_id = s.session_id
    WHERE e.website_id = p_website_id
      AND e.created_at >= CURRENT_DATE
      AND e.event_type = 1
      AND (p_country IS NULL OR s.country = p_country)
      AND (p_browser IS NULL OR s.browser = p_browser)
      AND (p_device IS NULL OR s.device = p_device)
      AND (p_page_path IS NULL OR e.url_path = p_page_path);

    v_bounce_rate := 0;
    IF v_today_visitors > 0 THEN
        SELECT COUNT(*) INTO v_bounces
        FROM (
            SELECT e.session_id
            FROM website_event e
            JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id
              AND e.created_at >= CURRENT_DATE
              AND e.event_type = 1
              AND (p_country IS NULL OR s.country = p_country)
              AND (p_browser IS NULL OR s.browser = p_browser)
              AND (p_device IS NULL OR s.device = p_device)
              AND (p_page_path IS NULL OR e.url_path = p_page_path)
            GROUP BY e.session_id
            HAVING COUNT(*) = 1
               AND COALESCE(MAX(e.engagement_time), 0) < 10000
               AND COALESCE(MAX(e.scroll_depth), 0) < 50
        ) bounced_sessions;

        v_bounce_rate := (v_bounces::NUMERIC / v_today_visitors::NUMERIC) * 100;
    END IF;

    RETURN QUERY SELECT v_current_visitors, v_today_pageviews, v_today_visitors, v_bounce_rate;
END;
$$ LANGUAGE plpgsql STABLE;
