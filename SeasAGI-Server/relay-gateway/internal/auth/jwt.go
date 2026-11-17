package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	DeviceID  string `json:"device_id"`
	TenantID  string `json:"tenant_id"`
	jwt.RegisteredClaims
}

func ValidateToken(tokenString string) (*Claims, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// GenerateToken 签发网关侧令牌。tenantID 为可选参数（变长以兼容既有两参调用），
// 生产环境的租户身份应由 platform-api 签发并写入签名声明，不由客户端提供。
func GenerateToken(userID, channelID string, tenantID ...string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	var tenant string
	if len(tenantID) > 0 {
		tenant = tenantID[0]
	}
	claims := Claims{
		UserID:    userID,
		ChannelID: channelID,
		TenantID:  tenant,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
