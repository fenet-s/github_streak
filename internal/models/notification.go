package models

import "time"

// NotificationType categorises the purpose of a notification.
type NotificationType string

const (
	NotificationReminder     NotificationType = "REMINDER"
	NotificationStreakSaved  NotificationType = "STREAK_SAVED"
	NotificationStreakBroken NotificationType = "STREAK_BROKEN"
)

// NotificationStatus tracks delivery state.
type NotificationStatus string

const (
	NotificationSent   NotificationStatus = "SENT"
	NotificationFailed NotificationStatus = "FAILED"
)

// Notification records every outbound notification sent to a user.
// The (user_id, notification_type, notification_date) combination is
// used to prevent duplicate reminders on the same day.
type Notification struct {
	ID               int64              `json:"id"`
	UserID           int64              `json:"user_id"`
	NotificationType NotificationType   `json:"notification_type"`
	NotificationDate time.Time          `json:"notification_date"`
	SentAt           time.Time          `json:"sent_at"`
	Status           NotificationStatus `json:"status"`
}
