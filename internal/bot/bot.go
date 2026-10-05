package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"streak-guardian/internal/crypto"
	githubclient "streak-guardian/internal/github"
	"streak-guardian/internal/models"
	"streak-guardian/internal/repository"
	"streak-guardian/internal/service"
)

// Bot holds all dependencies needed to handle Telegram commands.
type Bot struct {
	api           *tgbotapi.BotAPI
	userRepo      repository.UserRepo
	streakSvc     *service.StreakService
	notifSvc      *service.NotificationService
	oauthCfg      *githubclient.OAuthConfig
	encryptionKey string
}

// New creates a new Bot instance.
func New(
	api *tgbotapi.BotAPI,
	userRepo repository.UserRepo,
	streakSvc *service.StreakService,
	notifSvc *service.NotificationService,
	oauthCfg *githubclient.OAuthConfig,
	encryptionKey string,
) *Bot {
	return &Bot{
		api:           api,
		userRepo:      userRepo,
		streakSvc:     streakSvc,
		notifSvc:      notifSvc,
		oauthCfg:      oauthCfg,
		encryptionKey: encryptionKey,
	}
}

// Start begins long-polling for updates. Blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)
	log.Printf("🤖 Telegram bot started (long polling)")

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return
		case update := <-updates:
			if update.Message == nil {
				continue
			}
			go b.handleMessage(ctx, update.Message)
		}
	}
}

// handleMessage dispatches incoming messages to the appropriate handler.
func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if !msg.IsCommand() {
		return
	}

	switch msg.Command() {
	case "start":
		b.handleStart(ctx, msg)
	case "status":
		b.handleStatus(ctx, msg)
	case "streak":
		b.handleStreak(ctx, msg)
	case "settings":
		b.handleSettings(ctx, msg)
	case "settime":
		b.handleSetTime(ctx, msg)
	case "settimezone":
		b.handleSetTimezone(ctx, msg)
	case "test", "testreminder":
		b.handleTest(ctx, msg)
	case "stop":
		b.handleStop(ctx, msg)
	case "resume":
		b.handleResume(ctx, msg)
	case "help":
		b.handleHelp(ctx, msg)
	default:
		b.reply(msg.Chat.ID, "Unknown command. Send /help to see available commands.")
	}
}

// ─── /start ──────────────────────────────────────────────────────────────────

func (b *Bot) handleStart(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Check if user already exists
	user, err := b.userRepo.GetByTelegramChatID(ctx, chatID)
	if err == nil && user.HasGitHubConnected() {
		// Already registered and connected
		b.reply(chatID, fmt.Sprintf(
			"👋 Welcome back, *%s*\\!\n\nYour GitHub account is already connected\\.\n\nSend /status to check your streak\\.",
			escMD(user.GitHubUsername),
		))
		return
	}

	// Generate OAuth URL with the Telegram chat ID embedded in the state
	authURL, err := b.oauthCfg.GenerateAuthURL(chatID)
	if err != nil {
		log.Printf("error generating auth URL for chat %d: %v", chatID, err)
		b.reply(chatID, "❌ Something went wrong. Please try again.")
		return
	}

	text := fmt.Sprintf(
		"👋 *Welcome to Streak Guardian\\!*\n\n"+
			"I'll help you protect your GitHub contribution streak\\.\n\n"+
			"*Connect your GitHub account to get started:*\n\n"+
			"[🔗 Connect GitHub](%s)\n\n"+
			"_The link expires in 10 minutes\\._",
		authURL,
	)

	b.replyMD(chatID, text)
}

// ─── /status ─────────────────────────────────────────────────────────────────

func (b *Bot) handleStatus(ctx context.Context, msg *tgbotapi.Message) {
	user, ok := b.requireGitHub(ctx, msg)
	if !ok {
		return
	}

	info, err := b.streakSvc.CheckAndUpdateStreak(ctx, user)
	if err != nil {
		log.Printf("error checking streak for user %d: %v", user.ID, err)
		b.reply(msg.Chat.ID, "❌ Failed to fetch your GitHub data. Please try again.")
		return
	}

	todayStatus := "❌ No contribution yet"
	if info.TodayContributed {
		todayStatus = fmt.Sprintf("✅ Contributed \\(%d contribution\\(s\\)\\)", info.TodayCount)
	}

	text := fmt.Sprintf(
		"🔥 *GitHub Streak*\n\n"+
			"Current streak: *%d day\\(s\\)*\n"+
			"Longest streak: *%d day\\(s\\)*\n\n"+
			"Today's status: %s\n\n"+
			"Reminder: *%s* \\(%s\\)",
		info.CurrentStreak,
		info.LongestStreak,
		todayStatus,
		escMD(user.ReminderTime),
		escMD(user.Timezone),
	)

	b.replyMD(msg.Chat.ID, text)
}

