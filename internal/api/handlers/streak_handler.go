package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"streak-guardian/internal/models"
	"streak-guardian/internal/repository"
	"streak-guardian/internal/service"
)

// StreakHandler serves streak and status REST endpoints.
type StreakHandler struct {
	userRepo  repository.UserRepo
	streakSvc *service.StreakService
}

// NewStreakHandler creates a new StreakHandler.
func NewStreakHandler(userRepo repository.UserRepo, streakSvc *service.StreakService) *StreakHandler {
	return &StreakHandler{userRepo: userRepo, streakSvc: streakSvc}
}

// GetStreak returns streak info for a user identified by telegram_chat_id query param.
// GET /api/v1/streak?chat_id=<id>
func (h *StreakHandler) GetStreak(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	info, err := h.streakSvc.GetStreakInfo(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute streak"})
		return
	}
	c.JSON(http.StatusOK, info)
}

// GetTodayStatus returns today's contribution status for a user.
// GET /api/v1/status/today?chat_id=<id>
func (h *StreakHandler) GetTodayStatus(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	info, err := h.streakSvc.CheckAndUpdateStreak(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check today's status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"today_contributed":  info.TodayContributed,
		"today_count":        info.TodayCount,
		"current_streak":     info.CurrentStreak,
		"longest_streak":     info.LongestStreak,
		"contributions_year": info.ContributionsYear,
	})
}

// resolveUser extracts the user from a chat_id query parameter.
func (h *StreakHandler) resolveUser(c *gin.Context) (*models.User, bool) {
	chatIDStr := c.Query("chat_id")
	if chatIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_id is required"})
		return nil, false
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return nil, false
	}
	user, err := h.userRepo.GetByTelegramChatID(c.Request.Context(), chatID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return nil, false
	}
	return user, true
}
