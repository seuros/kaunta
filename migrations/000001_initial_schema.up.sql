-- Kaunta Initial Schema (PostgreSQL 18+)
-- Squashed migration combining all initialization steps
-- Idempotent: Safe to run on existing Umami databases or fresh installs

-- ============================================================
-- PART 1: Extensions
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- PART 2: Core Tables (Website, Session, Events)
-- ============================================================

-- Website table
CREATE TABLE IF NOT EXISTS website (
    website_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    domain VARCHAR(500) NOT NULL,
    name VARCHAR(100),
    share_id VARCHAR(50) UNIQUE,
    allowed_domains JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS website_domain_idx ON website(domain);
CREATE INDEX IF NOT EXISTS website_share_id_idx ON website(share_id);
CREATE INDEX IF NOT EXISTS website_allowed_domains_idx ON website USING gin(allowed_domains);

-- Session table
CREATE TABLE IF NOT EXISTS session (
    session_id UUID PRIMARY KEY,
    website_id UUID NOT NULL,
    hostname VARCHAR(100),
    browser VARCHAR(20),
    os VARCHAR(20),
    device VARCHAR(20),
    screen VARCHAR(11),
    language VARCHAR(35),
    country CHAR(2),
    subdivision1 VARCHAR(20),
    subdivision2 VARCHAR(50),
    city VARCHAR(50),
    region VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    distinct_id VARCHAR(500),
    CONSTRAINT session_website_id_fkey FOREIGN KEY (website_id) REFERENCES website(website_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS session_created_at_idx ON session(created_at);
CREATE INDEX IF NOT EXISTS session_website_id_idx ON session(website_id);
CREATE INDEX IF NOT EXISTS session_website_id_created_at_idx ON session(website_id, created_at);
CREATE INDEX IF NOT EXISTS idx_session_website_created ON session (website_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_country ON session (website_id, country) WHERE country IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_session_browser ON session (website_id, browser) WHERE browser IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_session_device ON session (website_id, device) WHERE device IS NOT NULL;

-- Partitioned Website Event table (PostgreSQL 18+)
CREATE TABLE IF NOT EXISTS website_event (
    event_id UUID DEFAULT gen_random_uuid(),
    website_id UUID NOT NULL,
    session_id UUID NOT NULL,
    visit_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    url_path VARCHAR(500),
    url_query VARCHAR(500),
    referrer_path VARCHAR(500),
    referrer_query VARCHAR(500),
    referrer_domain VARCHAR(500),
    page_title VARCHAR(500),
    hostname VARCHAR(100),
    event_type SMALLINT NOT NULL DEFAULT 1,
    event_name VARCHAR(50),
    tag VARCHAR(50),
    scroll_depth SMALLINT,
    engagement_time INTEGER,
    props JSONB,
    PRIMARY KEY (event_id, created_at),
    CONSTRAINT website_event_website_id_fkey FOREIGN KEY (website_id) REFERENCES website(website_id) ON DELETE CASCADE,
    CONSTRAINT website_event_session_id_fkey FOREIGN KEY (session_id) REFERENCES session(session_id) ON DELETE CASCADE,
    CONSTRAINT valid_event_type CHECK (event_type IN (1, 2))
) PARTITION BY RANGE (created_at);

-- Create initial partitions (30 days forward + 7 days back)
DO $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date TEXT;
    end_date TEXT;
BEGIN
    FOR i IN -7..30 LOOP
        partition_date := CURRENT_DATE + i;
        partition_name := 'website_event_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
        start_date := TO_CHAR(partition_date, 'YYYY-MM-DD');
        end_date := TO_CHAR(partition_date + 1, 'YYYY-MM-DD');

        EXECUTE format('
            CREATE TABLE IF NOT EXISTS %I
            PARTITION OF website_event
            FOR VALUES FROM (%L) TO (%L)
        ', partition_name, start_date, end_date);
    END LOOP;
END $$;

-- Event table indexes (optimized for PostgreSQL 18)
CREATE INDEX IF NOT EXISTS idx_event_website_created ON website_event (website_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_session_created ON website_event (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_event_url_path ON website_event (url_path) WHERE event_type = 1 AND url_path IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_event_referrer_domain ON website_event (referrer_domain) WHERE referrer_domain IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_event_props_gin ON website_event USING gin (props jsonb_path_ops) WHERE props IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_event_created_brin ON website_event USING brin (created_at) WITH (pages_per_range = 128);
CREATE INDEX IF NOT EXISTS idx_event_type ON website_event (website_id, event_type, created_at);
CREATE INDEX IF NOT EXISTS idx_event_custom ON website_event (website_id, event_name, created_at) WHERE event_type = 2;

-- ============================================================
-- PART 3: Bot Detection Tables and Functions
-- ============================================================

CREATE TABLE IF NOT EXISTS ip_metadata (
    ip inet PRIMARY KEY,
    first_seen timestamptz NOT NULL DEFAULT NOW(),
    last_seen timestamptz NOT NULL DEFAULT NOW(),
    total_requests bigint NOT NULL DEFAULT 1,
    requests_last_hour integer NOT NULL DEFAULT 1,
    requests_last_minute integer NOT NULL DEFAULT 1,
    is_bot boolean NOT NULL DEFAULT false,
    bot_type varchar(50),
    confidence smallint DEFAULT 0,
    detection_reason text,
    avg_requests_per_minute numeric(10,2),
    max_requests_per_minute integer DEFAULT 0,
    unique_user_agents smallint DEFAULT 1,
    user_agent_sample text[],
    asn integer,
    asn_org varchar(255),
    is_hosting_provider boolean DEFAULT false,
    is_vpn boolean DEFAULT false,
    is_tor boolean DEFAULT false,
    country char(2),
    updated_at timestamptz DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ip_metadata_bot ON ip_metadata (is_bot, last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_ip_metadata_country ON ip_metadata (country) WHERE country IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ip_metadata_asn ON ip_metadata (asn) WHERE asn IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ip_metadata_requests ON ip_metadata (requests_last_minute DESC) WHERE requests_last_minute > 10;

-- Bot detection log (partitioned by date)
CREATE TABLE IF NOT EXISTS bot_detection_log (
    log_id bigserial,
    ip inet NOT NULL,
    detected_at timestamptz NOT NULL DEFAULT NOW(),
    pattern_type varchar(50) NOT NULL,
    confidence smallint NOT NULL,
    details jsonb,
    user_agent text,
    website_id uuid,
    PRIMARY KEY (log_id, detected_at)
) PARTITION BY RANGE (detected_at);

-- Create initial partitions for bot log
DO $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date TEXT;
    end_date TEXT;
BEGIN
    FOR i IN 0..7 LOOP
        partition_date := CURRENT_DATE + i;
        partition_name := 'bot_detection_log_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
        start_date := TO_CHAR(partition_date, 'YYYY-MM-DD');
        end_date := TO_CHAR(partition_date + 1, 'YYYY-MM-DD');

        EXECUTE format('
            CREATE TABLE IF NOT EXISTS %I
            PARTITION OF bot_detection_log
            FOR VALUES FROM (%L) TO (%L)
        ', partition_name, start_date, end_date);
    END LOOP;
END $$;

CREATE INDEX IF NOT EXISTS idx_bot_log_ip ON bot_detection_log (ip, detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_bot_log_pattern ON bot_detection_log (pattern_type, detected_at DESC);

-- Bot user agent patterns
CREATE TABLE IF NOT EXISTS bot_user_agent_patterns (
    pattern_id serial PRIMARY KEY,
    pattern_name varchar(100) NOT NULL UNIQUE,
    pattern_regex text NOT NULL,
    bot_type varchar(50) NOT NULL,
    is_legitimate boolean DEFAULT true,
    notes text
);

INSERT INTO bot_user_agent_patterns (pattern_name, pattern_regex, bot_type, is_legitimate, notes) VALUES
('ChatGPT-User', 'ChatGPT-User', 'llm_crawler', true, 'OpenAI ChatGPT user agent'),
('GPTBot', 'GPTBot', 'llm_crawler', true, 'OpenAI GPT crawler'),
('Claude-Web', 'Claude-Web|anthropic-ai', 'llm_crawler', true, 'Anthropic Claude web crawler'),
('PerplexityBot', 'PerplexityBot|Perplexity', 'llm_crawler', true, 'Perplexity AI search crawler'),
('Google-Extended', 'Google-Extended', 'llm_crawler', true, 'Google AI/SGE crawler'),
('Bytespider', 'Bytespider', 'llm_crawler', true, 'ByteDance (TikTok) AI crawler'),
('Claude', 'Claude', 'llm_crawler', true, 'Anthropic Claude AI'),
('Bard', 'Bard|Google-Bard', 'llm_crawler', true, 'Google Bard (Gemini)'),
('Playwright', 'Playwright|playwright', 'headless_browser', false, 'Playwright headless browser - can run JS'),
('Puppeteer', 'Puppeteer|HeadlessChrome', 'headless_browser', false, 'Puppeteer/Headless Chrome - can run JS'),
('Selenium', 'Selenium|selenium', 'headless_browser', false, 'Selenium WebDriver - can run JS'),
('PhantomJS', 'PhantomJS|phantom', 'headless_browser', false, 'PhantomJS headless browser - can run JS'),
('Splash', 'Splash', 'headless_browser', false, 'Splash rendering service - can run JS'),
('Zombie', 'Zombie.js|zombie', 'headless_browser', false, 'Zombie.js headless browser'),
('axios', 'axios/', 'scraper', false, 'Axios HTTP client (Node.js) - often used in scrapers'),
('node-fetch', 'node-fetch', 'scraper', false, 'Node.js fetch - often used in scrapers'),
('python-requests', 'python-requests/', 'scraper', false, 'Python requests library'),
('Go-http-client', 'Go-http-client/', 'scraper', false, 'Go HTTP client'),
('Java', '^Java/', 'scraper', false, 'Java HTTP client'),
('generic_bot', 'bot|crawler|spider|scraper', 'generic_bot', false, 'Generic bot/crawler/spider pattern')
ON CONFLICT (pattern_name) DO NOTHING;

-- Real-time stats cache
CREATE TABLE IF NOT EXISTS realtime_stats_cache (
    website_id UUID PRIMARY KEY,
    current_visitors INTEGER DEFAULT 0,
    pageviews_today INTEGER DEFAULT 0,
    visitors_today INTEGER DEFAULT 0,
    last_updated TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT realtime_stats_cache_website_fkey FOREIGN KEY (website_id) REFERENCES website(website_id) ON DELETE CASCADE
);

-- ============================================================
-- PART 4: Materialized Views
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS daily_website_stats AS
WITH base_events AS (
    SELECT website_id, DATE(created_at) as date, created_at, session_id, visit_id, event_type, engagement_time, scroll_depth, url_path, referrer_domain
    FROM website_event
    WHERE created_at >= CURRENT_DATE - INTERVAL '90 days' AND event_type = 1
),
page_counts AS (
    SELECT website_id, date, url_path, COUNT(*) as page_count
    FROM base_events WHERE url_path IS NOT NULL
    GROUP BY website_id, date, url_path
),
ranked_pages AS (
    SELECT website_id, date, url_path, page_count,
           ROW_NUMBER() OVER (PARTITION BY website_id, date ORDER BY page_count DESC) as page_rank
    FROM page_counts
),
referrer_counts AS (
    SELECT website_id, date, referrer_domain, COUNT(*) as referrer_count
    FROM base_events WHERE referrer_domain IS NOT NULL
    GROUP BY website_id, date, referrer_domain
),
ranked_referrers AS (
    SELECT website_id, date, referrer_domain, referrer_count,
           ROW_NUMBER() OVER (PARTITION BY website_id, date ORDER BY referrer_count DESC) as referrer_rank
    FROM referrer_counts
),
aggregated_metrics AS (
    SELECT website_id, date, COUNT(*) as pageviews, COUNT(DISTINCT session_id) as sessions,
           COUNT(DISTINCT visit_id) as visits, AVG(engagement_time) FILTER (WHERE engagement_time IS NOT NULL) as avg_engagement_time,
           AVG(scroll_depth) FILTER (WHERE scroll_depth IS NOT NULL) as avg_scroll_depth,
           COUNT(*) FILTER (WHERE event_type = 1) as pageview_count, COUNT(*) FILTER (WHERE event_type = 2) as custom_event_count
    FROM base_events GROUP BY website_id, date
)
SELECT am.website_id, am.date, am.pageviews, am.sessions, am.visits, am.avg_engagement_time, am.avg_scroll_depth,
       am.pageview_count, am.custom_event_count,
       (SELECT jsonb_object_agg(url_path, page_count) FROM ranked_pages rp WHERE rp.website_id = am.website_id AND rp.date = am.date AND rp.page_rank <= 10) as top_pages,
       (SELECT jsonb_object_agg(referrer_domain, referrer_count) FROM ranked_referrers rr WHERE rr.website_id = am.website_id AND rr.date = am.date AND rr.referrer_rank <= 10) as top_referrers
FROM aggregated_metrics am;

CREATE UNIQUE INDEX IF NOT EXISTS idx_daily_stats_pk ON daily_website_stats (website_id, date);

CREATE MATERIALIZED VIEW IF NOT EXISTS hourly_website_stats AS
WITH base_events AS (
    SELECT website_id, DATE_TRUNC('hour', created_at) as hour, created_at, session_id, visit_id, event_type, engagement_time, url_path
    FROM website_event WHERE created_at >= NOW() - INTERVAL '48 hours' AND event_type = 1
),
page_counts AS (
    SELECT website_id, hour, url_path, COUNT(*) as page_count
    FROM base_events WHERE url_path IS NOT NULL
    GROUP BY website_id, hour, url_path
),
ranked_pages AS (
    SELECT website_id, hour, url_path, page_count,
           ROW_NUMBER() OVER (PARTITION BY website_id, hour ORDER BY page_count DESC) as page_rank
    FROM page_counts
),
aggregated_metrics AS (
    SELECT website_id, hour, COUNT(*) as pageviews, COUNT(DISTINCT session_id) as sessions,
           COUNT(DISTINCT visit_id) as visits, COUNT(*) FILTER (WHERE event_type = 2) as custom_events,
           AVG(engagement_time) FILTER (WHERE engagement_time IS NOT NULL) as avg_engagement_time
    FROM base_events GROUP BY website_id, hour
)
SELECT am.website_id, am.hour, am.pageviews, am.sessions, am.visits, am.custom_events, am.avg_engagement_time,
       (SELECT jsonb_object_agg(url_path, page_count) FROM ranked_pages rp WHERE rp.website_id = am.website_id AND rp.hour = am.hour AND rp.page_rank <= 5) as top_pages
FROM aggregated_metrics am;

CREATE UNIQUE INDEX IF NOT EXISTS idx_hourly_stats_pk ON hourly_website_stats (website_id, hour);

CREATE MATERIALIZED VIEW IF NOT EXISTS realtime_website_stats AS
WITH base_events AS (
    SELECT website_id, session_id, url_path FROM website_event WHERE created_at >= NOW() - INTERVAL '5 minutes' AND event_type = 1
),
page_counts AS (
    SELECT website_id, url_path, COUNT(*) as view_count FROM base_events WHERE url_path IS NOT NULL GROUP BY website_id, url_path
),
ranked_pages AS (
    SELECT website_id, url_path, view_count, ROW_NUMBER() OVER (PARTITION BY website_id ORDER BY view_count DESC) as page_rank FROM page_counts
),
aggregated_metrics AS (
    SELECT website_id, COUNT(DISTINCT session_id) as current_visitors, COUNT(*) as recent_pageviews FROM base_events GROUP BY website_id
)
SELECT am.website_id, am.current_visitors, am.recent_pageviews,
       (SELECT jsonb_object_agg(url_path, view_count) FROM ranked_pages rp WHERE rp.website_id = am.website_id AND rp.page_rank <= 5) as active_pages
FROM aggregated_metrics am;

CREATE UNIQUE INDEX IF NOT EXISTS idx_realtime_stats_pk ON realtime_website_stats (website_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS bot_stats_by_country AS
SELECT country, COUNT(*) as total_ips, COUNT(*) FILTER (WHERE is_bot) as bot_ips,
       ROUND(100.0 * COUNT(*) FILTER (WHERE is_bot) / NULLIF(COUNT(*), 0), 1) as bot_percentage,
       SUM(total_requests) as total_requests, SUM(total_requests) FILTER (WHERE is_bot) as bot_requests
FROM ip_metadata WHERE country IS NOT NULL GROUP BY country ORDER BY bot_ips DESC;

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_stats_country_pk ON bot_stats_by_country (country);

-- ============================================================
-- PART 5: Views
-- ============================================================

CREATE OR REPLACE VIEW v_today_stats AS
SELECT website_id, COUNT(*) as pageviews, COUNT(DISTINCT session_id) as visitors, COUNT(DISTINCT visit_id) as visits,
       COUNT(*) FILTER (WHERE event_type = 2) as custom_events, AVG(engagement_time) FILTER (WHERE engagement_time IS NOT NULL) as avg_engagement,
       AVG(scroll_depth) FILTER (WHERE scroll_depth IS NOT NULL) as avg_scroll
FROM website_event WHERE created_at >= CURRENT_DATE AND event_type = 1 GROUP BY website_id;

CREATE OR REPLACE VIEW v_current_visitors AS
SELECT website_id, COUNT(DISTINCT session_id) as current_visitors, COUNT(*) as recent_pageviews
FROM website_event WHERE created_at >= NOW() - INTERVAL '5 minutes' AND event_type = 1 GROUP BY website_id;

CREATE OR REPLACE VIEW v_website_summary AS
SELECT w.website_id, w.domain, w.name, w.created_at, COALESCE(cv.current_visitors, 0) as current_visitors,
       COALESCE(ts.pageviews, 0) as today_pageviews, COALESCE(ts.visitors, 0) as today_visitors
FROM website w
LEFT JOIN v_current_visitors cv ON w.website_id = cv.website_id
LEFT JOIN v_today_stats ts ON w.website_id = ts.website_id
WHERE w.deleted_at IS NULL;

CREATE OR REPLACE VIEW v_recent_bots AS
SELECT ip, bot_type, confidence, detection_reason, total_requests, requests_last_minute, last_seen, country
FROM ip_metadata WHERE is_bot = true AND last_seen >= NOW() - INTERVAL '24 hours'
ORDER BY last_seen DESC LIMIT 1000;

-- ============================================================
-- PART 6: Database Functions
-- ============================================================

CREATE OR REPLACE FUNCTION get_dashboard_stats(p_website_id UUID, p_days INTEGER DEFAULT 1, p_country VARCHAR DEFAULT NULL, p_browser VARCHAR DEFAULT NULL, p_device VARCHAR DEFAULT NULL, p_page_path VARCHAR DEFAULT NULL)
RETURNS TABLE (current_visitors BIGINT, today_pageviews BIGINT, today_visitors BIGINT, today_bounce_rate NUMERIC) AS $$
BEGIN
    RETURN QUERY
    WITH filtered_events AS (
        SELECT e.event_id, e.session_id, e.created_at FROM website_event e
        JOIN session s ON e.session_id = s.session_id
        WHERE e.website_id = p_website_id AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
          AND e.event_type = 1 AND (p_country IS NULL OR s.country = p_country) AND (p_browser IS NULL OR s.browser = p_browser)
          AND (p_device IS NULL OR s.device = p_device) AND (p_page_path IS NULL OR e.url_path = p_page_path)
    ),
    visitor_counts AS (SELECT COUNT(DISTINCT session_id) as current_count FROM filtered_events WHERE created_at >= NOW() - INTERVAL '5 minutes'),
    today_stats AS (SELECT COUNT(*) as pageview_count, COUNT(DISTINCT session_id) as visitor_count FROM filtered_events WHERE created_at >= CURRENT_DATE),
    bounce_calc AS (SELECT COUNT(*) as bounced_sessions FROM (SELECT session_id, COUNT(*) as views FROM filtered_events WHERE created_at >= CURRENT_DATE GROUP BY session_id HAVING COUNT(*) = 1) bounces)
    SELECT vc.current_count::BIGINT, ts.pageview_count::BIGINT, ts.visitor_count::BIGINT,
           CASE WHEN ts.visitor_count > 0 THEN ROUND((bc.bounced_sessions::NUMERIC / ts.visitor_count::NUMERIC * 100), 1) ELSE 0 END
    FROM visitor_counts vc CROSS JOIN today_stats ts CROSS JOIN bounce_calc bc;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION get_top_pages(p_website_id UUID, p_days INTEGER DEFAULT 1, p_limit INTEGER DEFAULT 10, p_country VARCHAR DEFAULT NULL, p_browser VARCHAR DEFAULT NULL, p_device VARCHAR DEFAULT NULL)
RETURNS TABLE (path VARCHAR, views BIGINT, unique_visitors BIGINT, avg_engagement_time NUMERIC) AS $$
BEGIN
    RETURN QUERY
    SELECT e.url_path::VARCHAR, COUNT(*)::BIGINT as views, COUNT(DISTINCT e.session_id)::BIGINT as unique_visitors,
           ROUND(AVG(e.engagement_time) FILTER (WHERE e.engagement_time IS NOT NULL), 0) as avg_engagement_time
    FROM website_event e JOIN session s ON e.session_id = s.session_id
    WHERE e.website_id = p_website_id AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
      AND e.event_type = 1 AND e.url_path IS NOT NULL AND (p_country IS NULL OR s.country = p_country)
      AND (p_browser IS NULL OR s.browser = p_browser) AND (p_device IS NULL OR s.device = p_device)
    GROUP BY e.url_path ORDER BY views DESC LIMIT p_limit;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION get_timeseries(p_website_id UUID, p_days INTEGER DEFAULT 7, p_country VARCHAR DEFAULT NULL, p_browser VARCHAR DEFAULT NULL, p_device VARCHAR DEFAULT NULL, p_page_path VARCHAR DEFAULT NULL)
RETURNS TABLE (time_bucket TIMESTAMP WITH TIME ZONE, pageviews BIGINT) AS $$
BEGIN
    RETURN QUERY
    SELECT DATE_TRUNC('hour', e.created_at) as hour, COUNT(*)::BIGINT as views
    FROM website_event e JOIN session s ON e.session_id = s.session_id
    WHERE e.website_id = p_website_id AND e.created_at >= NOW() - (p_days || ' days')::INTERVAL
      AND e.event_type = 1 AND (p_country IS NULL OR s.country = p_country) AND (p_browser IS NULL OR s.browser = p_browser)
      AND (p_device IS NULL OR s.device = p_device) AND (p_page_path IS NULL OR e.url_path = p_page_path)
    GROUP BY hour ORDER BY hour ASC;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION get_breakdown(p_website_id UUID, p_dimension VARCHAR, p_days INTEGER DEFAULT 1, p_limit INTEGER DEFAULT 10, p_country VARCHAR DEFAULT NULL, p_browser VARCHAR DEFAULT NULL, p_device VARCHAR DEFAULT NULL)
RETURNS TABLE (name VARCHAR, count BIGINT) AS $$
BEGIN
    CASE p_dimension
        WHEN 'country' THEN
            RETURN QUERY SELECT COALESCE(s.country, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1 AND (p_browser IS NULL OR s.browser = p_browser) AND (p_device IS NULL OR s.device = p_device)
            GROUP BY s.country ORDER BY count DESC LIMIT p_limit;
        WHEN 'browser' THEN
            RETURN QUERY SELECT COALESCE(s.browser, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1 AND (p_country IS NULL OR s.country = p_country) AND (p_device IS NULL OR s.device = p_device)
            GROUP BY s.browser ORDER BY count DESC LIMIT p_limit;
        WHEN 'device' THEN
            RETURN QUERY SELECT COALESCE(s.device, 'Unknown')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1 AND (p_country IS NULL OR s.country = p_country) AND (p_browser IS NULL OR s.browser = p_browser)
            GROUP BY s.device ORDER BY count DESC LIMIT p_limit;
        WHEN 'referrer' THEN
            RETURN QUERY SELECT COALESCE(e.referrer_domain, 'Direct / None')::VARCHAR as name, COUNT(*)::BIGINT as count
            FROM website_event e JOIN session s ON e.session_id = s.session_id
            WHERE e.website_id = p_website_id AND e.created_at >= CURRENT_DATE - (p_days || ' days')::INTERVAL
              AND e.event_type = 1 AND (p_country IS NULL OR s.country = p_country) AND (p_browser IS NULL OR s.browser = p_browser)
              AND (p_device IS NULL OR s.device = p_device)
            GROUP BY e.referrer_domain ORDER BY count DESC LIMIT p_limit;
        ELSE
            RAISE EXCEPTION 'Invalid dimension: %. Must be country, browser, device, or referrer', p_dimension;
    END CASE;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION is_known_bot_ua(p_user_agent text)
RETURNS TABLE (is_bot boolean, bot_type varchar(50), pattern_name varchar(100), is_legitimate boolean) AS $$
BEGIN
    RETURN QUERY SELECT true, bp.bot_type, bp.pattern_name, bp.is_legitimate
    FROM bot_user_agent_patterns bp WHERE p_user_agent ~* bp.pattern_regex
    ORDER BY CASE WHEN bp.pattern_regex = p_user_agent THEN 1 WHEN bp.pattern_name != 'generic_bot' THEN 2 ELSE 3 END
    LIMIT 1;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION update_ip_metadata(p_ip inet, p_user_agent text, p_country char(2) DEFAULT NULL)
RETURNS boolean AS $$
DECLARE
    v_is_bot boolean := false;
    v_bot_type varchar(50);
    v_pattern_name varchar(100);
    v_is_legitimate boolean;
    v_confidence smallint := 0;
    v_detection_reason text := '';
BEGIN
    SELECT kb.is_bot, kb.bot_type, kb.pattern_name, kb.is_legitimate INTO v_is_bot, v_bot_type, v_pattern_name, v_is_legitimate
    FROM is_known_bot_ua(p_user_agent) kb;

    -- FOUND is true if SELECT INTO returned a row, false if no rows matched
    IF FOUND AND v_is_bot THEN
        v_confidence := CASE WHEN v_pattern_name != 'generic_bot' THEN 90 ELSE 60 END;
        v_detection_reason := 'User agent matches known pattern: ' || v_pattern_name;
    END IF;

    INSERT INTO ip_metadata (ip, first_seen, last_seen, total_requests, requests_last_hour, requests_last_minute,
        is_bot, bot_type, confidence, detection_reason, unique_user_agents, user_agent_sample, country)
    VALUES (p_ip, NOW(), NOW(), 1, 1, 1, v_is_bot, v_bot_type, v_confidence, v_detection_reason, 1, ARRAY[p_user_agent], p_country)
    ON CONFLICT (ip) DO UPDATE SET
        last_seen = NOW(), total_requests = ip_metadata.total_requests + 1,
        requests_last_hour = CASE WHEN ip_metadata.last_seen < NOW() - INTERVAL '1 hour' THEN 1 ELSE ip_metadata.requests_last_hour + 1 END,
        requests_last_minute = CASE WHEN ip_metadata.last_seen < NOW() - INTERVAL '1 minute' THEN 1 ELSE ip_metadata.requests_last_minute + 1 END,
        max_requests_per_minute = GREATEST(ip_metadata.max_requests_per_minute, CASE WHEN ip_metadata.last_seen < NOW() - INTERVAL '1 minute' THEN 1 ELSE ip_metadata.requests_last_minute + 1 END),
        is_bot = CASE WHEN NOT ip_metadata.is_bot AND v_is_bot THEN true ELSE ip_metadata.is_bot END,
        bot_type = COALESCE(v_bot_type, ip_metadata.bot_type),
        confidence = GREATEST(COALESCE(v_confidence, 0), ip_metadata.confidence),
        detection_reason = CASE WHEN v_detection_reason != '' THEN v_detection_reason ELSE ip_metadata.detection_reason END,
        unique_user_agents = CASE WHEN p_user_agent = ANY(ip_metadata.user_agent_sample) THEN ip_metadata.unique_user_agents ELSE ip_metadata.unique_user_agents + 1 END,
        user_agent_sample = CASE WHEN p_user_agent = ANY(ip_metadata.user_agent_sample) THEN ip_metadata.user_agent_sample
            WHEN array_length(ip_metadata.user_agent_sample, 1) < 5 THEN array_append(ip_metadata.user_agent_sample, p_user_agent)
            ELSE ip_metadata.user_agent_sample END,
        country = COALESCE(p_country, ip_metadata.country), updated_at = NOW();

    RETURN v_is_bot;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_bot_status(p_ip inet)
RETURNS TABLE (is_bot boolean, bot_type varchar(50), confidence smallint) AS $$
BEGIN
    RETURN QUERY SELECT COALESCE(im.is_bot, false), im.bot_type, COALESCE(im.confidence, 0)
    FROM ip_metadata im WHERE im.ip = p_ip;
    IF NOT FOUND THEN
        RETURN QUERY SELECT false, NULL::varchar(50), 0::smallint;
    END IF;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION detect_high_frequency_bot()
RETURNS TABLE (ip inet, requests_last_minute integer, confidence smallint) AS $$
BEGIN
    RETURN QUERY UPDATE ip_metadata SET is_bot = true, bot_type = COALESCE(bot_type, 'high_frequency_scraper'),
        confidence = GREATEST(confidence, 80), detection_reason = 'High frequency requests: ' || requests_last_minute || ' req/min'
    WHERE requests_last_minute > 60 AND NOT is_bot
    RETURNING ip_metadata.ip, ip_metadata.requests_last_minute, 80::smallint;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cleanup_old_partitions(retention_days INTEGER DEFAULT 90)
RETURNS TABLE (partition_name TEXT, dropped BOOLEAN) AS $$
DECLARE
    cutoff_date DATE := CURRENT_DATE - retention_days;
    part_name TEXT;
    dropped_count INTEGER := 0;
BEGIN
    FOR part_name IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'website_event_%'
        AND tablename < 'website_event_' || TO_CHAR(cutoff_date, 'YYYY_MM_DD') ORDER BY tablename
    LOOP
        BEGIN
            EXECUTE format('DROP TABLE IF EXISTS %I', part_name);
            partition_name := part_name;
            dropped := TRUE;
            dropped_count := dropped_count + 1;
            RETURN NEXT;
        EXCEPTION WHEN OTHERS THEN
            partition_name := part_name;
            dropped := FALSE;
            RETURN NEXT;
        END;
    END LOOP;
    RAISE NOTICE 'Dropped % old partitions', dropped_count;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_partition_stats()
RETURNS TABLE (partition_name TEXT, partition_size TEXT, row_count BIGINT, start_date DATE, end_date DATE) AS $$
BEGIN
    RETURN QUERY SELECT tablename::TEXT, pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size,
        (SELECT COUNT(*) FROM (SELECT ('public.' || tablename)::regclass) t(tbl) CROSS JOIN LATERAL (SELECT * FROM t.tbl LIMIT 1) x)::BIGINT as rows,
        SUBSTRING(tablename FROM 'website_event_(\d{4}_\d{2}_\d{2})')::DATE as start_dt,
        (SUBSTRING(tablename FROM 'website_event_(\d{4}_\d{2}_\d{2})')::DATE + INTERVAL '1 day')::DATE as end_dt
    FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'website_event_%' ORDER BY tablename DESC;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cleanup_old_bot_logs(retention_days integer DEFAULT 30)
RETURNS integer AS $$
DECLARE
    deleted_count integer := 0;
    cutoff_date date := CURRENT_DATE - retention_days;
    partition_name text;
BEGIN
    FOR partition_name IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'bot_detection_log_%'
        AND tablename < 'bot_detection_log_' || TO_CHAR(cutoff_date, 'YYYY_MM_DD')
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I', partition_name);
        deleted_count := deleted_count + 1;
        RAISE NOTICE 'Dropped old bot log partition: %', partition_name;
    END LOOP;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reset_stale_request_counters()
RETURNS integer AS $$
DECLARE
    updated_count integer;
BEGIN
    UPDATE ip_metadata SET requests_last_hour = 0, requests_last_minute = 0
    WHERE last_seen < NOW() - INTERVAL '1 hour';
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_origin(p_website_id UUID, p_origin TEXT)
RETURNS boolean AS $$
DECLARE
    v_allowed_domains JSONB;
    v_domain TEXT;
BEGIN
    IF p_origin IS NULL OR p_origin = '' OR p_origin = 'null' THEN
        RETURN true;
    END IF;

    SELECT allowed_domains INTO v_allowed_domains FROM website
    WHERE website_id = p_website_id AND deleted_at IS NULL;

    IF NOT FOUND THEN
        RETURN false;
    END IF;

    v_domain := regexp_replace(p_origin, '^https?://', '');
    v_domain := regexp_replace(v_domain, ':\d+$', '');

    RETURN v_allowed_domains ? v_domain;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================
-- PART 7: Triggers
-- ============================================================

CREATE OR REPLACE FUNCTION trg_update_realtime_stats()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO realtime_stats_cache (website_id, current_visitors, pageviews_today, visitors_today, last_updated)
    SELECT website_id, COUNT(DISTINCT session_id) FILTER (WHERE created_at >= NOW() - INTERVAL '5 minutes') as current_visitors,
           COUNT(*) FILTER (WHERE created_at >= CURRENT_DATE) as pageviews_today,
           COUNT(DISTINCT session_id) FILTER (WHERE created_at >= CURRENT_DATE) as visitors_today, NOW()
    FROM new_events GROUP BY website_id
    ON CONFLICT (website_id) DO UPDATE SET
        current_visitors = (SELECT COUNT(DISTINCT session_id) FROM website_event WHERE website_id = EXCLUDED.website_id AND created_at >= NOW() - INTERVAL '5 minutes' AND event_type = 1),
        pageviews_today = (SELECT COUNT(*) FROM website_event WHERE website_id = EXCLUDED.website_id AND created_at >= CURRENT_DATE AND event_type = 1),
        visitors_today = (SELECT COUNT(DISTINCT session_id) FROM website_event WHERE website_id = EXCLUDED.website_id AND created_at >= CURRENT_DATE AND event_type = 1),
        last_updated = NOW();
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_website_event_realtime_stats ON website_event;
CREATE TRIGGER trg_website_event_realtime_stats
    AFTER INSERT ON website_event REFERENCING NEW TABLE AS new_events FOR EACH STATEMENT
    EXECUTE FUNCTION trg_update_realtime_stats();

-- ============================================================
-- PART 8: Comments and Documentation
-- ============================================================

COMMENT ON TABLE website IS 'Tracked websites. PostgreSQL 18+ optimized.';
COMMENT ON TABLE session IS 'User sessions with device/location information.';
COMMENT ON TABLE website_event IS 'Partitioned by created_at (daily partitions). Optimized for PostgreSQL 18.';
COMMENT ON TABLE ip_metadata IS 'Privacy-preserving IP metadata for bot detection.';
COMMENT ON TABLE bot_detection_log IS 'Partitioned log of bot detection events.';
COMMENT ON TABLE realtime_stats_cache IS 'Cache table for real-time statistics.';

COMMENT ON COLUMN website.allowed_domains IS 'JSON array of allowed domains for CORS validation.';
COMMENT ON COLUMN website_event.event_type IS '1 = pageview, 2 = custom event';
COMMENT ON COLUMN website_event.scroll_depth IS 'Scroll percentage (0-100)';
COMMENT ON COLUMN website_event.engagement_time IS 'Time spent on page in milliseconds';
COMMENT ON COLUMN website_event.props IS 'Custom event properties (JSON)';
COMMENT ON COLUMN ip_metadata.confidence IS 'Bot detection confidence: 0-100 scale';

COMMENT ON INDEX idx_event_props_gin IS 'jsonb_path_ops: 78% smaller, 650% faster than default';
COMMENT ON INDEX idx_event_created_brin IS 'BRIN index: very small, perfect for time-series queries';

COMMENT ON MATERIALIZED VIEW daily_website_stats IS 'Refresh hourly: REFRESH MATERIALIZED VIEW CONCURRENTLY daily_website_stats;';
COMMENT ON MATERIALIZED VIEW hourly_website_stats IS 'Refresh every 5 minutes for up-to-date data';
COMMENT ON MATERIALIZED VIEW realtime_website_stats IS 'Refresh every minute for real-time dashboard';
COMMENT ON MATERIALIZED VIEW bot_stats_by_country IS 'Refresh hourly for updated statistics';
