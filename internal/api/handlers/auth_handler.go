package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"streak-guardian/internal/bot"
	"streak-guardian/internal/crypto"
	githubclient "streak-guardian/internal/github"
	"streak-guardian/internal/models"
	"streak-guardian/internal/repository"
)

// AuthHandler handles GitHub OAuth redirect and callback.
type AuthHandler struct {
	telegramBot   *bot.Bot
	oauthCfg      *githubclient.OAuthConfig
	githubClient  *githubclient.Client
	userRepo      repository.UserRepo
	encryptionKey string
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	telegramBot *bot.Bot,
	oauthCfg *githubclient.OAuthConfig,
	githubClient *githubclient.Client,
	userRepo repository.UserRepo,
	encryptionKey string,
) *AuthHandler {
	return &AuthHandler{
		telegramBot:   telegramBot,
		oauthCfg:      oauthCfg,
		githubClient:  githubClient,
		userRepo:      userRepo,
		encryptionKey: encryptionKey,
	}
}

// Redirect generates a GitHub OAuth URL and redirects the browser.
// GET /api/v1/auth/github?chat_id=<telegram_chat_id>
func (h *AuthHandler) Redirect(c *gin.Context) {
	// This endpoint is normally not needed — the bot generates the URL directly.
	// Included for completeness.
	c.JSON(http.StatusOK, gin.H{"message": "Use the link from the Telegram bot to connect GitHub."})
}

// Callback handles the GitHub OAuth callback.
// GET /api/v1/auth/github/callback?code=...&state=...
func (h *AuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
		return
	}

	// Validate state and extract the Telegram chat ID
	telegramChatID, err := h.oauthCfg.ValidateState(state)
	if err != nil {
		log.Printf("OAuth callback: invalid state: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired authorization link — please try /start again"})
		return
	}

	// Exchange code for access token
	token, err := h.oauthCfg.ExchangeCode(c.Request.Context(), code)
	if err != nil {
		log.Printf("OAuth callback: code exchange failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange code with GitHub"})
		return
	}

	// Fetch the authenticated GitHub user profile
	ghUser, err := h.githubClient.GetAuthenticatedUser(c.Request.Context(), token)
	if err != nil {
		log.Printf("OAuth callback: fetching GitHub user failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch GitHub profile"})
		return
	}

	// Encrypt the token before storing
	encryptedToken, err := crypto.Encrypt(token, h.encryptionKey)
	if err != nil {
		log.Printf("OAuth callback: encrypting token failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Upsert user in the database
	user := &models.User{
		GitHubID:            ghUser.ID,
		GitHubUsername:      ghUser.Login,
		GitHubAccessToken:   encryptedToken,
		TelegramChatID:      telegramChatID,
		Timezone:            "UTC",
		ReminderTime:        "20:00",
		NotificationEnabled: true,
	}

	if _, err := h.userRepo.Upsert(c.Request.Context(), user); err != nil {
		log.Printf("OAuth callback: upserting user failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save user"})
		return
	}

	// Notify the user on Telegram
	go h.telegramBot.SendGitHubConnected(telegramChatID, ghUser.Login)

	// Show a success page in the browser
	c.HTML(http.StatusOK, "success.html", gin.H{
		"username": ghUser.Login,
	})
}
