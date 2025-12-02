-- Migration 000018: Add Goal Completion Tracking
-- Implements dual-tracking: event tagging + completion deduplication

-- ============================================================================
-- 1. ADD goal_id TO website_event TABLE
-- ============================================================================

ALTER TABLE website_event ADD COLUMN IF NOT EXISTS goal_id UUID;

-- Foreign key with SET NULL: if goal deleted, events remain but lose attribution
ALTER TABLE website_event
    ADD CONSTRAINT fk_website_event_goal
    FOREIGN KEY (goal_id)
    REFERENCES goals(id)
    ON DELETE SET NULL;

-- ============================================================================
-- 2. CREATE PARTIAL INDEXES ON website_event.goal_id
-- ============================================================================

-- Partial index: only index rows where goal_id is set (saves space/performance)
CREATE INDEX IF NOT EXISTS idx_website_event_goal_id
    ON website_event(website_id, goal_id, created_at DESC)
    WHERE goal_id IS NOT NULL;

-- Index for looking up all events for a specific goal
CREATE INDEX IF NOT EXISTS idx_website_event_goal_lookup
    ON website_event(goal_id, created_at DESC)
    WHERE goal_id IS NOT NULL;

-- ============================================================================
-- 3. CREATE goal_completions TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS goal_completions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    goal_id UUID NOT NULL,
    session_id UUID NOT NULL,
    event_id UUID,  -- Nullable: event may be in old partition or deleted
    website_id UUID NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Foreign keys with appropriate cascade behavior
    CONSTRAINT fk_goal_completions_goal
        FOREIGN KEY (goal_id)
        REFERENCES goals(id)
        ON DELETE CASCADE,  -- Goal deleted → completions deleted

    CONSTRAINT fk_goal_completions_session
        FOREIGN KEY (session_id)
        REFERENCES session(session_id)
        ON DELETE CASCADE,  -- Session deleted → completions deleted

    CONSTRAINT fk_goal_completions_website
        FOREIGN KEY (website_id)
        REFERENCES website(website_id)
        ON DELETE CASCADE,  -- Website deleted → completions deleted

    -- UNIQUE constraint: one completion per goal per session (deduplication)
    CONSTRAINT uq_goal_completion_per_session
        UNIQUE (goal_id, session_id)
);

-- ============================================================================
-- 4. CREATE INDEXES FOR goal_completions
-- ============================================================================

-- Primary query pattern: get completions for a goal
CREATE INDEX IF NOT EXISTS idx_goal_completions_goal_time
    ON goal_completions(goal_id, completed_at DESC);

-- Query pattern: get completions for a website (via goal)
CREATE INDEX IF NOT EXISTS idx_goal_completions_website_time
    ON goal_completions(website_id, completed_at DESC);

-- Query pattern: lookup by session (for deduplication check)
CREATE INDEX IF NOT EXISTS idx_goal_completions_session
    ON goal_completions(session_id);

-- ============================================================================
-- 5. ADD COMMENTS
-- ============================================================================

COMMENT ON COLUMN website_event.goal_id IS 'Goal ID if this event triggered a goal completion';

COMMENT ON TABLE goal_completions IS 'Tracks goal completions with session-level deduplication. One completion per goal per session.';
COMMENT ON COLUMN goal_completions.goal_id IS 'The goal that was completed';
COMMENT ON COLUMN goal_completions.session_id IS 'The session in which the goal was completed';
COMMENT ON COLUMN goal_completions.event_id IS 'The event that triggered the completion (nullable, may reference old partition)';
COMMENT ON COLUMN goal_completions.completed_at IS 'Timestamp when goal was first completed in this session';

COMMENT ON CONSTRAINT uq_goal_completion_per_session ON goal_completions IS 'Enforces deduplication: one goal completion per session';

-- ============================================================================
-- MIGRATION COMPLETE
-- ============================================================================
