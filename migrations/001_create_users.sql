-- 001_create_users.sql
-- Creates the users table.

CREATE TABLE IF NOT EXISTS users (
    id                   BIGSERIAL PRIMARY KEY,
    github_id            BIGINT       NOT NULL UNIQUE,
    github_username      VARCHAR(255) NOT NULL,
    github_access_token  TEXT         NOT NULL,          -- AES-256 encrypted
    telegram_chat_id     BIGINT       NOT NULL UNIQUE,
    timezone             VARCHAR(100) NOT NULL DEFAULT 'UTC',
    reminder_time        VARCHAR(5)   NOT NULL DEFAULT '20:00', -- HH:MM 24h
    notification_enabled BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_telegram_chat_id ON users(telegram_chat_id);
CREATE INDEX IF NOT EXISTS idx_users_github_id        ON users(github_id);
