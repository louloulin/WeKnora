-- Notification center tables (Build #P0.4).
-- See internal/types/notification.go for the model + kind/status sets.

CREATE TABLE IF NOT EXISTS notifications (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    recipient_user_id VARCHAR(64) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    body TEXT,
    payload JSONB,
    status VARCHAR(16) NOT NULL DEFAULT 'unread',
    actor_user_id VARCHAR(64),
    resource_type VARCHAR(64),
    resource_id VARCHAR(128),
    read_at TIMESTAMPTZ,
    dismissed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notif_tenant_recipient_created
    ON notifications (tenant_id, recipient_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notif_tenant_recipient_status
    ON notifications (tenant_id, recipient_user_id, status);
CREATE INDEX IF NOT EXISTS idx_notif_kind
    ON notifications (kind);
CREATE INDEX IF NOT EXISTS idx_notif_actor
    ON notifications (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_notif_resource
    ON notifications (resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_notif_deleted_at
    ON notifications (deleted_at);
