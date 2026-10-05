package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"streak-guardian/internal/crypto"
	githubclient "streak-guardian/internal/github"
	"streak-guardian/internal/models"
	"streak-guardian/internal/repository"
)

// StreakService handles streak calculation and contribution checking.
type StreakService struct {
	userRepo         repository.UserRepo
	contributionRepo repository.ContributionRepo
	githubClient     *githubclient.Client
	encryptionKey    string
}

// NewStreakService creates a new StreakService.
func NewStreakService(
	userRepo repository.UserRepo,
	contributionRepo repository.ContributionRepo,
	githubClient *githubclient.Client,
	encryptionKey string,
) *StreakService {
	return &StreakService{
		userRepo:         userRepo,
		contributionRepo: contributionRepo,
		githubClient:     githubClient,
		encryptionKey:    encryptionKey,
	}
}

// CheckAndUpdateStreak fetches today's GitHub contributions for a user,
// updates the daily_contributions table, and returns the current streak info.
func (s *StreakService) CheckAndUpdateStreak(ctx context.Context, user *models.User) (*models.StreakInfo, error) {
	// Determine "today" in the user's timezone
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		loc = time.UTC
	}
	today := time.Now().In(loc).Truncate(24 * time.Hour)

	// Decrypt the stored GitHub token
	token, err := decryptToken(user.GitHubAccessToken, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting token for user %d: %w", user.ID, err)
	}

	// Fetch today's contribution count from GitHub
	count, err := s.githubClient.GetContributionCount(ctx, token, user.GitHubUsername, today)
	if err != nil {
		return nil, fmt.Errorf("fetching contributions for user %s: %w", user.GitHubUsername, err)
	}

	// Determine status
	status := models.StatusNotContributed
	if count > 0 {
		status = models.StatusContributed
	}

	// Upsert today's record
	if _, err := s.contributionRepo.Upsert(ctx, &models.DailyContribution{
		UserID:            user.ID,
		ContributionDate:  today,
		ContributionCount: count,
		Status:            status,
	}); err != nil {
		return nil, fmt.Errorf("upserting contribution: %w", err)
	}

	// Build and return full streak info
	return s.GetStreakInfo(ctx, user)
}

// GetStreakInfo computes the full streak info for a user from their stored records.
func (s *StreakService) GetStreakInfo(ctx context.Context, user *models.User) (*models.StreakInfo, error) {
	// Get the last 400 contribution dates (covers ~1 year generously)
	dates, err := s.contributionRepo.GetRecentDates(ctx, user.ID, 400)
	if err != nil {
		return nil, fmt.Errorf("fetching recent dates: %w", err)
	}

	current, longest, lastContrib := CalculateStreak(dates)

	yearCount, err := s.contributionRepo.CountThisYear(ctx, user.ID)
	if err != nil {
		log.Printf("warning: could not count yearly contributions for user %d: %v", user.ID, err)
	}

	// Check today
	loc, _ := time.LoadLocation(user.Timezone)
	today := time.Now().In(loc).Truncate(24 * time.Hour)

	todayContributed := false
	todayCount := 0
	rec, err := s.contributionRepo.GetByDate(ctx, user.ID, today)
	if err == nil {
		todayContributed = rec.ContributionCount > 0
		todayCount = rec.ContributionCount
	}

	return &models.StreakInfo{
		CurrentStreak:     current,
		LongestStreak:     longest,
		LastContribution:  lastContrib,
		ContributionsYear: yearCount,
		TodayContributed:  todayContributed,
		TodayCount:        todayCount,
	}, nil
}

// CalculateStreak computes the current and longest streak from a list of
// contribution dates in descending order (most recent first).
// This is a pure function — easy to unit test independently.
func CalculateStreak(dates []time.Time) (current, longest int, lastContrib time.Time) {
	if len(dates) == 0 {
		return 0, 0, time.Time{}
	}

	lastContrib = dates[0]

	// Normalize all dates to UTC midnight for comparison
	normalized := make([]time.Time, len(dates))
	for i, d := range dates {
		normalized[i] = d.UTC().Truncate(24 * time.Hour)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)

	// Current streak: count consecutive days ending today or yesterday
	current = 0
	if len(normalized) > 0 {
		mostRecent := normalized[0]
		// Streak is only alive if the most recent contribution is today or yesterday
		if mostRecent.Equal(today) || mostRecent.Equal(yesterday) {
			current = 1
			for i := 1; i < len(normalized); i++ {
				expected := normalized[i-1].Add(-24 * time.Hour)
				if normalized[i].Equal(expected) {
					current++
				} else {
					break
				}
			}
		}
	}

	// Longest streak: scan all dates for the longest consecutive run
	longest = 0
	if len(normalized) > 0 {
		run := 1
		for i := 1; i < len(normalized); i++ {
			expected := normalized[i-1].Add(-24 * time.Hour)
			if normalized[i].Equal(expected) {
				run++
			} else {
				if run > longest {
					longest = run
				}
				run = 1
			}
		}
		if run > longest {
			longest = run
		}
	}
	if current > longest {
		longest = current
	}

	return current, longest, lastContrib
}

// decryptToken decrypts an AES-encrypted GitHub access token.
func decryptToken(encrypted, key string) (string, error) {
	return crypto.Decrypt(encrypted, key)
}
