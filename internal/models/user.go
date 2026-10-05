package models

import "time"

// User represents a registered user of Streak Guardian.
type User struct {
	ID                  int64     `json:"id"`
	GitHubID            int64     `json:"github_id"`
	GitHubUsername      string    `json:"github_username"`
	GitHubAccessToken   string    `json:"-"` // encrypted at rest, never exposed in JSON
	TelegramChatID      int64     `json:"telegram_chat_id"`
	Timezone            string    `json:"timezone"`
	ReminderTime        string    `json:"reminder_time"` // "HH:MM" in 24h format, e.g. "20:00"
	NotificationEnabled bool      `json:"notification_enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// HasGitHubConnected returns true if the user has linked their GitHub account.
func (u *User) HasGitHubConnected() bool {
	return u.GitHubAccessToken != "" && u.GitHubUsername != ""
}
