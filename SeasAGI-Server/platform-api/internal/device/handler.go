package device

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
)

type Device struct {
	DeviceID   string `json:"device_id"`
	UserID     string `json:"user_id"`
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
	BoundAt    string `json:"bound_at"`
}

type BindRequest struct {
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
}

func BindDevice(c *gin.Context) {
	userID := c.GetString("user_id")

	var req BindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	deviceID, err := createDeviceBinding(userID, req.DeviceName, req.Platform)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"device_id": deviceID,
		"message":   "device bound successfully",
	})
}

func ListDevices(c *gin.Context) {
	userID := c.GetString("user_id")

	devices, err := fetchDevicesForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": devices})
}

func RemoveDevice(c *gin.Context) {
	deviceID := c.Param("id")
	userID := c.GetString("user_id")

	if err := deleteDeviceBinding(userID, deviceID, c); err != nil {
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "device removed"})
}

func createDeviceBinding(userID, deviceName, platform string) (string, error) {
	deviceID := fmt.Sprintf("dev_%d", time.Now().UnixNano())
	boundAt := time.Now().UTC().Format(time.RFC3339)

	_, err := database.DB.Exec(
		`INSERT INTO devices (device_id, user_id, device_name, platform, bound_at)
		 VALUES (?, ?, ?, ?, ?)`,
		deviceID, userID, deviceName, platform, boundAt,
	)
	if err != nil {
		return "", fmt.Errorf("failed to bind device: %w", err)
	}

	return deviceID, nil
}

func fetchDevicesForUser(userID string) ([]Device, error) {
	rows, err := database.DB.Query(
		`SELECT device_id, user_id, device_name, platform, bound_at
		 FROM devices WHERE user_id = ? ORDER BY bound_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.DeviceID, &d.UserID, &d.DeviceName, &d.Platform, &d.BoundAt); err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}
		devices = append(devices, d)
	}

	if devices == nil {
		devices = []Device{}
	}

	return devices, nil
}

func deleteDeviceBinding(userID, deviceID string, c *gin.Context) error {
	result, err := database.DB.Exec(
		`DELETE FROM devices WHERE device_id = ? AND user_id = ?`,
		deviceID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "device.notFound")})
		return fmt.Errorf("device not found")
	}

	return nil
}
