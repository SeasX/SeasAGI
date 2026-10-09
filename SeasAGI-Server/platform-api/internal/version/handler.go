package version

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
)

func Check(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":        "0.2.0",
		"minimum_version": "0.1.0",
		"force_upgrade":  false,
	})
}

func Migrations(c *gin.Context) {
	items, err := database.ListMigrations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}
