-- v0.7.38 Build #46.x — Webhooks (MySQL mirror of sqlite 000051).
CREATE TABLE IF NOT EXISTS webhooks (
    id                VARCHAR(36)  NOT NULL PRIMARY KEY,
    tenant_id         BIGINT       NOT NULL,
    name              VARCHAR(128) NOT NULL DEFAULT '',
    url               VARCHAR(512) NOT NULL,
    events            TEXT         NOT NULL,
    secret            VARCHAR(128) NOT NULL DEFAULT '',
    active            TINYINT      NOT NULL DEFAULT 1,
    last_delivery_at  DATETIME     NULL,
    last_error        VARCHAR(1024) NOT NULL DEFAULT '',
    created_by        BIGINT       NOT NULL DEFAULT 0,
    created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_webhooks_tenant (tenant_id, active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    webhook_id      VARCHAR(36)  NOT NULL,
    tenant_id       BIGINT       NOT NULL,
    event           VARCHAR(64)  NOT NULL,
    payload         TEXT         NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    attempts        INT          NOT NULL DEFAULT 0,
    next_retry_at   DATETIME     NULL,
    response_code   INT          NOT NULL DEFAULT 0,
    response_body   VARCHAR(2048) NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    delivered_at    DATETIME     NULL,
    INDEX idx_webhook_deliveries_hook (webhook_id, created_at),
    INDEX idx_webhook_deliveries_pending (status, next_retry_at),
    INDEX idx_webhook_deliveries_tenant (tenant_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
