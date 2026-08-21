-- =====================================================
-- PARTITION MAINTENANCE
-- =====================================================
-- The original function created exactly two partitions and ran once, at
-- migration time. That is an outage with a date on it: once the window lapses,
-- every insert fails with "no partition of relation found for row".
--
-- This replaces it with a parameterised version that is safe to run repeatedly,
-- called on every boot and on a schedule by the orchestrator.

CREATE OR REPLACE FUNCTION create_notification_partitions(months_ahead INTEGER DEFAULT 3)
RETURNS TABLE(partition_name TEXT, created BOOLEAN) AS $$
DECLARE
    start_date DATE;
    end_date DATE;
    name TEXT;
    existed BOOLEAN;
    i INTEGER;
BEGIN
    -- Start one month back so a deployment that has been down across a month
    -- boundary can still write late-arriving rows.
    FOR i IN -1..months_ahead LOOP
        start_date := date_trunc('month', CURRENT_DATE + (i || ' month')::INTERVAL);
        end_date := start_date + INTERVAL '1 month';
        name := 'notifications_' || to_char(start_date, 'YYYY_MM');

        SELECT EXISTS (
            SELECT 1 FROM pg_class WHERE relname = name
        ) INTO existed;

        IF NOT existed THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF notifications FOR VALUES FROM (%L) TO (%L)',
                name, start_date, end_date
            );
        END IF;

        partition_name := name;
        created := NOT existed;
        RETURN NEXT;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Ensure the window is open now, not only from the next scheduled run.
SELECT * FROM create_notification_partitions(3);
