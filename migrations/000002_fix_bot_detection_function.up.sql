-- Fix update_ip_metadata function to handle normal (non-bot) User-Agents
-- Issue: The function was crashing with "index out of range" when is_known_bot_ua() returned 0 rows
-- Root cause: INTO statement tried to unpack values when no rows matched
-- Solution: Use FOUND flag to handle both cases (bot match and no match)

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

    -- FOUND is true if SELECT INTO returned a row, false if no rows matched (normal browsers)
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
