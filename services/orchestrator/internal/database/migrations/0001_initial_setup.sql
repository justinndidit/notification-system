-- =====================================================
-- DATABASE 3: notification_db
-- Responsibility: Tracking notification requests, deliveries, and events.
-- =====================================================

-- -----------------------------------------------------
-- PARTITIONED TABLES (For high volume data: Notifications & Deliveries)
-- -----------------------------------------------------

-- Note: The ORCHESTRATOR/DBA must handle partition creation (e.g., monthly).
-- Example Partition Creation (done by separate script/migration):
-- CREATE TABLE notifications_2025_11 PARTITION OF notifications FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE notifications (
    id UUID NOT NULL DEFAULT gen_random_uuid(), -- Removed PRIMARY KEY from parent table due to partitioning

    -- References (No cross-DB FKs)
    user_id UUID NOT NULL,
    template_id UUID NOT NULL,

    -- Service Context
    user_service_id VARCHAR(50) DEFAULT 'user-service',
    template_service_id VARCHAR(50) DEFAULT 'template-service',

    -- Correlation ID (Crucial for external API Gateway tracing)
    correlation_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    CONSTRAINT chk_status CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'cancelled', 'scheduled')),

    -- Priority
    priority VARCHAR(10) NOT NULL DEFAULT 'normal',
    CONSTRAINT chk_priority CHECK (priority IN ('low', 'normal', 'high', 'urgent')),

    -- Request details
    channel JSONB NOT NULL, -- Channels requested (e.g., ['email', 'push'])
    payload JSONB DEFAULT '{}',          -- Dynamic template data provided by caller
    request_id VARCHAR(100),             -- ID from the initial caller (optional)

    -- Scheduling - Extended version
    --scheduled_for TIMESTAMPTZ,           -- When the notification should be sent
    sent_at TIMESTAMPTZ,

    -- Metadata
    metadata JSONB DEFAULT '{}',
    tags VARCHAR(50)[],

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,
    deleted_at TIMESTAMPTZ
) PARTITION BY RANGE (created_at);

-- Add Primary Key back to partitioned tables via the combination of columns
ALTER TABLE notifications ADD CONSTRAINT pk_notifications PRIMARY KEY (id, created_at);

-- -----------------------------------------------------
-- NOTIFICATION_DELIVERIES (Per-Channel Tracking)
-- -----------------------------------------------------
CREATE TABLE notification_deliveries (
    id UUID NOT NULL DEFAULT gen_random_uuid(), -- Removed PRIMARY KEY
    notification_id UUID NOT NULL, -- No FK constraint here, partition key is 'created_at'

    -- Channel
    channel VARCHAR(20) NOT NULL,
    CONSTRAINT chk_channel CHECK (channel IN ('email', 'push', 'sms', 'in_app')),

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'queued',
    CONSTRAINT chk_delivery_status CHECK (
        status IN ('queued', 'sending', 'sent', 'delivered', 'failed', 'bounced', 'rejected')
    ),

    -- Recipient
    recipient VARCHAR(255) NOT NULL,

    -- Provider details
    provider VARCHAR(50),
    provider_message_id VARCHAR(255),
    provider_response JSONB,

    -- Error handling/Retry mechanism
    error_code VARCHAR(50),
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ,

    -- Engagement
    opened_at TIMESTAMPTZ,
    clicked_at TIMESTAMPTZ,

    -- Content snapshot
    rendered_content JSONB,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);

ALTER TABLE notification_deliveries ADD CONSTRAINT pk_notification_deliveries PRIMARY KEY (id, created_at);

-- -----------------------------------------------------
-- IDEMPOTENCY_KEYS (API Gateway Protection)
-- -----------------------------------------------------
CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    request_key VARCHAR(255) UNIQUE NOT NULL, -- The unique X-Request-ID header value

    response_data JSONB NOT NULL,             -- The response sent back to the client on the original successful request
    notification_id UUID REFERENCES notifications(id), -- Correlation to the resulting notification

    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours', -- TTL for cleanup

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------
-- RATE_LIMITS (Daily Limit Tracking)
-- -----------------------------------------------------
CREATE TABLE rate_limits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL,
    date DATE NOT NULL DEFAULT CURRENT_DATE,

    -- Counts per channel
    email_count INTEGER NOT NULL DEFAULT 0,
    push_count INTEGER NOT NULL DEFAULT 0,
    sms_count INTEGER NOT NULL DEFAULT 0,

    -- Limits (can be denormalized here or fetched from User Service)
    email_limit INTEGER NOT NULL DEFAULT 50,
    push_limit INTEGER NOT NULL DEFAULT 100,
    sms_limit INTEGER NOT NULL DEFAULT 10,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(user_id, date)
);

-- -----------------------------------------------------
-- WEBHOOK_EVENTS (Raw Provider Callbacks)
-- -----------------------------------------------------
CREATE TABLE webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    provider VARCHAR(50) NOT NULL,
    event_type VARCHAR(50) NOT NULL,

    payload JSONB NOT NULL,                   -- Raw JSON payload from the provider (e.g., SendGrid/FCM)

    processed BOOLEAN NOT NULL DEFAULT false, -- Flag for background worker
    processed_at TIMESTAMPTZ,

    provider_message_id VARCHAR(255),         -- Used to correlate back to notification_deliveries
    notification_id UUID,                     -- Resolved after processing

    ip_address INET,
    user_agent TEXT,

    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------