// ─── /streak ─────────────────────────────────────────────────────────────────

func (b *Bot) handleStreak(ctx context.Context, msg *tgbotapi.Message) {
	user, ok := b.requireGitHub(ctx, msg)
	if !ok {
		return
	}

	info, err := b.streakSvc.GetStreakInfo(ctx, user)
	if err != nil {
		b.reply(msg.Chat.ID, "❌ Failed to fetch your streak info.")
		return
	}

	lastDate := "N/A"
	if !info.LastContribution.IsZero() {
		lastDate = info.LastContribution.Format("January 2, 2006")
	}

	text := fmt.Sprintf(
		"🔥 *Your GitHub Streak*\n\n"+
			"Current: *%d day(s)*\n"+
			"Longest: *%d day(s)*\n\n"+
			"Contributions this year: *%d*\n\n"+
			"Last contribution:\n_%s_",
		info.CurrentStreak,
		info.LongestStreak,
		info.ContributionsYear,
		escMD(lastDate),
	)

	b.replyMD(msg.Chat.ID, text)
}

// ─── /settings ───────────────────────────────────────────────────────────────

func (b *Bot) handleSettings(ctx context.Context, msg *tgbotapi.Message) {
	user, ok := b.requireGitHub(ctx, msg)
	if !ok {
		return
	}

	// Show current settings and instructions for updating them
	text := fmt.Sprintf(
		"⚙️ *Settings*\n\n"+
			"GitHub: *%s*\n"+
			"Reminder time: *%s*\n"+
			"Timezone: *%s*\n"+
			"Notifications: *%s*\n\n"+
			"To change your reminder time, send:\n"+
			"`/settime HH:MM` \\(e\\.g\\. `/settime 20:00`\\)\n\n"+
			"To change your timezone, send:\n"+
			"`/settimezone Region/City` \\(e\\.g\\. `/settimezone Africa/Addis_Ababa`\\)",
		escMD(user.GitHubUsername),
		escMD(user.ReminderTime),
		escMD(user.Timezone),
		notifStatus(user.NotificationEnabled),
	)

	b.replyMD(msg.Chat.ID, text)
}

// ─── /settime ────────────────────────────────────────────────────────────────

// handleSetTime updates the user's daily reminder time.
// Supports both 24-hour format (e.g. 20:00, 13:20) and 12-hour AM/PM format (e.g. 1:20 PM, 1:20pm, 8:00 AM).
func (b *Bot) handleSetTime(ctx context.Context, msg *tgbotapi.Message) {
	user, ok := b.requireGitHub(ctx, msg)
	if !ok {
		return
	}

	arg := msg.CommandArguments() // everything after /settime
	if strings.TrimSpace(arg) == "" {
		b.replyMD(msg.Chat.ID, "⚠️ Please provide a time\\.\n\nExamples: `/settime 20:00` or `/settime 1:20 PM`")
		return
	}

	reminderTime, err := parseTimeArg(arg)
	if err != nil {
		b.replyMD(msg.Chat.ID, "❌ Invalid time format\\.\n\nExamples:\n• 24\\-hour: `/settime 20:00`\n• 12\\-hour: `/settime 1:20 PM`")
		return
	}

	if err := b.userRepo.UpdateSettings(ctx, user.ID, reminderTime, user.Timezone, user.NotificationEnabled); err != nil {
		b.reply(msg.Chat.ID, "❌ Failed to update reminder time. Please try again.")
		return
	}

	b.replyMD(msg.Chat.ID, fmt.Sprintf(
		"✅ Reminder time updated to *%s* \\(%s\\)\\.\n\nI'll check your GitHub activity at this time every day\\.",
		escMD(reminderTime),
		escMD(user.Timezone),
	))
}

func parseTimeArg(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	rawUpper := strings.ToUpper(raw)
	for strings.Contains(rawUpper, "  ") {
		rawUpper = strings.ReplaceAll(rawUpper, "  ", " ")
	}

	formats := []string{
		"15:04",
		"3:04 PM",
		"3:04PM",
		"03:04 PM",
		"03:04PM",
		"3 PM",
		"3PM",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, rawUpper); err == nil {
			return t.Format("15:04"), nil
		}
	}
	return "", fmt.Errorf("invalid time format")
}

