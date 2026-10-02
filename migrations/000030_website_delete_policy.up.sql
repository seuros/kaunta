ALTER TABLE website ADD COLUMN pending_delete_at TIMESTAMPTZ;

CREATE FUNCTION website_delete_policy() RETURNS trigger AS $$
DECLARE
    v_has_data BOOLEAN;
BEGIN
    IF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL THEN
        SELECT EXISTS (
            SELECT 1 FROM website_event e WHERE e.website_id = OLD.website_id
        ) INTO v_has_data;
        IF v_has_data THEN
            IF OLD.pending_delete_at IS NULL THEN
                NEW.deleted_at := NULL;
                NEW.pending_delete_at := NOW();
            ELSIF OLD.pending_delete_at > NOW() - INTERVAL '30 days' THEN
                NEW.deleted_at := NULL;
                NEW.pending_delete_at := OLD.pending_delete_at;
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER website_delete_policy
BEFORE UPDATE ON website
FOR EACH ROW EXECUTE FUNCTION website_delete_policy();

CREATE FUNCTION website_block_hard_delete() RETURNS trigger AS $$
BEGIN
    IF OLD.deleted_at IS NULL THEN
        RAISE EXCEPTION 'website % is not soft-deleted; hard DELETE is blocked by policy',
            OLD.domain;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER website_block_hard_delete
BEFORE DELETE ON website
FOR EACH ROW EXECUTE FUNCTION website_block_hard_delete();
