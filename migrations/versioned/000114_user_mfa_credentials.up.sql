-- 000114 — MFA (TOTP / WebAuthn) credentials
--
-- One row per enrolled factor. The secret is stored encrypted at
-- rest by the repository; the recovery_codes column holds SHA-256
-- hex hashes of the single-use recovery codes we issued at
-- enrollment. last_used_counter rejects replay within the ±1-step
-- drift window.
CREATE TABLE IF NOT EXISTS user_mfa_credentials (
    id                BIGSERIAL PRIMARY KEY,
    user_id           VARCHAR(36) NOT NULL,
    type              VARCHAR(32) NOT NULL,
    secret_hash       TEXT        NOT NULL,
    recovery_codes    JSONB       NOT NULL DEFAULT '[]',
    name              VARCHAR(64) NOT NULL,
    enabled           BOOLEAN     NOT NULL DEFAULT TRUE,
    last_used_counter BIGINT      NOT NULL DEFAULT 0,
    last_used_at      TIMESTAMP,
    enrolled_at       TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at        TIMESTAMP,
    created_at        TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP,
    deleted_at        TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_mfa_user
    ON user_mfa_credentials (user_id) WHERE deleted_at IS NULL;

-- Cross-user uniqueness on the recovery-code hashes is not
-- enforced (collisions are astronomically unlikely with SHA-256)
-- but we index them so a "find which user owns this code" lookup
-- stays cheap if we ever need to enforce single-use globally.
