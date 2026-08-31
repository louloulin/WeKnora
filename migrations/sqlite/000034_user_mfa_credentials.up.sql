CREATE TABLE user_mfa_credentials (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           TEXT    NOT NULL,
    type              TEXT    NOT NULL,
    secret_hash       TEXT    NOT NULL,
    recovery_codes    TEXT    NOT NULL DEFAULT '[]',
    name              TEXT    NOT NULL,
    enabled           INTEGER NOT NULL DEFAULT 1,
    last_used_counter INTEGER NOT NULL DEFAULT 0,
    last_used_at      DATETIME,
    enrolled_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at        DATETIME,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME,
    deleted_at        DATETIME
);

CREATE INDEX idx_mfa_user ON user_mfa_credentials (user_id) WHERE deleted_at IS NULL;
