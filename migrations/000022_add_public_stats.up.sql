-- Add public_stats_enabled flag to website table
-- When enabled, allows unauthenticated access to basic stats (online users, pageviews, visitors)
ALTER TABLE website ADD COLUMN public_stats_enabled BOOLEAN NOT NULL DEFAULT FALSE;
