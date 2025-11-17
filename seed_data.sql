-- Seed 30 days of realistic analytics data
-- Usage: psql $DATABASE_URL -f seed_data.sql

DO $$
DECLARE
    -- Configuration: Change this to your website_id
    v_website_id UUID := '9028aafb-4eeb-4078-b066-7a051c662b07';
    v_session_id UUID;
    v_visit_id UUID;
    v_day_offset INT;
    v_hour INT;
    v_sessions_for_day INT;
    v_events_per_session INT;
    v_session_num INT;
    v_event_num INT;
    v_timestamp TIMESTAMPTZ;
    v_traffic_multiplier FLOAT;
    v_is_weekend BOOLEAN;
    v_is_spike_day BOOLEAN;

    -- Realistic data arrays
    v_browsers TEXT[] := ARRAY['Chrome', 'Safari', 'Firefox', 'Edge'];
    v_browser_weights FLOAT[] := ARRAY[0.60, 0.20, 0.12, 0.08];
    v_oses TEXT[] := ARRAY['Windows', 'macOS', 'iOS', 'Android', 'Linux'];
    v_os_weights FLOAT[] := ARRAY[0.45, 0.25, 0.15, 0.10, 0.05];
    v_devices TEXT[] := ARRAY['desktop', 'mobile', 'mobile', 'desktop', 'desktop', 'mobile', 'desktop'];
    v_countries TEXT[] := ARRAY['US', 'GB', 'CA', 'DE', 'FR', 'AU', 'NL', 'ES', 'IT', 'JP'];
    v_cities TEXT[][] := ARRAY[
        ARRAY['New York', 'Los Angeles', 'Chicago', 'Houston', 'Phoenix'],
        ARRAY['London', 'Manchester', 'Birmingham', 'Leeds', 'Glasgow'],
        ARRAY['Toronto', 'Montreal', 'Vancouver', 'Calgary', 'Ottawa'],
        ARRAY['Berlin', 'Munich', 'Hamburg', 'Frankfurt', 'Cologne'],
        ARRAY['Paris', 'Lyon', 'Marseille', 'Toulouse', 'Nice']
    ];
    v_languages TEXT[] := ARRAY['en-US', 'en-GB', 'fr-FR', 'de-DE', 'es-ES', 'ja-JP', 'en-CA'];
    v_screens TEXT[] := ARRAY['1920x1080', '1366x768', '2560x1440', '390x844', '414x896', '1536x864'];

    v_urls TEXT[] := ARRAY[
        '/', '/about', '/products', '/pricing', '/contact',
        '/blog', '/blog/getting-started', '/blog/best-practices', '/blog/tutorials',
        '/docs', '/docs/installation', '/docs/api', '/docs/guides',
        '/features', '/customers', '/team', '/careers'
    ];
    v_page_titles TEXT[] := ARRAY[
        'Home - Kaunta Analytics', 'About Us', 'Products', 'Pricing Plans', 'Contact',
        'Blog', 'Getting Started Guide', 'Best Practices', 'Tutorials',
        'Documentation', 'Installation Guide', 'API Reference', 'User Guides',
        'Features', 'Customers', 'Our Team', 'Careers'
    ];
    v_referrers TEXT[] := ARRAY[
        NULL, NULL, NULL,  -- 30% direct traffic
        'google.com', 'google.com', 'google.com',  -- 25% Google
        'twitter.com', 'twitter.com',  -- 15% Twitter
        'github.com', 'github.com',  -- 10% GitHub
        'facebook.com', 'linkedin.com', 'reddit.com', 'news.ycombinator.com'  -- 20% others
    ];

    -- Spike days (random selection from 30 days)
    v_spike_days INT[] := ARRAY[5, 12, 23];

    v_country_idx INT;
    v_browser TEXT;
    v_os TEXT;
    v_device TEXT;
    v_country TEXT;
    v_city TEXT;
    v_language TEXT;
    v_screen TEXT;
    v_url TEXT;
    v_title TEXT;
    v_referrer TEXT;
    v_referrer_domain TEXT;

    v_total_sessions INT := 0;
    v_total_events INT := 0;
