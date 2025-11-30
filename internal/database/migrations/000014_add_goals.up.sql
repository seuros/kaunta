CREATE TABLE IF NOT EXISTS goals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    website_id UUID NOT NULL REFERENCES website(website_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    target_url TEXT,
    target_event TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(website_id, name)
);

CREATE INDEX IF NOT EXISTS idx_goals_website_id ON goals(website_id);
