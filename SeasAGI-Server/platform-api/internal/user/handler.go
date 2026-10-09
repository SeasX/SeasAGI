package user

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
)

type UserProfile struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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

func fetchProfile(userID string) (*UserProfile, error) {
	var profile UserProfile
	var createdAt, updatedAt string

	err := database.DB.QueryRow(
		`SELECT user_id, created_at, updated_at FROM users WHERE user_id = ?`,
		userID,
	).Scan(&profile.UserID, &createdAt, &updatedAt)
	if err != nil {
		profile.UserID = userID
		return &profile, nil
	}

	profile.CreatedAt = createdAt
	profile.UpdatedAt = updatedAt
	return &profile, nil
}
