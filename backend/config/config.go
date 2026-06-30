package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type Config struct {
	Server struct {
		Port                string `json:"port"`
		Environment         string `json:"environment"`
		ShutdownTimeoutSecs int    `json:"shutdown_timeout_secs"`
	} `json:"server"`

	Security struct {
		JWTSecret           string `json:"jwt_secret"`
		JWTExpiryHours      int    `json:"jwt_expiry_hours"`
		RefreshTokenHours   int    `json:"refresh_token_hours"`
		PasswordMinLength   int    `json:"password_min_length"`
		MaxLoginAttempts    int    `json:"max_login_attempts"`
		LockoutDurationMins int    `json:"lockout_duration_mins"`
	} `json:"security"`

	RateLimit struct {
		Enabled        bool `json:"enabled"`
		RequestsPerMin int  `json:"requests_per_min"`
		BurstSize      int  `json:"burst_size"`
	} `json:"rate_limit"`

	Workers struct {
		PoolSize           int `json:"pool_size"`
		QueueSize          int `json:"queue_size"`
		MaxConcurrent      int `json:"max_concurrent"`
		TaskTimeoutSeconds int `json:"task_timeout_seconds"`
	} `json:"workers"`

	Retry struct {
		MaxAttempts       int `json:"max_attempts"`
		InitialDelayMs    int `json:"initial_delay_ms"`
		MaxDelayMs        int `json:"max_delay_ms"`
		BackoffMultiplier int `json:"backoff_multiplier"`
	} `json:"retry"`

	Proxy struct {
		Enabled       bool     `json:"enabled"`
		ProxyList     []string `json:"proxy_list"`
		RotationMode  string   `json:"rotation_mode"`
		TimeoutSecs   int      `json:"timeout_secs"`
	} `json:"proxy"`

	Akamai struct {
		APIKey    string `json:"api_key"`
		BaseURL   string `json:"base_url"`
		UserAgent string `json:"user_agent"`
		SecChUa   string `json:"sec_ch_ua"`
	} `json:"akamai"`

	CebupacificAir struct {
		BaseURL    string `json:"base_url"`
		SoarURL    string `json:"soar_url"`
		AcceptLang string `json:"accept_lang"`
	} `json:"cebupacific_air"`

	Database struct {
		StoragePath       string `json:"storage_path"`
		BackupEnabled     bool   `json:"backup_enabled"`
		BackupIntervalMin int    `json:"backup_interval_min"`
	} `json:"database"`

	Logging struct {
		Level          string `json:"level"`
		OutputFile     string `json:"output_file"`
		MaxSizeMB      int    `json:"max_size_mb"`
		MaxBackups     int    `json:"max_backups"`
		MaxAgeDays     int    `json:"max_age_days"`
		Compress       bool   `json:"compress"`
	} `json:"logging"`

	License struct {
		ValidationURL     string `json:"validation_url"`
		OfflineCacheMins  int    `json:"offline_cache_mins"`
		CheckIntervalMins int    `json:"check_interval_mins"`
	} `json:"license"`

	mu sync.RWMutex
}

var (
	instance *Config
	once     sync.Once
)

// GetConfig returns the singleton config instance
func GetConfig() *Config {
	once.Do(func() {
		instance = &Config{}
		if err := instance.Load(); err != nil {
			// Set defaults if config file doesn't exist
			instance.setDefaults()
		}
	})
	return instance
}

// Load reads config from storage/config.json
func (c *Config) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile("storage/config.json")
	if err != nil {
		if os.IsNotExist(err) {
			c.setDefaults()
			return c.save()
		}
		return fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	return nil
}

// Save writes config to storage/config.json
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.save()
}

func (c *Config) save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.MkdirAll("storage", 0755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	if err := os.WriteFile("storage/config.json", data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// setDefaults initializes config with default values
func (c *Config) setDefaults() {
	c.Server.Port = "5000"
	c.Server.Environment = "production"
	c.Server.ShutdownTimeoutSecs = 30

	c.Security.JWTSecret = generateSecret()
	c.Security.JWTExpiryHours = 24
	c.Security.RefreshTokenHours = 168 // 7 days
	c.Security.PasswordMinLength = 8
	c.Security.MaxLoginAttempts = 5
	c.Security.LockoutDurationMins = 15

	c.RateLimit.Enabled = true
	c.RateLimit.RequestsPerMin = 60
	c.RateLimit.BurstSize = 10

	c.Workers.PoolSize = 10
	c.Workers.QueueSize = 100
	c.Workers.MaxConcurrent = 5
	c.Workers.TaskTimeoutSeconds = 300

	c.Retry.MaxAttempts = 10
	c.Retry.InitialDelayMs = 1000
	c.Retry.MaxDelayMs = 30000
	c.Retry.BackoffMultiplier = 2

	c.Proxy.Enabled = false
	c.Proxy.ProxyList = []string{}
	c.Proxy.RotationMode = "round-robin"
	c.Proxy.TimeoutSecs = 30

	c.Akamai.APIKey = "b260f3c7-23ea-422c-bcd4-a0b57a11f8a9"
	c.Akamai.BaseURL = "https://www.cebupacificair.com"
	c.Akamai.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"
	c.Akamai.SecChUa = `"Google Chrome";v="137", "Chromium";v="137", "Not/A)Brand";v="24"`

	c.CebupacificAir.BaseURL = "https://www.cebupacificair.com"
	c.CebupacificAir.SoarURL = "https://soar.cebupacificair.com"
	c.CebupacificAir.AcceptLang = "en-US,en;q=0.9"

	c.Database.StoragePath = "storage"
	c.Database.BackupEnabled = true
	c.Database.BackupIntervalMin = 60

	c.Logging.Level = "info"
	c.Logging.OutputFile = "storage/logs/app.log"
	c.Logging.MaxSizeMB = 100
	c.Logging.MaxBackups = 10
	c.Logging.MaxAgeDays = 30
	c.Logging.Compress = true

	c.License.ValidationURL = ""
	c.License.OfflineCacheMins = 60
	c.License.CheckIntervalMins = 30
}

// GetServerPort returns server port thread-safe
func (c *Config) GetServerPort() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Server.Port
}

// GetJWTSecret returns JWT secret thread-safe
func (c *Config) GetJWTSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Security.JWTSecret
}

// UpdateRuntimeConfig updates specific config values at runtime
func (c *Config) UpdateRuntimeConfig(updates map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Apply updates based on key paths
	for key, value := range updates {
		switch key {
		case "workers.pool_size":
			if v, ok := value.(float64); ok {
				c.Workers.PoolSize = int(v)
			}
		case "retry.max_attempts":
			if v, ok := value.(float64); ok {
				c.Retry.MaxAttempts = int(v)
			}
		case "rate_limit.enabled":
			if v, ok := value.(bool); ok {
				c.RateLimit.Enabled = v
			}
		// Add more runtime-updateable fields as needed
		}
	}

	return c.save()
}

func generateSecret() string {
	// In production, this should be loaded from environment or secure vault
	return "CHANGE_THIS_SECRET_IN_PRODUCTION_" + fmt.Sprintf("%d", os.Getpid())
}
