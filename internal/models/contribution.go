package models

import "time"

// DailyStatus represents whether the user has contributed on a given day.
type DailyStatus string

const (
	StatusContributed    DailyStatus = "CONTRIBUTED"
	StatusNotContributed DailyStatus = "NOT_CONTRIBUTED"
	StatusReminderSent   DailyStatus = "REMINDER_SENT"
)

// DailyContribution records the contribution activity for one user on one day.
type DailyContribution struct {
	ID                int64       `json:"id"`
	UserID            int64       `json:"user_id"`
	ContributionDate  time.Time   `json:"contribution_date"`
	ContributionCount int         `json:"contribution_count"`
	Status            DailyStatus `json:"status"`
	CheckedAt         time.Time   `json:"checked_at"`
	CreatedAt         time.Time   `json:"created_at"`
}

// StreakInfo holds calculated streak data for a user.
type StreakInfo struct {
	CurrentStreak      int       `json:"current_streak"`
	LongestStreak      int       `json:"longest_streak"`
	LastContribution   time.Time `json:"last_contribution"`
	ContributionsYear  int       `json:"contributions_this_year"`
	TodayContributed   bool      `json:"today_contributed"`
	TodayCount         int       `json:"today_count"`
}
