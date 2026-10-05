package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"streak-guardian/internal/api/handlers"
	"streak-guardian/internal/bot"
	githubclient "streak-guardian/internal/github"
	"streak-guardian/internal/repository"
	"streak-guardian/internal/service"
)

// NewRouter creates and returns the configured Gin router.
func NewRouter(
	telegramBot *bot.Bot,
	oauthCfg *githubclient.OAuthConfig,
	githubClient *githubclient.Client,
	userRepo repository.UserRepo,
	streakSvc *service.StreakService,
	encryptionKey string,
) *gin.Engine {
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// GitHub OAuth routes
	authHandler := handlers.NewAuthHandler(telegramBot, oauthCfg, githubClient, userRepo, encryptionKey)
	r.GET("/api/v1/auth/github", authHandler.Redirect)
	r.GET("/api/v1/auth/github/callback", authHandler.Callback)

	// Streak / status routes (simple GET, no session needed for MVP)
	streakHandler := handlers.NewStreakHandler(userRepo, streakSvc)
	r.GET("/api/v1/streak", streakHandler.GetStreak)
	r.GET("/api/v1/status/today", streakHandler.GetTodayStatus)

	return r
}
