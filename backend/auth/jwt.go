package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/logger"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrBlacklistedToken = errors.New("token is blacklisted")
	ErrWrongTokenType   = errors.New("unexpected token type")
)

// Claims contains authenticated user context embedded in JWTs.
type Claims struct {
	UserID    string `json:"userID"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	SessionID string `json:"sessionID,omitempty"`
	DeviceID  string `json:"deviceID,omitempty"`
	TokenType string `json:"tokenType"`
	jwt.RegisteredClaims
}

// TokenPair groups access and refresh tokens for API clients.
type TokenPair struct {
	AccessToken   string    `json:"accessToken"`
	RefreshToken  string    `json:"refreshToken"`
	AccessExpiry  time.Time `json:"accessExpiry"`
	RefreshExpiry time.Time `json:"refreshExpiry"`
}

// JWTManager handles token issuance, validation, refresh, and blacklisting.
type JWTManager struct {
	cfg        *config.Config
	logger     *logger.Logger
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	mu         sync.RWMutex
	blacklist  map[string]time.Time
}

// NewJWTManager constructs a JWT manager from application configuration.
func NewJWTManager(cfg *config.Config) *JWTManager {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	return &JWTManager{
		cfg:        cfg,
		logger:     logger.GetLogger(),
		secret:     []byte(cfg.GetJWTSecret()),
		accessTTL:  time.Duration(cfg.Security.JWTExpiryHours) * time.Hour,
		refreshTTL: time.Duration(cfg.Security.RefreshTokenHours) * time.Hour,
		blacklist:  make(map[string]time.Time),
	}
}

// GenerateTokenPair issues access and refresh tokens for a user.
func (m *JWTManager) GenerateTokenPair(user database.User) (TokenPair, error) {
	return m.generatePair(user.ID, user.Username, user.Role, user.SessionID, user.DeviceID)
}

// GenerateTokenPairFromClaims issues a fresh pair from validated token claims.
func (m *JWTManager) GenerateTokenPairFromClaims(claims *Claims) (TokenPair, error) {
	if claims == nil {
		return TokenPair{}, errors.New("claims are required")
	}
	return m.generatePair(claims.UserID, claims.Username, claims.Role, claims.SessionID, claims.DeviceID)
}

func (m *JWTManager) generatePair(userID, username, role, sessionID, deviceID string) (TokenPair, error) {
	now := time.Now().UTC()
	accessExpiry := now.Add(m.accessTTL)
	refreshExpiry := now.Add(m.refreshTTL)

	accessClaims := &Claims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		SessionID: sessionID,
		DeviceID:  deviceID,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   userID,
			ID:        randomTokenID(),
		},
	}
	refreshClaims := &Claims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		SessionID: sessionID,
		DeviceID:  deviceID,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   userID,
			ID:        randomTokenID(),
		},
	}

	accessToken, err := m.sign(accessClaims)
	if err != nil {
		return TokenPair{}, err
	}
	refreshToken, err := m.sign(refreshClaims)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		AccessExpiry:  accessExpiry,
		RefreshExpiry: refreshExpiry,
	}, nil
}

// ValidateToken parses and validates a token, optionally enforcing token type.
func (m *JWTManager) ValidateToken(tokenString string, expectedTypes ...string) (*Claims, error) {
	m.cleanupBlacklist()
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if m.IsBlacklisted(claims.ID) {
		return nil, ErrBlacklistedToken
	}
	if len(expectedTypes) > 0 {
		matched := false
		for _, tokenType := range expectedTypes {
			if claims.TokenType == tokenType {
				matched = true
				break
			}
		}
		if !matched {
			return nil, ErrWrongTokenType
		}
	}
	return claims, nil
}

// RefreshTokens validates a refresh token, blacklists it, and issues a new pair.
func (m *JWTManager) RefreshTokens(refreshToken string) (TokenPair, error) {
	claims, err := m.ValidateToken(refreshToken, "refresh")
	if err != nil {
		return TokenPair{}, err
	}
	if err := m.BlacklistToken(refreshToken); err != nil {
		return TokenPair{}, err
	}
	return m.GenerateTokenPairFromClaims(claims)
}

// BlacklistToken invalidates a token until its expiry time.
func (m *JWTManager) BlacklistToken(tokenString string) error {
	claims := &Claims{}
	_, _, err := new(jwt.Parser).ParseUnverified(tokenString, claims)
	if err != nil {
		return fmt.Errorf("parse token for blacklist: %w", err)
	}
	expiresAt := time.Now().UTC().Add(m.refreshTTL)
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	m.mu.Lock()
	m.blacklist[claims.ID] = expiresAt
	m.mu.Unlock()
	return nil
}

// IsBlacklisted reports whether the token identifier is currently blocked.
func (m *JWTManager) IsBlacklisted(tokenID string) bool {
	m.mu.RLock()
	expiresAt, ok := m.blacklist[tokenID]
	m.mu.RUnlock()
	return ok && time.Now().UTC().Before(expiresAt)
}

func (m *JWTManager) cleanupBlacklist() {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for tokenID, expiresAt := range m.blacklist {
		if !now.Before(expiresAt) {
			delete(m.blacklist, tokenID)
		}
	}
}

func (m *JWTManager) sign(claims *Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func randomTokenID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
