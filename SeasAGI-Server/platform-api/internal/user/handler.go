package user

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
)

type UserProfile struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Plan      string `json:"plan"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type UpdateProfileRequest struct {
	Plan *string `json:"plan"`
}

func GetProfile(c *gin.Context) {
	userID := c.GetString("user_id")

	profile, err := fetchProfile(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	email := c.GetString("email")
	profile.Email = email

	c.JSON(http.StatusOK, profile)
}

func UpdateProfile(c *gin.Context) {
	userID := c.GetString("user_id")

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Plan != nil {
		if err := updatePlan(userID, *req.Plan); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	profile, _ := fetchProfile(userID)
	email := c.GetString("email")
	profile.Email = email

	c.JSON(http.StatusOK, profile)
}

func fetchProfile(userID string) (*UserProfile, error) {
	var profile UserProfile
	var createdAt, updatedAt, plan string

	err := database.DB.QueryRow(
		`SELECT user_id, plan, created_at, updated_at FROM users WHERE user_id = ?`,
		userID,
	).Scan(&profile.UserID, &plan, &createdAt, &updatedAt)
	if err != nil {
		profile.UserID = userID
		profile.Plan = "free"
		return &profile, nil
	}

	activePlan, _ := fetchActiveSubscriptionPlan(userID)
	if activePlan != "" {
		plan = activePlan
	}

	profile.Plan = plan
	profile.CreatedAt = createdAt
	profile.UpdatedAt = updatedAt
	return &profile, nil
}

func fetchActiveSubscriptionPlan(userID string) (string, error) {
	var plan string
	err := database.DB.QueryRow(
		`SELECT plan FROM subscriptions WHERE user_id = ? AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP) ORDER BY created_at DESC LIMIT 1`,
		userID,
	).Scan(&plan)
	if err != nil {
		return "", err
	}
	return plan, nil
}

func updatePlan(userID string, plan string) error {
	_, err := database.DB.Exec(
		`UPDATE users SET plan = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`,
		plan, userID,
	)
	return err
}
