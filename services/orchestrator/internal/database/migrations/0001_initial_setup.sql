-- =====================================================
-- NOTIFICATION DATABASE - Schema Only
-- =====================================================

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- -----------------------------------------------------
-- NOTIFICATIONS TABLE (Partitioned)
-- -----------------------------------------------------
CREATE TABLE IF NOT EXISTS notifications (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    template_id UUID NOT NULL,
    correlation_id UUID NOT NULL,
    idempotency_key VARCHAR(255),
    channel VARCHAR(20) NOT NULL,
    CONSTRAINT chk_channel CHECK (channel IN ('email', 'push', 'sms', 'in_app')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    CONSTRAINT chk_status CHECK (status IN ('pending', 'enriching', 'queued', 'processing', 'sent', 'failed', 'cancelled')),
    priority VARCHAR(10) NOT NULL DEFAULT 'normal',
    CONSTRAINT chk_priority CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    variables JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    enriched_payload JSONB,
    recipient VARCHAR(255),
    enriched_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    error_code VARCHAR(50),
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    provider VARCHAR(50),
    provider_message_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
) PARTITION BY RANGE (created_at);

ALTER TABLE notifications ADD CONSTRAINT pk_notifications PRIMARY KEY (id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_correlation ON notifications(correlation_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_idempotency ON notifications(idempotency_key, created_at)
    WHERE idempotency_key IS NOT NULL;

-- -----------------------------------------------------
-- NOTIFICATION EVENTS TABLE
-- -----------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL,
    correlation_id UUID NOT NULL,
    event_type VARCHAR(30) NOT NULL,
    CONSTRAINT chk_event_type CHECK (
        event_type IN (
            'created', 'enriched', 'queued', 'sent', 'delivered', 'failed',
            'opened', 'clicked', 'bounced', 'unsubscribed', 'cancelled', 'retried'
        )
    ),
    channel VARCHAR(20),
    event_data JSONB DEFAULT '{}',
    provider VARCHAR(50),
    provider_message_id VARCHAR(255),
    user_agent TEXT,
    ip_address INET,
    event_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_notification ON notification_events(notification_id, event_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_correlation ON notification_events(correlation_id, event_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_type ON notification_events(event_type, event_at DESC);

-- -----------------------------------------------------
-- NOTIFICATION INDEXES
-- -----------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_pending_enrich ON notifications(status, created_at) WHERE status = 'pending' AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_failed_retry ON notifications(status, retry_count, created_at) WHERE status = 'failed' AND retry_count < max_retries AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_channel_status ON notifications(channel, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_correlation_lookup ON notifications(correlation_id) WHERE deleted_at IS NULL;
-- Fixed: Removed NOW() from index predicate
CREATE INDEX IF NOT EXISTS idx_notifications_idempotency_lookup ON notifications(idempotency_key, created_at DESC)
WHERE idempotency_key IS NOT NULL;

-- -----------------------------------------------------
-- TRIGGERS
-- -----------------------------------------------------
DROP TRIGGER IF EXISTS trg_notifications_updated_at ON notifications;
CREATE TRIGGER trg_notifications_updated_at
    BEFORE UPDATE ON notifications
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

-- -----------------------------------------------------
-- PARTITION MANAGEMENT
-- -----------------------------------------------------
CREATE OR REPLACE FUNCTION create_notification_partitions()
RETURNS void AS $$
DECLARE
    start_date DATE;
    end_date DATE;
    partition_name TEXT;
BEGIN
    start_date := date_trunc('month', CURRENT_DATE);
    end_date := start_date + INTERVAL '1 month';
    partition_name := 'notifications_' || to_char(start_date, 'YYYY_MM');

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF notifications FOR VALUES FROM (%L) TO (%L)',
        partition_name, start_date, end_date
    );

    start_date := date_trunc('month', CURRENT_DATE + INTERVAL '1 month');
    end_date := start_date + INTERVAL '1 month';
    partition_name := 'notifications_' || to_char(start_date, 'YYYY_MM');

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF notifications FOR VALUES FROM (%L) TO (%L)',
        partition_name, start_date, end_date
    );
END;
$$ LANGUAGE plpgsql;

SELECT create_notification_partitions();