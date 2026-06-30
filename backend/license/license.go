package license

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/logger"
)

// ValidationResult describes the outcome of a license validation request.
type ValidationResult struct {
	Valid       bool              `json:"valid"`
	Status      string            `json:"status"`
	Indicator   string            `json:"indicator"`
	Reason      string            `json:"reason,omitempty"`
	Source      string            `json:"source"`
	CheckedAt   time.Time         `json:"checkedAt"`
	CachedUntil time.Time         `json:"cachedUntil,omitempty"`
	License     *database.License `json:"license,omitempty"`
}

type cachedValidation struct {
	result    ValidationResult
	expiresAt time.Time
}

// Service validates, binds, and revokes licenses with optional online checks.
type Service struct {
	cfg      *config.Config
	logger   *logger.Logger
	repo     *database.Repository[database.License]
	client   *http.Client
	cacheTTL time.Duration
	mu       sync.RWMutex
	cache    map[string]cachedValidation
}

// NewService creates a license service using the JSON database.
func NewService(cfg *config.Config, db *database.JSONDatabase) (*Service, error) {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	if db == nil {
		var err error
		db, err = database.NewJSONDatabase(cfg)
		if err != nil {
			return nil, err
		}
	}
	repo, err := db.Licenses()
	if err != nil {
		return nil, err
	}
	cacheTTL := time.Duration(cfg.License.OfflineCacheMins) * time.Minute
	if cacheTTL <= 0 {
		cacheTTL = time.Hour
	}
	return &Service{
		cfg:      cfg,
		logger:   logger.GetLogger(),
		repo:     repo,
		client:   &http.Client{Timeout: 15 * time.Second},
		cacheTTL: cacheTTL,
		cache:    make(map[string]cachedValidation),
	}, nil
}

// ValidateLicense validates a license locally and optionally online with offline cache fallback.
func (s *Service) ValidateLicense(ctx context.Context, key, userID, deviceID string) (ValidationResult, error) {
	cacheKey := s.cacheKey(key, deviceID)
	if cached, ok := s.getCache(cacheKey); ok {
		cached.Source = "offline-cache"
		return cached, nil
	}

	if strings.TrimSpace(s.cfg.License.ValidationURL) != "" {
		onlineResult, err := s.validateOnline(ctx, key, userID, deviceID)
		if err == nil {
			s.storeCache(cacheKey, onlineResult)
			return onlineResult, nil
		}
		s.logger.Warn("Online license validation failed, falling back to local validation", map[string]string{
			"license_key": key,
			"error":       err.Error(),
		})
	}

	result, err := s.validateLocal(ctx, key, userID, deviceID)
	if err != nil {
		return result, err
	}
	s.storeCache(cacheKey, result)
	return result, nil
}

// CheckRevocation returns whether a license has been revoked or suspended.
func (s *Service) CheckRevocation(ctx context.Context, key string) (bool, error) {
	license, found, err := s.repo.FindOne(ctx, func(item database.License) bool { return item.Key == key })
	if err != nil {
		return false, err
	}
	if !found {
		return false, database.ErrRecordNotFound
	}
	return license.Status == database.LicenseStatusRevoked || license.Status == database.LicenseStatusSuspended, nil
}

// BindDevice assigns a device identifier to a license when not already bound.
func (s *Service) BindDevice(ctx context.Context, key, deviceID string) error {
	if strings.TrimSpace(deviceID) == "" {
		return errors.New("device ID is required")
	}
	license, found, err := s.repo.FindOne(ctx, func(item database.License) bool { return item.Key == key })
	if err != nil {
		return err
	}
	if !found {
		return database.ErrRecordNotFound
	}
	if license.DeviceID != "" && license.DeviceID != deviceID {
		return errors.New("license is already bound to another device")
	}
	_, err = s.repo.Update(ctx, license.ID, func(item *database.License) error {
		item.DeviceID = deviceID
		return nil
	})
	return err
}

