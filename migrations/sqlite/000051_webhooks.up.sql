-- v0.7.38 Build #46.x — Webhooks: tenant-configured HTTP delivery of
-- collaborative_doc / slide_deck / comment lifecycle events.
--
-- Each row is one subscription: a URL + a list of event names + a
-- shared HMAC secret used to sign outbound POST bodies (so the
-- receiver can verify authenticity without a TLS-only trust model).
--
-- `last_delivery_at` powers a simple "are we healthy" diagnostic in
-- the UI; `last_error` carries the latest non-2xx response body (truncated
-- to 1KB) so an operator can see *why* deliveries are failing without
-- grepping service logs.
--
-- The companion table `webhook_deliveries` holds per-event delivery
-- attempts so a 5xx from the receiver becomes a retryable job, not a
-- lost signal.
CREATE TABLE IF NOT EXISTS webhooks (
    id                VARCHAR(36)  PRIMARY KEY,
    tenant_id         INTEGER      NOT NULL,
    name              VARCHAR(128) NOT NULL DEFAULT '',
    url               VARCHAR(512) NOT NULL,
    events            TEXT         NOT NULL DEFAULT '[]',
    secret            VARCHAR(128) NOT NULL DEFAULT '',
    active            INTEGER      NOT NULL DEFAULT 1,
    last_delivery_at  DATETIME,
    last_error        VARCHAR(1024) NOT NULL DEFAULT '',
    created_by        INTEGER      NOT NULL DEFAULT 0,
    created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_webhooks_tenant ON webhooks (tenant_id, active);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              VARCHAR(36)  PRIMARY KEY,
    webhook_id      VARCHAR(36)  NOT NULL,
    tenant_id       INTEGER      NOT NULL,
    event           VARCHAR(64)  NOT NULL,
    payload         TEXT         NOT NULL DEFAULT '{}',
    status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    attempts        INTEGER      NOT NULL DEFAULT 0,
    next_retry_at   DATETIME,
    response_code   INTEGER      NOT NULL DEFAULT 0,
    response_body   VARCHAR(2048) NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at    DATETIME
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_hook ON webhook_deliveries (webhook_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending ON webhook_deliveries (status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_tenant ON webhook_deliveries (tenant_id, created_at DESC);