// ─── /settimezone ─────────────────────────────────────────────────────────────

// handleSetTimezone updates the user's timezone.
// Usage: /settimezone Africa/Addis_Ababa
func (b *Bot) handleSetTimezone(ctx context.Context, msg *tgbotapi.Message) {
	user, ok := b.requireGitHub(ctx, msg)
	if !ok {
		return
	}

	tz := msg.CommandArguments()
	if tz == "" {
		b.replyMD(msg.Chat.ID,
			"⚠️ Please provide a timezone\\.\n\n"+
				"Example: `/settimezone Africa/Addis_Ababa`\n\n"+
				"Common timezones:\n"+
				"`Africa/Addis_Ababa`\n"+
				"`America/New_York`\n"+
				"`Europe/London`\n"+
				"`Asia/Tokyo`\n"+
				"`UTC`",
		)
		return
	}

	// Validate the timezone using Go's time package
	if _, err := time.LoadLocation(tz); err != nil {
		b.replyMD(msg.Chat.ID, fmt.Sprintf(
			"❌ Unknown timezone: `%s`\n\nUse a valid IANA timezone like `Africa/Addis_Ababa` or `America/New_York`\\.",
			escMD(tz),
		))
		return
	}

	if err := b.userRepo.UpdateSettings(ctx, user.ID, user.ReminderTime, tz, user.NotificationEnabled); err != nil {
		b.reply(msg.Chat.ID, "❌ Failed to update timezone. Please try again.")
		return
	}

	b.replyMD(msg.Chat.ID, fmt.Sprintf(
		"✅ Timezone updated to *%s*\\.\n\nYour daily reminder will now fire at *%s %s*\\.",
		escMD(tz),
		escMD(user.ReminderTime),
		escMD(tz),
	))
}

// ─── /stop ──────────────────────────────────────────────────────────────────

// handleStop disables all notifications for the user.
func (b *Bot) handleStop(ctx context.Context, msg *tgbotapi.Message) {
	user, err := b.userRepo.GetByTelegramChatID(ctx, msg.Chat.ID)
	if err != nil {
		b.reply(msg.Chat.ID, "You don't have an account yet. Send /start to get started.")
		return
	}

	if !user.NotificationEnabled {
		b.replyMD(msg.Chat.ID, "\u26a0\ufe0f Notifications are already paused\\.\n\nSend /resume to turn them back on\\.")
		return
	}

	if err := b.userRepo.UpdateSettings(ctx, user.ID, user.ReminderTime, user.Timezone, false); err != nil {
		b.reply(msg.Chat.ID, "❌ Failed to pause notifications. Please try again.")
		return
	}

	b.replyMD(msg.Chat.ID,
		"\U0001f515 *Notifications paused*\\.\n\n"+
			"I won't send you any more reminders\\.\n\n"+
			"Send /resume whenever you want to turn them back on\\.",
	)
}

// ─── /resume ─────────────────────────────────────────────────────────────────

// handleResume re-enables notifications for the user.
func (b *Bot) handleResume(ctx context.Context, msg *tgbotapi.Message) {
	user, err := b.userRepo.GetByTelegramChatID(ctx, msg.Chat.ID)
	if err != nil {
		b.reply(msg.Chat.ID, "You don't have an account yet. Send /start to get started.")
		return
	}

	if user.NotificationEnabled {
		b.replyMD(msg.Chat.ID, "\u2705 Notifications are already enabled\\.\n\nSend /stop to pause them\\.")
		return
	}

	if err := b.userRepo.UpdateSettings(ctx, user.ID, user.ReminderTime, user.Timezone, true); err != nil {
		b.reply(msg.Chat.ID, "❌ Failed to resume notifications. Please try again.")
		return
	}

	b.replyMD(msg.Chat.ID,
		"\U0001f514 *Notifications resumed*\\.\n\n"+
			"I'll remind you again at *"+escMD(user.ReminderTime)+"* if you haven't contributed\\.",
	)
}

// ─── /test ───────────────────────────────────────────────────────────────────

