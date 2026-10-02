ALTER TABLE website ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(user_id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_website_user_id ON website(user_id);
