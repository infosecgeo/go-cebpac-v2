package database

import (
	"context"
	"sync"
)

// SettingsCache provides cached access to admin settings
type SettingsCache struct {
	mu       sync.RWMutex
	settings *AdminSettings
	loaded   bool
}

var (
	settingsCache     = &SettingsCache{}
	settingsCacheMu   sync.Mutex
)

// GetAdminSettings returns the current admin settings with caching
func GetAdminSettings(ctx context.Context) (*AdminSettings, error) {
	settingsCache.mu.RLock()
	if settingsCache.loaded && settingsCache.settings != nil {
		defer settingsCache.mu.RUnlock()
		return settingsCache.settings, nil
	}
	settingsCache.mu.RUnlock()

	// Load from database
	settingsCache.mu.Lock()
	defer settingsCache.mu.Unlock()

	// Double-check after acquiring write lock
	if settingsCache.loaded && settingsCache.settings != nil {
		return settingsCache.settings, nil
	}

	db := GetDatabase()
	settingsRepo, err := db.AdminSettings()
	if err != nil {
		return nil, err
	}

	settings, err := settingsRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	if len(settings) > 0 {
		settingsCache.settings = &settings[0]
	} else {
		// Return default settings if none exist
		settingsCache.settings = &AdminSettings{
			ID:                  "default",
			ProxyURL:            "",
			APIKey:              "b260f3c7-23ea-422c-bcd4-a0b57a11f8a9", // Default API key
			PaymentInstructions: "Please scan the QR code and send payment proof.",
			MinTopupAmount:      1,
		}
	}

	settingsCache.loaded = true
	return settingsCache.settings, nil
}

// InvalidateSettingsCache forces a reload of settings on next access
func InvalidateSettingsCache() {
	settingsCache.mu.Lock()
	defer settingsCache.mu.Unlock()
	settingsCache.loaded = false
	settingsCache.settings = nil
}

// GetAPIKey returns the configured API key or default
func GetAPIKey(ctx context.Context) string {
	settings, err := GetAdminSettings(ctx)
	if err != nil || settings == nil || settings.APIKey == "" {
		return "b260f3c7-23ea-422c-bcd4-a0b57a11f8a9" // Default fallback
	}
	return settings.APIKey
}

// GetProxyURL returns the configured proxy URL or empty string
func GetProxyURL(ctx context.Context) string {
	settings, err := GetAdminSettings(ctx)
	if err != nil || settings == nil {
		return ""
	}
	return settings.ProxyURL
}
