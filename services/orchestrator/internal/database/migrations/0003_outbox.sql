-- =====================================================
-- TRANSACTIONAL OUTBOX
-- =====================================================
-- Enrichment previously published to RabbitMQ and then updated the notification
-- row as two independent operations. A crash between them left the two out of
-- step, and a crash before them lost the work entirely: the publish lived only
-- in a detached goroutine with nothing durable behind it.
--
-- The outbox makes "this notification is queued" and "this message must be
-- published" a single atomic fact. A separate publisher drains the table, so a
-- process that dies mid-flight loses nothing — the row is still there.

CREATE TABLE IF NOT EXISTS notification_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL,
    correlation_id UUID NOT NULL,
    routing_key VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,

    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    CONSTRAINT chk_outbox_status CHECK (status IN ('pending', 'published', 'failed')),

    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 10,
    last_error TEXT,

    -- Backoff is expressed by pushing this forward rather than sleeping in the
    -- publisher, so a stuck row never blocks the ones behind it.
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The publisher's only query: oldest due rows first.
CREATE INDEX IF NOT EXISTS idx_outbox_due
    ON notification_outbox (available_at, created_at)
    WHERE status = 'pending';

-- One outbox row per notification, so a retried enrichment cannot enqueue the
-- same message twice.
CREATE UNIQUE INDEX IF NOT EXISTS idx_outbox_notification
    ON notification_outbox (notification_id);

CREATE INDEX IF NOT EXISTS idx_outbox_failed
    ON notification_outbox (created_at DESC)
    WHERE status = 'failed';

DROP TRIGGER IF EXISTS trg_outbox_updated_at ON notification_outbox;
CREATE TRIGGER trg_outbox_updated_at
    BEFORE UPDATE ON notification_outbox
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();
