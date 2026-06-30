package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"cebupac/backend/config"
	"golang.org/x/crypto/bcrypt"
)

// PasswordManager centralizes password validation and hashing policy.
type PasswordManager struct {
	cfg *config.Config
}

// NewPasswordManager creates a password manager using runtime configuration.
func NewPasswordManager(cfg *config.Config) *PasswordManager {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	return &PasswordManager{cfg: cfg}
}

// HashPassword validates and hashes a password with bcrypt.
func (m *PasswordManager) HashPassword(ctx context.Context, password string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := m.ValidatePasswordComplexity(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword verifies a plaintext password against a bcrypt hash.
func (m *PasswordManager) ComparePassword(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return errors.New("invalid username or password")
	}
	return nil
}

// ValidatePasswordComplexity enforces minimum production password requirements.
func (m *PasswordManager) ValidatePasswordComplexity(password string) error {
	minLength := m.cfg.Security.PasswordMinLength
	if minLength <= 0 {
		minLength = 8
	}
	if len(password) < minLength {
		return fmt.Errorf("password must be at least %d characters", minLength)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsSpace(r):
			return errors.New("password must not contain whitespace")
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	missing := make([]string, 0, 4)
	if !hasUpper {
		missing = append(missing, "uppercase")
	}
	if !hasLower {
		missing = append(missing, "lowercase")
	}
	if !hasDigit {
		missing = append(missing, "digit")
	}
	if !hasSpecial {
		missing = append(missing, "special character")
	}
	if len(missing) > 0 {
		return fmt.Errorf("password is missing: %s", strings.Join(missing, ", "))
	}
	return nil
}
