package repository

import (
	"context"
	"time"

	"streak-guardian/internal/models"
)

// UserRepo defines the contract for user storage.
// Services depend on this interface, not the concrete implementation.
type UserRepo interface {
	Create(ctx context.Context, u *models.User) (*models.User, error)
	Upsert(ctx context.Context, u *models.User) (*models.User, error)
	GetByTelegramChatID(ctx context.Context, chatID int64) (*models.User, error)
	GetByGitHubID(ctx context.Context, githubID int64) (*models.User, error)
	GetByID(ctx context.Context, id int64) (*models.User, error)
	GetAllActive(ctx context.Context) ([]*models.User, error)
	UpdateSettings(ctx context.Context, userID int64, reminderTime, timezone string, notifEnabled bool) error
	UpdateToken(ctx context.Context, userID int64, encryptedToken string) error
}

// ContributionRepo defines the contract for daily contribution storage.
type ContributionRepo interface {
	Upsert(ctx context.Context, c *models.DailyContribution) (*models.DailyContribution, error)
	GetByDate(ctx context.Context, userID int64, date time.Time) (*models.DailyContribution, error)
	GetRecentDates(ctx context.Context, userID int64, limit int) ([]time.Time, error)
	CountThisYear(ctx context.Context, userID int64) (int, error)
}

// NotificationRepo defines the contract for notification storage.
type NotificationRepo interface {
	HasSentToday(ctx context.Context, userID int64, notifType models.NotificationType) (bool, error)
	Record(ctx context.Context, n *models.Notification) error
}

// Compile-time checks: ensure concrete types satisfy the interfaces.
// This causes a build error immediately if any method is missing.
var _ UserRepo = (*UserRepository)(nil)
var _ ContributionRepo = (*ContributionRepository)(nil)
var _ NotificationRepo = (*NotificationRepository)(nil)
