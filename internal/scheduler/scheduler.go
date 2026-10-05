package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"

	"streak-guardian/internal/models"
	"streak-guardian/internal/repository"
	"streak-guardian/internal/service"
)

const maxWorkers = 10 // max concurrent GitHub API calls

// Scheduler runs periodic contribution checks and notification jobs.
type Scheduler struct {
	cron      *cron.Cron
	userRepo  repository.UserRepo
	streakSvc *service.StreakService
	notifSvc  *service.NotificationService
}

// New creates and configures the scheduler.
func New(
	userRepo repository.UserRepo,
	streakSvc *service.StreakService,
	notifSvc *service.NotificationService,
) *Scheduler {
	c := cron.New(cron.WithSeconds())
	s := &Scheduler{
		cron:      c,
		userRepo:  userRepo,
		streakSvc: streakSvc,
		notifSvc:  notifSvc,
	}

	// Job 1: Check contributions every 30 minutes
	c.AddFunc("0 */30 * * * *", func() {
		s.runContributionCheck()
	})

	// Job 2: Detect broken streaks at 00:05 AM UTC every day
	c.AddFunc("0 5 0 * * *", func() {
		s.runStreakBrokenCheck()
	})

	return s
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
	log.Println("⏰ Scheduler started (contribution check every 30 min, broken streak check daily at 00:05 UTC)")
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// runContributionCheck fetches contributions for all active users and sends
// reminders to those who haven't contributed by their reminder time.
func (s *Scheduler) runContributionCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	users, err := s.userRepo.GetAllActive(ctx)
	if err != nil {
		log.Printf("scheduler: error fetching active users: %v", err)
		return
	}

	log.Printf("scheduler: checking contributions for %d active user(s)", len(users))

	// Use a semaphore to limit concurrent GitHub API calls
	sem := make(chan struct{}, maxWorkers)
	g, ctx := errgroup.WithContext(ctx)

	for _, user := range users {
		u := user // capture loop variable
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			s.processUser(ctx, u)
			return nil
		})
	}

	_ = g.Wait()
}

// processUser checks and updates one user's streak, sending appropriate notifications.
func (s *Scheduler) processUser(ctx context.Context, user *models.User) {
	// Check if reminder time has passed in the user's timezone
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	reminderPassed := hasReminderTimePassed(now, user.ReminderTime)

	// Always update streak data
	info, err := s.streakSvc.CheckAndUpdateStreak(ctx, user)
	if err != nil {
		log.Printf("scheduler: error checking streak for user %s: %v", user.GitHubUsername, err)
		return
	}

	if info.TodayContributed {
		// If they contributed, send a "streak saved" if a reminder was sent earlier today
		if err := s.notifSvc.SendStreakSaved(ctx, user, info); err != nil {
			log.Printf("scheduler: error sending streak-saved to user %d: %v", user.ID, err)
		}
		return
	}

	// No contribution yet — send reminder if reminder time has passed
	if reminderPassed {
		if err := s.notifSvc.SendReminder(ctx, user, info); err != nil {
			log.Printf("scheduler: error sending reminder to user %d: %v", user.ID, err)
		}
	}
}

// runStreakBrokenCheck detects users whose streak broke overnight.
func (s *Scheduler) runStreakBrokenCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	users, err := s.userRepo.GetAllActive(ctx)
	if err != nil {
		log.Printf("scheduler: error fetching users for broken-streak check: %v", err)
		return
	}

	sem := make(chan struct{}, maxWorkers)
	g, ctx := errgroup.WithContext(ctx)

	for _, user := range users {
		u := user
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			s.checkBrokenStreak(ctx, u)
			return nil
		})
	}

	_ = g.Wait()
}

// checkBrokenStreak sends a STREAK_BROKEN notification if the user missed yesterday.
func (s *Scheduler) checkBrokenStreak(ctx context.Context, user *models.User) {
	info, err := s.streakSvc.GetStreakInfo(ctx, user)
	if err != nil {
		log.Printf("scheduler: broken-streak check error for user %d: %v", user.ID, err)
		return
	}

	// If the current streak is 0 and we had contributions before, the streak broke
	if info.CurrentStreak == 0 && !info.LastContribution.IsZero() {
		loc, _ := time.LoadLocation(user.Timezone)
		yesterday := time.Now().In(loc).Add(-24 * time.Hour).Truncate(24 * time.Hour)

		// Only notify if the last contribution was before yesterday (meaning they missed yesterday)
		lastDay := info.LastContribution.UTC().Truncate(24 * time.Hour)
		if lastDay.Before(yesterday) {
			// Compute previous streak from the dates before the break
			if err := s.notifSvc.SendStreakBroken(ctx, user, 0); err != nil {
				log.Printf("scheduler: error sending streak-broken to user %d: %v", user.ID, err)
			}
		}
	}
}

// hasReminderTimePassed returns true if the current time is past the reminder time.
func hasReminderTimePassed(now time.Time, reminderTime string) bool {
	// reminderTime format: "HH:MM"
	var hour, minute int
	if _, err := fmt.Sscanf(reminderTime, "%d:%d", &hour, &minute); err != nil {
		return false
	}
	reminderAt := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	return now.After(reminderAt)
}
