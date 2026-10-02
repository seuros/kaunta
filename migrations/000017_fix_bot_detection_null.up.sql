-- Fix bot detection NULL constraint violation
-- Issue: SELECT INTO can set variables to NULL when no rows are returned, even if initialized
-- Solution: Only read from SELECT INTO if FOUND is true

CREATE OR REPLACE FUNCTION update_ip_metadata(p_ip inet, p_user_agent text, p_country char(2) DEFAULT NULL)
RETURNS boolean AS $$
DECLARE
    v_is_bot boolean := false;
    v_bot_type varchar(50) := NULL;
    v_pattern_name varchar(100) := NULL;
    v_is_legitimate boolean := NULL;
    v_confidence smallint := 0;
    v_detection_reason text := '';
    v_tmp_is_bot boolean;
    v_tmp_bot_type varchar(50);
    v_tmp_pattern_name varchar(100);
    v_tmp_is_legitimate boolean;
BEGIN
    -- Use temporary variables for SELECT INTO to avoid NULL contamination
    SELECT kb.is_bot, kb.bot_type, kb.pattern_name, kb.is_legitimate
    INTO v_tmp_is_bot, v_tmp_bot_type, v_tmp_pattern_name, v_tmp_is_legitimate
    FROM is_known_bot_ua(p_user_agent) kb;

    -- Only assign if a row was found (avoids NULL override of initialized values)
    IF FOUND THEN
        v_is_bot := COALESCE(v_tmp_is_bot, false);
        v_bot_type := v_tmp_bot_type;
        v_pattern_name := v_tmp_pattern_name;
        v_is_legitimate := v_tmp_is_legitimate;

        IF v_is_bot THEN
            v_confidence := CASE WHEN v_pattern_name != 'generic_bot' THEN 90 ELSE 60 END;
            v_detection_reason := 'User agent matches known pattern: ' || v_pattern_name;
        END IF;
    END IF;

    INSERT INTO ip_metadata (ip, first_seen, last_seen, total_requests, requests_last_hour, requests_last_minute,
        is_bot, bot_type, confidence, detection_reason, unique_user_agents, user_agent_sample, country)
    VALUES (p_ip, NOW(), NOW(), 1, 1, 1, v_is_bot, v_bot_type, v_confidence, v_detection_reason, 1, ARRAY[p_user_agent], p_country)
    ON CONFLICT (ip) DO UPDATE SET
        last_seen = NOW(), total_requests = ip_metadata.total_requests + 1,
        requests_last_hour = CASE WHEN ip_metadata.last_seen < NOW() - INTERVAL '1 hour' THEN 1 ELSE ip_metadata.requests_last_hour + 1 END,
        requests_last_minute = CASE WHEN ip_metadata.last_seen < NOW() - INTERVAL '1 minute' THEN 1 ELSE ip_metadata.requests_last_minute + 1 END,
        max_requests_per_minute = GREATEST(ip_metadata.max_requests_per_minute, CASE WHEN ip_metadata.last_seen < NOW() - INTERVAL '1 minute' THEN 1 ELSE ip_metadata.requests_last_minute + 1 END),
        is_bot = CASE WHEN NOT COALESCE(ip_metadata.is_bot, false) AND v_is_bot THEN true ELSE COALESCE(ip_metadata.is_bot, false) END,
        bot_type = COALESCE(v_bot_type, ip_metadata.bot_type),
        confidence = GREATEST(COALESCE(v_confidence, 0), COALESCE(ip_metadata.confidence, 0)),
        detection_reason = CASE WHEN v_detection_reason != '' THEN v_detection_reason ELSE ip_metadata.detection_reason END,
        unique_user_agents = CASE WHEN p_user_agent = ANY(ip_metadata.user_agent_sample) THEN ip_metadata.unique_user_agents ELSE ip_metadata.unique_user_agents + 1 END,
        user_agent_sample = CASE WHEN p_user_agent = ANY(ip_metadata.user_agent_sample) THEN ip_metadata.user_agent_sample
            WHEN array_length(ip_metadata.user_agent_sample, 1) < 5 THEN array_append(ip_metadata.user_agent_sample, p_user_agent)
            ELSE ip_metadata.user_agent_sample END,
        country = COALESCE(p_country, ip_metadata.country), updated_at = NOW();

    RETURN v_is_bot;
END;
$$ LANGUAGE plpgsql;
