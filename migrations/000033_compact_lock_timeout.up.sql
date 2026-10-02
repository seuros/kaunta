CREATE OR REPLACE FUNCTION kaunta_compact_next_partition_month(
    p_parent regclass,
    p_older_than_days integer DEFAULT 90
)
RETURNS text AS $$
DECLARE
    v_parent text := p_parent::text;
    v_cutoff date := current_date - p_older_than_days;
    v_key text := quote_ident(kaunta_partition_key(p_parent));
    v_month date;
    v_month_end date;
    v_monthly text;
    v_columns text;
    v_child text;
    v_is_partition boolean;
BEGIN
    SET LOCAL lock_timeout = '5s';
    SET LOCAL statement_timeout = '120s';

    SELECT min(month) INTO v_month
    FROM (
        SELECT date_trunc(
                   'month',
                   to_date(substring(c.relname from '([0-9]{4}_[0-9]{2}_[0-9]{2})$'), 'YYYY_MM_DD')
               )::date AS month
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        WHERE i.inhparent = p_parent
          AND c.relkind = 'r'
          AND c.relname ~ ('^' || v_parent || '_[0-9]{4}_[0-9]{2}_[0-9]{2}$')
    ) days
    WHERE (month + interval '1 month')::date <= v_cutoff;

    IF v_month IS NULL THEN
        RETURN NULL;
    END IF;

    v_month_end := (v_month + interval '1 month')::date;
    v_monthly := v_parent || '_' || to_char(v_month, 'YYYY_MM');

    SELECT relpartbound IS NOT NULL INTO v_is_partition
    FROM pg_class WHERE relname = v_monthly AND relnamespace = 'public'::regnamespace;
    IF v_is_partition THEN
        RAISE EXCEPTION 'partition % already exists', v_monthly;
    END IF;
    EXECUTE format('DROP TABLE IF EXISTS %I', v_monthly);

    SELECT string_agg(quote_ident(attname), ', ' ORDER BY attnum) INTO v_columns
    FROM pg_attribute
    WHERE attrelid = p_parent AND attnum > 0 AND NOT attisdropped AND attgenerated = '';

    EXECUTE format('CREATE TABLE %I (LIKE %s INCLUDING ALL)', v_monthly, v_parent);
    EXECUTE format(
        'INSERT INTO %I (%s) SELECT %s FROM %s WHERE %s >= %L AND %s < %L',
        v_monthly, v_columns, v_columns, v_parent,
        v_key, v_month, v_key, v_month_end
    );

    FOR v_child IN
        SELECT c.relname
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        WHERE i.inhparent = p_parent
          AND c.relkind = 'r'
          AND c.relname ~ ('^' || v_parent || '_' || to_char(v_month, 'YYYY_MM') || '_[0-9]{2}$')
    LOOP
        EXECUTE format('ALTER TABLE %s DETACH PARTITION %I', v_parent, v_child);
        EXECUTE format('DROP TABLE %I', v_child);
    END LOOP;

    EXECUTE format(
        'ALTER TABLE %s ATTACH PARTITION %I FOR VALUES FROM (%L) TO (%L)',
        v_parent, v_monthly, v_month, v_month_end
    );

    RETURN v_monthly;
EXCEPTION
    WHEN lock_not_available OR query_canceled THEN
        RAISE WARNING 'compaction of % deferred: %', v_monthly, SQLERRM;
        RETURN NULL;
END;
$$ LANGUAGE plpgsql;
