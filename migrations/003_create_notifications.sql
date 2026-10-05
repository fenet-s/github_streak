-- 003_create_notifications.sql
-- Tracks every notification sent. Used to prevent duplicate reminders.

CREATE TABLE IF NOT EXISTS notifications (
    id                BIGSERIAL   PRIMARY KEY,
    user_id           BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type VARCHAR(20) NOT NULL
                          CHECK (notification_type IN ('REMINDER', 'STREAK_SAVED', 'STREAK_BROKEN')),
    notification_date DATE        NOT NULL,
    sent_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status            VARCHAR(10) NOT NULL DEFAULT 'SENT'
                          CHECK (status IN ('SENT', 'FAILED')),

    -- Prevent sending the same notification type more than once per day per user
    UNIQUE (user_id, notification_type, notification_date)
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_date
    ON notifications(user_id, notification_date DESC);
