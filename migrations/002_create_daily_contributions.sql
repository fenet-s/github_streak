-- 002_create_daily_contributions.sql
-- Records daily GitHub contribution data per user.

CREATE TABLE IF NOT EXISTS daily_contributions (
    id                 BIGSERIAL    PRIMARY KEY,
    user_id            BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contribution_date  DATE         NOT NULL,
    contribution_count INT          NOT NULL DEFAULT 0,
    status             VARCHAR(20)  NOT NULL DEFAULT 'NOT_CONTRIBUTED'
                           CHECK (status IN ('CONTRIBUTED', 'NOT_CONTRIBUTED', 'REMINDER_SENT')),
    checked_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE (user_id, contribution_date)
);

CREATE INDEX IF NOT EXISTS idx_daily_contributions_user_date
    ON daily_contributions(user_id, contribution_date DESC);
