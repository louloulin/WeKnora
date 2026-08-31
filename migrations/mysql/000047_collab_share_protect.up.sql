-- v0.7.38 Build #46.x — share-password protection on collaborative docs (MySQL mirror).
ALTER TABLE collaborative_docs
    ADD COLUMN share_password_hash VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN share_expires_at DATETIME NULL;