// Revoke marks a license as revoked and invalidates any cached validations.
func (s *Service) Revoke(ctx context.Context, key string) error {
	license, found, err := s.repo.FindOne(ctx, func(item database.License) bool { return item.Key == key })
	if err != nil {
		return err
	}
	if !found {
		return database.ErrRecordNotFound
	}
	if _, err := s.repo.Update(ctx, license.ID, func(item *database.License) error {
		item.Status = database.LicenseStatusRevoked
		return nil
	}); err != nil {
		return err
	}
	s.mu.Lock()
	for cacheKey := range s.cache {
		if strings.HasPrefix(cacheKey, key+"|") {
			delete(s.cache, cacheKey)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) validateLocal(ctx context.Context, key, userID, deviceID string) (ValidationResult, error) {
	license, found, err := s.repo.FindOne(ctx, func(item database.License) bool { return item.Key == key })
	if err != nil {
		return ValidationResult{}, err
	}
	if !found {
		return s.invalidResult("not-found", "license key was not found", "local"), nil
	}
	if license.UserID != "" && userID != "" && license.UserID != userID {
		return s.invalidResult("mismatch", "license does not belong to this user", "local"), nil
	}
	if license.Status == database.LicenseStatusRevoked || license.Status == database.LicenseStatusSuspended {
		return s.invalidResult(license.Status, "license is not active", "local"), nil
	}
	if !license.ExpiresAt.IsZero() && time.Now().UTC().After(license.ExpiresAt) {
		return s.invalidResult(database.LicenseStatusExpired, "license has expired", "local"), nil
	}
	if license.DeviceID != "" && deviceID != "" && license.DeviceID != deviceID {
		return s.invalidResult("device-mismatch", "license is bound to another device", "local"), nil
	}
	if license.DeviceID == "" && deviceID != "" {
		if _, err := s.repo.Update(ctx, license.ID, func(item *database.License) error {
			item.DeviceID = deviceID
			return nil
		}); err != nil {
			return ValidationResult{}, err
		}
		license.DeviceID = deviceID
	}

	result := ValidationResult{
		Valid:       true,
		Status:      database.LicenseStatusActive,
		Indicator:   "green",
		Source:      "local",
		CheckedAt:   time.Now().UTC(),
		CachedUntil: time.Now().UTC().Add(s.cacheTTL),
		License:     &license,
	}
	return result, nil
}

func (s *Service) validateOnline(ctx context.Context, key, userID, deviceID string) (ValidationResult, error) {
	payload, err := json.Marshal(map[string]string{
		"key":      key,
		"userID":   userID,
		"deviceID": deviceID,
	})
	if err != nil {
		return ValidationResult{}, fmt.Errorf("marshal online validation payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.License.ValidationURL, bytes.NewReader(payload))
	if err != nil {
		return ValidationResult{}, fmt.Errorf("build online validation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("perform online validation request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return ValidationResult{}, fmt.Errorf("online validation returned status %d", resp.StatusCode)
	}
	var body struct {
		Valid     bool      `json:"valid"`
		Status    string    `json:"status"`
		Reason    string    `json:"reason"`
		Revoked   bool      `json:"revoked"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ValidationResult{}, fmt.Errorf("decode online validation response: %w", err)
	}
	status := body.Status
	if status == "" {
		status = database.LicenseStatusActive
		if !body.Valid {
			status = "invalid"
		}
	}
	if body.Revoked {
		status = database.LicenseStatusRevoked
	}
	if !body.ExpiresAt.IsZero() && time.Now().UTC().After(body.ExpiresAt) {
		status = database.LicenseStatusExpired
		body.Valid = false
	}
	result := ValidationResult{
		Valid:       body.Valid && status == database.LicenseStatusActive,
		Status:      status,
		Indicator:   indicatorForStatus(status),
		Reason:      body.Reason,
		Source:      "online",
		CheckedAt:   time.Now().UTC(),
		CachedUntil: time.Now().UTC().Add(s.cacheTTL),
	}
	return result, nil
}

func (s *Service) invalidResult(status, reason, source string) ValidationResult {
	return ValidationResult{
		Valid:       false,
		Status:      status,
		Indicator:   indicatorForStatus(status),
		Reason:      reason,
		Source:      source,
		CheckedAt:   time.Now().UTC(),
		CachedUntil: time.Now().UTC().Add(s.cacheTTL),
	}
}

func (s *Service) cacheKey(key, deviceID string) string {
	return key + "|" + deviceID
}

func (s *Service) getCache(key string) (ValidationResult, bool) {
	now := time.Now().UTC()
	s.mu.RLock()
	cached, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok || !now.Before(cached.expiresAt) {
		if ok {
			s.mu.Lock()
			delete(s.cache, key)
			s.mu.Unlock()
		}
		return ValidationResult{}, false
	}
	return cached.result, true
}

func (s *Service) storeCache(key string, result ValidationResult) {
	s.mu.Lock()
	s.cache[key] = cachedValidation{result: result, expiresAt: result.CachedUntil}
	s.mu.Unlock()
}

func indicatorForStatus(status string) string {
	switch status {
	case database.LicenseStatusActive:
		return "green"
	case database.LicenseStatusExpired, database.LicenseStatusSuspended:
		return "yellow"
	default:
		return "red"
	}
}
