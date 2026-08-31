CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL,
    recipient_user_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    payload TEXT,
    status TEXT NOT NULL DEFAULT 'unread',
    actor_user_id TEXT,
    resource_type TEXT,
    resource_id TEXT,
    read_at DATETIME,
    dismissed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_notif_tenant_recipient_created
    ON notifications (tenant_id, recipient_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notif_tenant_recipient_status
    ON notifications (tenant_id, recipient_user_id, status);
CREATE INDEX IF NOT EXISTS idx_notif_kind ON notifications (kind);
CREATE INDEX IF NOT EXISTS idx_notif_actor ON notifications (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_notif_resource ON notifications (resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_notif_deleted_at ON notifications (deleted_at);