-- SCHEDULED_NOTIFICATIONS (Future Sends Queue)
-- -----------------------------------------------------
CREATE TABLE scheduled_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL REFERENCES notifications(id), -- Note: FK is to the partition parent table

    scheduled_for TIMESTAMPTZ NOT NULL,

    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    CONSTRAINT chk_scheduled_status CHECK (status IN ('pending', 'sent', 'cancelled')),

    action_data JSONB NOT NULL, -- Data needed for the orchestrator to execute (e.g., context)

    processed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------
-- DIGEST_QUEUE (Summary Queue)
-- -----------------------------------------------------
CREATE TABLE digest_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL,
    notification_id UUID NOT NULL REFERENCES notifications(id),

    digest_type VARCHAR(20) NOT NULL,
    CONSTRAINT chk_digest_type CHECK (digest_type IN ('hourly', 'daily', 'weekly')),

    digest_date DATE NOT NULL, -- The date/time bucket this notification belongs to

    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- 'pending' until the digest is compiled

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(user_id, notification_id) -- Prevent the same notification from entering the queue twice
);

-- -----------------------------------------------------
-- PROVIDER_STATUS (Health Monitoring)
-- -----------------------------------------------------
CREATE TABLE provider_status (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    provider_name VARCHAR(50) NOT NULL,
    channel VARCHAR(20) NOT NULL,

    is_healthy BOOLEAN NOT NULL DEFAULT true,
    last_success_at TIMESTAMPTZ,
    last_failure_at TIMESTAMPTZ,

    -- Stats (rolling window for burst management)
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,

    rate_limit_remaining INTEGER,
    rate_limit_reset_at TIMESTAMPTZ,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(provider_name, channel)
);

-- -----------------------------------------------------
-- NOTIFICATION_EVENTS (Activity Timeline)
-- -----------------------------------------------------
CREATE TABLE notification_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    notification_id UUID NOT NULL REFERENCES notifications(id),
    delivery_id UUID REFERENCES notification_deliveries(id),

    event_type VARCHAR(30) NOT NULL,
    CONSTRAINT chk_event_type CHECK (
        event_type IN (
            'created', 'queued', 'sent', 'delivered', 'failed',
            'opened', 'clicked', 'bounced', 'complained', 'unsubscribed',
            'cancelled', 'retried'
        )
    ),

    channel VARCHAR(20),
    event_data JSONB DEFAULT '{}',

    user_agent TEXT,
    ip_address INET,

    event_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- -----------------------------------------------------
-- NOTIFICATION_SUPPRESSION (Block List)
-- -----------------------------------------------------
CREATE TABLE notification_suppression (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    identifier VARCHAR(255) NOT NULL,
    identifier_type VARCHAR(20) NOT NULL,
    CONSTRAINT chk_identifier_type CHECK (identifier_type IN ('email', 'phone', 'device_token')),

    channel VARCHAR(20),
    category VARCHAR(50),

    reason VARCHAR(50) NOT NULL,
    CONSTRAINT chk_reason CHECK (
        reason IN ('user_request', 'hard_bounce', 'soft_bounce', 'complaint', 'invalid', 'expired', 'admin')
    ),

    notes TEXT,
    source VARCHAR(50),

    expires_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,

    UNIQUE(identifier, identifier_type, channel, category)
);

-- Indexes for notification_db
-- 1. Orchestrator Deduplication Index (Suggested Improvement)
CREATE INDEX idx_notifications_dedup ON notifications(user_id, template_id, created_at DESC)
    WHERE status NOT IN ('failed', 'cancelled')
      AND created_at > NOW() - INTERVAL '5 minutes';

-- 2. Retry Polling Index (Optimized)
CREATE INDEX idx_deliveries_retry_poll ON notification_deliveries(next_retry_at, status)
    WHERE status = 'failed' AND next_retry_at <= NOW();

-- 3. Scheduled Job Polling Index (Suggested Improvement)
CREATE INDEX idx_scheduled_poll ON scheduled_notifications(scheduled_for, status)
    WHERE status = 'pending' AND scheduled_for <= NOW();

-- 4. General Notifications and Deliveries
CREATE INDEX idx_notifications_user ON notifications(user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_notifications_correlation ON notifications(correlation_id);
CREATE INDEX idx_deliveries_notification ON notification_deliveries(notification_id);
CREATE INDEX idx_deliveries_channel_status ON notification_deliveries(channel, status);

-- 5. Idempotency Keys
CREATE INDEX idx_idempotency_key ON idempotency_keys(request_key)
    WHERE expires_at > NOW();

-- 6. Webhooks Polling
CREATE INDEX idx_webhook_processing ON webhook_events(processed, received_at)
    WHERE processed = false;
CREATE INDEX idx_webhook_provider_msg ON webhook_events(provider_message_id);

-- Triggers for notification_db
CREATE TRIGGER trg_notifications_updated_at BEFORE UPDATE ON notifications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_deliveries_updated_at BEFORE UPDATE ON notification_deliveries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_rate_limits_updated_at BEFORE UPDATE ON rate_limits
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_scheduled_notifications_updated_at BEFORE UPDATE ON scheduled_notifications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_provider_status_updated_at BEFORE UPDATE ON provider_status
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();