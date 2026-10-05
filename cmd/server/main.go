package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"streak-guardian/internal/api"
	"streak-guardian/internal/bot"
	"streak-guardian/internal/config"
	"streak-guardian/internal/database"
	githubclient "streak-guardian/internal/github"
	"streak-guardian/internal/repository"
	"streak-guardian/internal/scheduler"
	"streak-guardian/internal/service"
)

func main() {
	// ── 1. Load configuration ─────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Configuration error: %v", err)
	}
	log.Printf("✅ Configuration loaded (env: %s)", cfg.AppEnv)

	// ── 2. Connect to PostgreSQL ───────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("❌ Database connection failed: %v", err)
	}
	defer db.Close()
	log.Println("✅ Database connected")

	// ── 3. Run migrations ─────────────────────────────────────────────────────
	migrationsDir := migrationsPath()
	if err := db.RunMigrations(context.Background(), migrationsDir); err != nil {
		log.Fatalf("❌ Migrations failed: %v", err)
	}
	log.Println("✅ Migrations complete")

	// ── 4. Initialize repositories ────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db.Pool)
	contribRepo := repository.NewContributionRepository(db.Pool)
	notifRepo := repository.NewNotificationRepository(db.Pool)

	// ── 5. Initialize GitHub client and OAuth config ───────────────────────────
	ghClient := githubclient.New()
	oauthCfg := &githubclient.OAuthConfig{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		CallbackURL:  cfg.GitHubCallbackURL,
		JWTSecret:    cfg.JWTSecret,
	}

	// ── 6. Initialize Telegram bot ────────────────────────────────────────────
	telegramAPI, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("❌ Telegram bot initialization failed: %v", err)
	}
	telegramAPI.Debug = cfg.IsDevelopment()
	log.Printf("✅ Telegram bot authorized as @%s", telegramAPI.Self.UserName)

	// ── 7. Initialize services ────────────────────────────────────────────────
	notifSvc := service.NewNotificationService(telegramAPI, notifRepo)
	streakSvc := service.NewStreakService(userRepo, contribRepo, ghClient, cfg.EncryptionKey)

	// ── 8. Initialize Telegram bot handler ────────────────────────────────────
	telegramBot := bot.New(
		telegramAPI,
		userRepo,
		streakSvc,
		notifSvc,
		oauthCfg,
		cfg.EncryptionKey,
	)

	// ── 9. Initialize scheduler ───────────────────────────────────────────────
	sched := scheduler.New(userRepo, streakSvc, notifSvc)
	sched.Start()
	defer sched.Stop()

	// ── 10. Setup HTTP router ─────────────────────────────────────────────────
	router := api.NewRouter(telegramBot, oauthCfg, ghClient, userRepo, streakSvc, cfg.EncryptionKey)
	router.LoadHTMLGlob("templates/*")

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.ServerPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── 11. Start everything ──────────────────────────────────────────────────
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Telegram bot (long polling) in background
	go telegramBot.Start(appCtx)

	// HTTP server in background
	go func() {
		log.Printf("🚀 HTTP server listening on :%s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	log.Println("✅ Streak Guardian is running. Press Ctrl+C to stop.")

	// ── 12. Wait for shutdown signal ──────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("⏳ Shutting down gracefully...")
	appCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server forced shutdown: %v", err)
	}

	log.Println("👋 Streak Guardian stopped.")
}

// migrationsPath returns the absolute path to the migrations directory,
// handling both local development and compiled binary scenarios.
func migrationsPath() string {
	// Try relative to the binary first
	exe, err := os.Executable()
	if err == nil {
		rel := filepath.Join(filepath.Dir(exe), "migrations")
		if _, err := os.Stat(rel); err == nil {
			return rel
		}
	}

	// Fall back to relative to the source file (development)
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	}

	return "migrations"
}
