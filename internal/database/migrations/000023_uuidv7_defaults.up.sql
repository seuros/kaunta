-- PostgreSQL 18: Switch to UUIDv7 for time-ordered IDs
-- UUIDv7 provides better B-tree index performance and natural time-ordering
-- Existing records keep their UUIDs; only new inserts use UUIDv7
-- Must update ALL tables using uuid_generate_v4() before dropping uuid-ossp

-- Update website table default
ALTER TABLE website ALTER COLUMN website_id SET DEFAULT uuidv7();

-- Update website_event table default
ALTER TABLE website_event ALTER COLUMN event_id SET DEFAULT uuidv7();

-- Update users table default
ALTER TABLE users ALTER COLUMN user_id SET DEFAULT uuidv7();

-- Update user_sessions table default
ALTER TABLE user_sessions ALTER COLUMN session_id SET DEFAULT uuidv7();

-- Update goals table default
ALTER TABLE goals ALTER COLUMN id SET DEFAULT uuidv7();

-- Update goal_completions table default
ALTER TABLE goal_completions ALTER COLUMN id SET DEFAULT uuidv7();

COMMENT ON COLUMN website.website_id IS 'UUIDv7: time-ordered for better index locality (PG 18+)';
COMMENT ON COLUMN website_event.event_id IS 'UUIDv7: time-ordered for better index locality (PG 18+)';