BEGIN
    RAISE NOTICE 'Starting data generation for website_id: %', v_website_id;

    -- Loop through 30 days (going backwards from today)
    FOR v_day_offset IN 0..29 LOOP
        -- Check if weekend
        v_is_weekend := EXTRACT(DOW FROM (CURRENT_DATE - v_day_offset)) IN (0, 6);

        -- Check if spike day
        v_is_spike_day := v_day_offset = ANY(v_spike_days);

        -- Base sessions per day: 30-50
        v_sessions_for_day := 30 + (random() * 20)::INT;

        -- Adjust for weekend (-40%)
        IF v_is_weekend THEN
            v_sessions_for_day := (v_sessions_for_day * 0.6)::INT;
        END IF;

        -- Apply spike multiplier (3-5x)
        IF v_is_spike_day THEN
            v_traffic_multiplier := 3.0 + (random() * 2.0);
            v_sessions_for_day := (v_sessions_for_day * v_traffic_multiplier)::INT;
            RAISE NOTICE 'Spike day detected: % (%.1fx traffic)', CURRENT_DATE - v_day_offset, v_traffic_multiplier;
        END IF;

        -- Generate sessions for this day
        FOR v_session_num IN 1..v_sessions_for_day LOOP
            -- Weighted hour distribution (business hours peak)
            -- 0-6: 2%, 7-9: 15%, 10-17: 60%, 18-23: 23%
            CASE
                WHEN random() < 0.02 THEN v_hour := (random() * 6)::INT;  -- Night
                WHEN random() < 0.17 THEN v_hour := 7 + (random() * 2)::INT;  -- Morning
                WHEN random() < 0.77 THEN v_hour := 10 + (random() * 7)::INT;  -- Peak hours
                ELSE v_hour := 18 + (random() * 5)::INT;  -- Evening
            END CASE;

            -- Generate timestamp
            v_timestamp := (CURRENT_DATE - v_day_offset) + (v_hour || ' hours')::INTERVAL + (random() * 3600 || ' seconds')::INTERVAL;

            -- Generate session ID (deterministic based on timestamp)
            v_session_id := md5(v_website_id::TEXT || v_timestamp::TEXT || v_session_num::TEXT)::UUID;

            -- Pick random browser (weighted)
            CASE
                WHEN random() < v_browser_weights[1] THEN v_browser := v_browsers[1];
                WHEN random() < v_browser_weights[1] + v_browser_weights[2] THEN v_browser := v_browsers[2];
                WHEN random() < v_browser_weights[1] + v_browser_weights[2] + v_browser_weights[3] THEN v_browser := v_browsers[3];
                ELSE v_browser := v_browsers[4];
            END CASE;

            -- Pick random OS (weighted)
            CASE
                WHEN random() < v_os_weights[1] THEN v_os := v_oses[1];
                WHEN random() < v_os_weights[1] + v_os_weights[2] THEN v_os := v_oses[2];
                WHEN random() < v_os_weights[1] + v_os_weights[2] + v_os_weights[3] THEN v_os := v_oses[3];
                WHEN random() < v_os_weights[1] + v_os_weights[2] + v_os_weights[3] + v_os_weights[4] THEN v_os := v_oses[4];
                ELSE v_os := v_oses[5];
            END CASE;

            -- Pick device, country, language, screen
            v_device := v_devices[1 + (random() * (array_length(v_devices, 1) - 1))::INT];
            v_country_idx := 1 + (random() * (array_length(v_countries, 1) - 1))::INT;
            v_country := v_countries[v_country_idx];
            v_city := v_cities[LEAST(v_country_idx, 5)][1 + (random() * 4)::INT];
            v_language := v_languages[1 + (random() * (array_length(v_languages, 1) - 1))::INT];
            v_screen := v_screens[1 + (random() * (array_length(v_screens, 1) - 1))::INT];

            -- Insert session
            INSERT INTO session (
                session_id, website_id, browser, os, device, screen, language,
                country, city, created_at
            ) VALUES (
                v_session_id, v_website_id, v_browser, v_os, v_device, v_screen, v_language,
                v_country, v_city, v_timestamp
            ) ON CONFLICT (session_id) DO NOTHING;

            v_total_sessions := v_total_sessions + 1;

            -- Generate 3-8 pageviews per session
            v_events_per_session := 3 + (random() * 5)::INT;

            FOR v_event_num IN 1..v_events_per_session LOOP
                -- Generate visit ID (changes every 1-3 events to simulate multiple visits)
                IF v_event_num = 1 OR (random() < 0.3 AND v_event_num > 1) THEN
                    v_visit_id := md5(v_session_id::TEXT || v_event_num::TEXT)::UUID;
                END IF;

                -- Pick random URL and title
                v_url := v_urls[1 + (random() * (array_length(v_urls, 1) - 1))::INT];
                v_title := v_page_titles[1 + (random() * (array_length(v_page_titles, 1) - 1))::INT];

                -- First pageview in session: pick referrer
                IF v_event_num = 1 THEN
                    v_referrer := v_referrers[1 + (random() * (array_length(v_referrers, 1) - 1))::INT];
                    IF v_referrer IS NOT NULL THEN
                        v_referrer_domain := v_referrer;
                    ELSE
                        v_referrer_domain := NULL;
                    END IF;
                ELSE
                    -- Internal navigation
                    v_referrer := NULL;
                    v_referrer_domain := NULL;
                END IF;

                -- Advance timestamp by 15s-2min per pageview
                v_timestamp := v_timestamp + ((15 + random() * 105) || ' seconds')::INTERVAL;

                -- Insert pageview event
                INSERT INTO website_event (
                    event_id, website_id, session_id, visit_id, created_at,
                    url_path, page_title, hostname, referrer_domain,
                    event_type, scroll_depth, engagement_time
                ) VALUES (
                    gen_random_uuid(), v_website_id, v_session_id, v_visit_id, v_timestamp,
                    v_url, v_title, 'kaunta.io', v_referrer_domain,
                    1,  -- pageview
                    (50 + random() * 50)::INT,  -- scroll depth 50-100%
                    ((15 + random() * 180) * 1000)::INT  -- engagement time 15s-3min
                );

                v_total_events := v_total_events + 1;

                -- 10% chance to add custom event (button click, etc.)
                IF random() < 0.10 THEN
                    INSERT INTO website_event (
                        event_id, website_id, session_id, visit_id, created_at,
                        url_path, event_type, event_name, props
                    ) VALUES (
                        gen_random_uuid(), v_website_id, v_session_id, v_visit_id, v_timestamp,
                        v_url, 2,  -- custom event
                        (ARRAY['button_click', 'form_submit', 'video_play', 'download'])[1 + (random() * 3)::INT],
                        jsonb_build_object('element', 'cta-button', 'position', 'hero')
                    );

                    v_total_events := v_total_events + 1;
                END IF;
            END LOOP;
        END LOOP;

        -- Progress indicator
        IF v_day_offset % 5 = 0 THEN
            RAISE NOTICE 'Progress: % days completed (% sessions, % events)', v_day_offset + 1, v_total_sessions, v_total_events;
        END IF;
    END LOOP;

    RAISE NOTICE '✓ Data generation complete!';
    RAISE NOTICE '  Sessions: %', v_total_sessions;
    RAISE NOTICE '  Events: %', v_total_events;
    RAISE NOTICE '  Date range: % to %', CURRENT_DATE - 29, CURRENT_DATE;
END $$;