// handleTest sends an immediate test notification so the user doesn't have to wait for the scheduler.
func (b *Bot) handleTest(ctx context.Context, msg *tgbotapi.Message) {
	user, ok := b.requireGitHub(ctx, msg)
	if !ok {
		return
	}

	b.reply(msg.Chat.ID, "🧪 Checking GitHub activity and sending test notification...")

	info, err := b.streakSvc.CheckAndUpdateStreak(ctx, user)
	if err != nil {
		b.reply(msg.Chat.ID, "❌ Failed to fetch your GitHub data: "+err.Error())
		return
	}

	if info.TodayContributed {
		if err := b.notifSvc.SendStreakSaved(ctx, user, info); err != nil {
			b.reply(msg.Chat.ID, "❌ Failed to send notification: "+err.Error())
		}
	} else {
		if err := b.notifSvc.ForceSendReminder(ctx, user, info); err != nil {
			b.reply(msg.Chat.ID, "❌ Failed to send notification: "+err.Error())
		}
	}
}

// ─── /help ───────────────────────────────────────────────────────────────────

func (b *Bot) handleHelp(_ context.Context, msg *tgbotapi.Message) {
	text := "🛡️ *Streak Guardian — Commands*\n\n" +
		"/start — Connect your GitHub account\n" +
		"/status — Check today's status and streak\n" +
		"/streak — View detailed streak information\n" +
		"/settings — View and update your settings\n" +
		"/settime HH:MM — Set reminder time \\(e\\.g\\. `20:00` or `1:20 PM`\\)\n" +
		"/settimezone Region/City — Set your timezone\n" +
		"/stop — Pause all notifications\n" +
		"/resume — Resume notifications\n" +
		"/test — Test notification alert right now\n" +
		"/help — Show this help message"

	b.replyMD(msg.Chat.ID, text)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// requireGitHub looks up the user and verifies they have connected GitHub.
// Returns false and sends an error message if they haven't.
func (b *Bot) requireGitHub(ctx context.Context, msg *tgbotapi.Message) (*models.User, bool) {
	user, err := b.userRepo.GetByTelegramChatID(ctx, msg.Chat.ID)
	if err != nil {
		authURL, _ := b.oauthCfg.GenerateAuthURL(msg.Chat.ID)
		b.replyMD(msg.Chat.ID, fmt.Sprintf(
			"👋 You haven't connected your GitHub account yet\\.\n\n[🔗 Connect GitHub](%s)", authURL,
		))
		return nil, false
	}
	if !user.HasGitHubConnected() {
		authURL, _ := b.oauthCfg.GenerateAuthURL(msg.Chat.ID)
		b.replyMD(msg.Chat.ID, fmt.Sprintf(
			"⚠️ Your GitHub account is not connected\\.\n\n[🔗 Connect GitHub](%s)", authURL,
		))
		return nil, false
	}
	return user, true
}

// reply sends a plain-text message.
func (b *Bot) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("error sending message to chat %d: %v", chatID, err)
	}
}

// replyMD sends a MarkdownV2 formatted message.
func (b *Bot) replyMD(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	msg.DisableWebPagePreview = true
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("error sending MD message to chat %d: %v", chatID, err)
		// Fallback to plain text
		plain := tgbotapi.NewMessage(chatID, stripMarkdown(text))
		b.api.Send(plain)
	}
}

// SendGitHubConnected sends a success message after OAuth completion.
func (b *Bot) SendGitHubConnected(chatID int64, username string) {
	text := fmt.Sprintf(
		"✅ *GitHub connected\\!*\n\n"+
			"Welcome, *%s*\\! 🎉\n\n"+
			"I'll monitor your daily contributions and remind you at *20:00* if you haven't contributed yet\\.\n\n"+
			"Send /settings to change your reminder time or timezone\\.",
		escMD(username),
	)
	b.replyMD(chatID, text)
}

// EncryptionKey exposes the key for use in OAuth callback handler.
func (b *Bot) EncryptionKey() string {
	return b.encryptionKey
}

// API exposes the underlying bot API for use in handlers.
func (b *Bot) API() *tgbotapi.BotAPI {
	return b.api
}

// escMD escapes a string for Telegram MarkdownV2.
func escMD(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}

func notifStatus(enabled bool) string {
	if enabled {
		return "enabled ✅"
	}
	return "disabled ❌"
}

func stripMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"*", "", "_", "", "\\.", ".", "\\!", "!", "\\-", "-",
	)
	return replacer.Replace(s)
}

// Ensure crypto import is used
var _ = crypto.Encrypt
var _ = time.Now
