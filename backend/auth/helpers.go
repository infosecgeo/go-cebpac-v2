package auth

import (
	"cebupac/backend/config"
	"cebupac/backend/database"
	"context"
	"sync"
)

var (
	jwtManagerInstance      *JWTManager
	jwtOnce                 sync.Once
	passwordManagerInstance *PasswordManager
	passwordOnce            sync.Once
)

// GetJWTManager returns the singleton JWT manager instance
func GetJWTManager() *JWTManager {
	jwtOnce.Do(func() {
		jwtManagerInstance = NewJWTManager(config.GetConfig())
	})
	return jwtManagerInstance
}

// GetPasswordManager returns the singleton password manager instance
func GetPasswordManager() *PasswordManager {
	passwordOnce.Do(func() {
		passwordManagerInstance = NewPasswordManager(config.GetConfig())
	})
	return passwordManagerInstance
}

// GenerateToken generates an access token for a user (simplified)
func GenerateToken(userID, username, role string) (string, error) {
	manager := GetJWTManager()
	user := database.User{
		ID:       userID,
		Username: username,
		Role:     role,
	}
	pair, err := manager.GenerateTokenPair(user)
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

// GenerateRefreshToken generates a refresh token for a user (simplified)
func GenerateRefreshToken(userID string) (string, error) {
	manager := GetJWTManager()
	user := database.User{
		ID: userID,
	}
	pair, err := manager.GenerateTokenPair(user)
	if err != nil {
		return "", err
	}
	return pair.RefreshToken, nil
}

// ValidateToken validates a token and returns claims (simplified)
func ValidateToken(tokenString string) (*Claims, error) {
	manager := GetJWTManager()
	return manager.ValidateToken(tokenString, "access")
}

// ValidateRefreshToken validates a refresh token and returns claims
func ValidateRefreshToken(tokenString string) (*Claims, error) {
	manager := GetJWTManager()
	return manager.ValidateToken(tokenString, "refresh")
}

// RefreshAccessToken refreshes an access token using a refresh token
func RefreshAccessToken(refreshToken string) (string, error) {
	manager := GetJWTManager()
	pair, err := manager.RefreshTokens(refreshToken)
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

// HashPassword hashes a password
func HashPassword(password string) (string, error) {
	manager := GetPasswordManager()
	return manager.HashPassword(context.Background(), password)
}

// VerifyPassword verifies a password against a hash
func VerifyPassword(password, hash string) error {
	manager := GetPasswordManager()
	return manager.ComparePassword(password, hash)
}

// ValidatePassword validates password complexity
func ValidatePassword(password string) error {
	manager := GetPasswordManager()
	return manager.ValidatePasswordComplexity(password)
}
