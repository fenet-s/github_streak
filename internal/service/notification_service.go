package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"streak-guardian/internal/models"
	"streak-guardian/internal/repository"
)

// NotificationService sends Telegram notifications and records them.
type NotificationService struct {
	bot       *tgbotapi.BotAPI
	notifRepo repository.NotificationRepo
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(bot *tgbotapi.BotAPI, notifRepo repository.NotificationRepo) *NotificationService {
	return &NotificationService{bot: bot, notifRepo: notifRepo}
}

// SendReminder sends a "streak at risk" reminder if one hasn't been sent today.
func (s *NotificationService) SendReminder(ctx context.Context, user *models.User, info *models.StreakInfo) error {
	already, err := s.notifRepo.HasSentToday(ctx, user.ID, models.NotificationReminder)
	if err != nil {
		return fmt.Errorf("checking deduplication: %w", err)
	}
	if already {
		return nil // already sent today
	}

	return s.ForceSendReminder(ctx, user, info)
}

// ForceSendReminder sends a "streak at risk" reminder directly (used by automated jobs and /test).
func (s *NotificationService) ForceSendReminder(ctx context.Context, user *models.User, info *models.StreakInfo) error {
	// Calculate time remaining in the user's timezone
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, loc)
	remaining := endOfDay.Sub(now)

	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60

	var timeLeft string
	if hours > 0 {
		timeLeft = fmt.Sprintf("%d hour\\(s\\) and %d minute\\(s\\)", hours, minutes)
	} else {
		timeLeft = fmt.Sprintf("%d minute\\(s\\)", minutes)
	}

	text := fmt.Sprintf(
		"🚨 *Your GitHub streak is at risk\\!*\n\n"+
			"You haven't made a contribution today\\.\n\n"+
			"🔥 Current streak: *%d day\\(s\\)*\n\n"+
			"You still have %s\\.\n\n"+
			"Make a contribution before the day ends\\!",
		info.CurrentStreak,
		timeLeft,
	)

	if err := s.send(user.TelegramChatID, text); err != nil {
		s.recordNotification(ctx, user.ID, models.NotificationReminder, models.NotificationFailed)
		return err
	}

	s.recordNotification(ctx, user.ID, models.NotificationReminder, models.NotificationSent)
	return nil
}

// SendStreakSaved sends a "you're safe!" confirmation after a contribution is detected.
func (s *NotificationService) SendStreakSaved(ctx context.Context, user *models.User, info *models.StreakInfo) error {
	already, err := s.notifRepo.HasSentToday(ctx, user.ID, models.NotificationStreakSaved)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	text := fmt.Sprintf(
		"✅ *You're safe\\!*\n\n"+
			"Today's GitHub contribution has been detected\\.\n\n"+
			"🔥 Current streak: *%d day\\(s\\)*\n"+
			"🏆 Longest streak: *%d day\\(s\\)*\n\n"+
			"Keep going\\!",
		info.CurrentStreak,
		info.LongestStreak,
	)

	if err := s.send(user.TelegramChatID, text); err != nil {
		s.recordNotification(ctx, user.ID, models.NotificationStreakSaved, models.NotificationFailed)
		return err
	}

	s.recordNotification(ctx, user.ID, models.NotificationStreakSaved, models.NotificationSent)
	return nil
}

// SendStreakBroken notifies the user that their streak has ended.
func (s *NotificationService) SendStreakBroken(ctx context.Context, user *models.User, previousStreak int) error {
	already, err := s.notifRepo.HasSentToday(ctx, user.ID, models.NotificationStreakBroken)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	text := fmt.Sprintf(
		"💀 *Streak ended\\.*\n\n"+
			"You didn't make a qualifying contribution yesterday\\.\n\n"+
			"Previous streak: *%d day\\(s\\)*\n"+
			"Current streak: *0 days*\n\n"+
			"Start a new streak today\\! 🔥",
		previousStreak,
	)

	if err := s.send(user.TelegramChatID, text); err != nil {
		s.recordNotification(ctx, user.ID, models.NotificationStreakBroken, models.NotificationFailed)
		return err
	}

	s.recordNotification(ctx, user.ID, models.NotificationStreakBroken, models.NotificationSent)
	return nil
}

// send delivers a Markdown V2 message to a Telegram chat, falling back to plain text if needed.
func (s *NotificationService) send(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	if _, err := s.bot.Send(msg); err != nil {
		log.Printf("warning: markdown send failed (%v), falling back to plain text", err)
		plain := tgbotapi.NewMessage(chatID, stripMarkdown(text))
		if _, plainErr := s.bot.Send(plain); plainErr != nil {
			return fmt.Errorf("sending Telegram message to chat %d: %w", chatID, plainErr)
		}
	}
	return nil
}

// recordNotification saves a notification record, logging any errors.
func (s *NotificationService) recordNotification(ctx context.Context, userID int64, notifType models.NotificationType, status models.NotificationStatus) {
	err := s.notifRepo.Record(ctx, &models.Notification{
		UserID:           userID,
		NotificationType: notifType,
		NotificationDate: time.Now().UTC().Truncate(24 * time.Hour),
		Status:           status,
	})
	if err != nil {
		log.Printf("warning: failed to record notification for user %d: %v", userID, err)
	}
}

// escapeMarkdown escapes special characters for Telegram MarkdownV2.
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}

func stripMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"*", "", "_", "", "\\.", ".", "\\!", "!", "\\-", "-", "\\(", "(", "\\)", ")",
	)
	return replacer.Replace(s)
}
