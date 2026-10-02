CREATE TABLE excluded_address (
    excluded_address_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule TEXT NOT NULL UNIQUE,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE excluded_address IS 'IPs and CIDR blocks whose traffic is never recorded; editable at runtime, unlike the excluded_ips config key';
