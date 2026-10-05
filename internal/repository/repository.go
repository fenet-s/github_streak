package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"streak-guardian/internal/models"
)

// ErrNotFound is returned when a record is not found.
var ErrNotFound = errors.New("record not found")

// UserRepository handles all database operations for users.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create inserts a new user and returns the created record.
func (r *UserRepository) Create(ctx context.Context, u *models.User) (*models.User, error) {
	query := `
		INSERT INTO users (
			github_id, github_username, github_access_token,
			telegram_chat_id, timezone, reminder_time, notification_enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		u.GitHubID, u.GitHubUsername, u.GitHubAccessToken,
		u.TelegramChatID, u.Timezone, u.ReminderTime, u.NotificationEnabled,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return u, nil
}

// Upsert inserts or updates a user by GitHub ID. Used during OAuth callback.
func (r *UserRepository) Upsert(ctx context.Context, u *models.User) (*models.User, error) {
	query := `
		INSERT INTO users (
			github_id, github_username, github_access_token,
			telegram_chat_id, timezone, reminder_time, notification_enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (github_id) DO UPDATE SET
			github_username      = EXCLUDED.github_username,
			github_access_token  = EXCLUDED.github_access_token,
			telegram_chat_id     = EXCLUDED.telegram_chat_id,
			updated_at           = NOW()
		RETURNING id, created_at, updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		u.GitHubID, u.GitHubUsername, u.GitHubAccessToken,
		u.TelegramChatID, u.Timezone, u.ReminderTime, u.NotificationEnabled,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting user: %w", err)
	}
	return u, nil
}

// GetByTelegramChatID finds a user by their Telegram chat ID.
func (r *UserRepository) GetByTelegramChatID(ctx context.Context, chatID int64) (*models.User, error) {
	return r.scanOne(ctx, `SELECT * FROM users WHERE telegram_chat_id = $1`, chatID)
}

// GetByGitHubID finds a user by their GitHub user ID.
func (r *UserRepository) GetByGitHubID(ctx context.Context, githubID int64) (*models.User, error) {
	return r.scanOne(ctx, `SELECT * FROM users WHERE github_id = $1`, githubID)
}

// GetByID finds a user by primary key.
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	return r.scanOne(ctx, `SELECT * FROM users WHERE id = $1`, id)
}

// GetAllActive returns all users with notifications enabled.
func (r *UserRepository) GetAllActive(ctx context.Context) ([]*models.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT * FROM users WHERE notification_enabled = TRUE`)
	if err != nil {
		return nil, fmt.Errorf("querying active users: %w", err)
	}
	defer rows.Close()
	return r.scanRows(rows)
}

// UpdateSettings updates reminder_time, timezone, and notification_enabled.
func (r *UserRepository) UpdateSettings(ctx context.Context, userID int64, reminderTime, timezone string, notifEnabled bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET reminder_time = $1, timezone = $2, notification_enabled = $3, updated_at = NOW()
		WHERE id = $4
	`, reminderTime, timezone, notifEnabled, userID)
	if err != nil {
		return fmt.Errorf("updating settings: %w", err)
	}
	return nil
}

// UpdateToken updates a user's GitHub access token (should be pre-encrypted).
func (r *UserRepository) UpdateToken(ctx context.Context, userID int64, encryptedToken string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET github_access_token = $1, updated_at = NOW() WHERE id = $2
	`, encryptedToken, userID)
	return err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (r *UserRepository) scanOne(ctx context.Context, query string, args ...any) (*models.User, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}
	defer rows.Close()

	users, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, ErrNotFound
	}
	return users[0], nil
}

func (r *UserRepository) scanRows(rows pgx.Rows) ([]*models.User, error) {
	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		err := rows.Scan(
			&u.ID, &u.GitHubID, &u.GitHubUsername, &u.GitHubAccessToken,
			&u.TelegramChatID, &u.Timezone, &u.ReminderTime, &u.NotificationEnabled,
			&u.CreatedAt, &u.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user rows: %w", err)
	}
	return users, nil
}

// ─── ContributionRepository ───────────────────────────────────────────────────

// ContributionRepository handles database operations for daily contributions.
type ContributionRepository struct {
	pool *pgxpool.Pool
}

// NewContributionRepository creates a new ContributionRepository.
func NewContributionRepository(pool *pgxpool.Pool) *ContributionRepository {
	return &ContributionRepository{pool: pool}
}

// Upsert inserts or updates a daily contribution record.
func (r *ContributionRepository) Upsert(ctx context.Context, c *models.DailyContribution) (*models.DailyContribution, error) {
	query := `
		INSERT INTO daily_contributions (user_id, contribution_date, contribution_count, status, checked_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id, contribution_date) DO UPDATE SET
			contribution_count = EXCLUDED.contribution_count,
			status             = EXCLUDED.status,
			checked_at         = NOW()
		RETURNING id, checked_at, created_at
	`
	err := r.pool.QueryRow(ctx, query,
		c.UserID, c.ContributionDate, c.ContributionCount, c.Status,
	).Scan(&c.ID, &c.CheckedAt, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting contribution: %w", err)
	}
	return c, nil
}

// GetByDate returns a user's contribution record for a specific date.
func (r *ContributionRepository) GetByDate(ctx context.Context, userID int64, date time.Time) (*models.DailyContribution, error) {
	c := &models.DailyContribution{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, contribution_date, contribution_count, status, checked_at, created_at
		FROM daily_contributions
		WHERE user_id = $1 AND contribution_date = $2
	`, userID, date.Format("2006-01-02")).Scan(
		&c.ID, &c.UserID, &c.ContributionDate, &c.ContributionCount,
		&c.Status, &c.CheckedAt, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting contribution by date: %w", err)
	}
	return c, nil
}

// GetRecentDates returns the last N contribution dates for a user.
func (r *ContributionRepository) GetRecentDates(ctx context.Context, userID int64, limit int) ([]time.Time, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT contribution_date FROM daily_contributions
		WHERE user_id = $1 AND contribution_count > 0
		ORDER BY contribution_date DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("getting recent contribution dates: %w", err)
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

// CountThisYear returns the total number of contribution days for the current year.
func (r *ContributionRepository) CountThisYear(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM daily_contributions
		WHERE user_id = $1
		  AND contribution_count > 0
		  AND EXTRACT(YEAR FROM contribution_date) = EXTRACT(YEAR FROM NOW())
	`, userID).Scan(&count)
	return count, err
}

// ─── NotificationRepository ───────────────────────────────────────────────────

// NotificationRepository handles database operations for notifications.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRepository creates a new NotificationRepository.
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

// HasSentToday checks if a notification of the given type was already sent today.
func (r *NotificationRepository) HasSentToday(ctx context.Context, userID int64, notifType models.NotificationType) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notifications
		WHERE user_id = $1
		  AND notification_type = $2
		  AND notification_date = CURRENT_DATE
		  AND status = 'SENT'
	`, userID, notifType).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking notification deduplication: %w", err)
	}
	return count > 0, nil
}

// Record saves a notification record after sending.
func (r *NotificationRepository) Record(ctx context.Context, n *models.Notification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notifications (user_id, notification_type, notification_date, sent_at, status)
		VALUES ($1, $2, $3, NOW(), $4)
		ON CONFLICT (user_id, notification_type, notification_date) DO NOTHING
	`, n.UserID, n.NotificationType, n.NotificationDate, n.Status)
	if err != nil {
		return fmt.Errorf("recording notification: %w", err)
	}
	return nil
}
