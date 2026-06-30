package database

import "time"

const (
	UserRoleAdmin = "admin"
	UserRoleUser  = "user"

	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
	UserStatusLocked   = "locked"

	LicenseStatusActive    = "active"
	LicenseStatusExpired   = "expired"
	LicenseStatusRevoked   = "revoked"
	LicenseStatusSuspended = "suspended"

	TransactionStatusPending   = "pending"
	TransactionStatusSucceeded = "succeeded"
	TransactionStatusFailed    = "failed"

	SessionStatusActive  = "active"
	SessionStatusExpired = "expired"
)

// User stores authentication, licensing, and device/session metadata.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash"`
	LicenseKey   string    `json:"licenseKey"`
	Credits      int       `json:"credits"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	LastLogin    time.Time `json:"lastLogin,omitempty"`
	LastIP       string    `json:"lastIP,omitempty"`
	DeviceID     string    `json:"deviceID,omitempty"`
	SessionID    string    `json:"sessionID,omitempty"`
}

// License stores a customer's entitlement and device binding.
type License struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	UserID    string    `json:"userID,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
	DeviceID  string    `json:"deviceID,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

// Transaction captures payment processing outcomes and reconciliation fields.
type Transaction struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userID"`
	CardLast4     string    `json:"cardLast4"`
	Amount        int64     `json:"amount"`
	Status        string    `json:"status"`
	LocCode       string    `json:"locCode,omitempty"`
	LocSubCode    string    `json:"locSubCode,omitempty"`
	FraudStatus   string    `json:"fraudStatus,omitempty"`
	RecordLocator string    `json:"recordLocator,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// Session stores API session state for a user/device pair.
type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userID"`
	Token        string    `json:"token"`
	DeviceID     string    `json:"deviceID,omitempty"`
	IPAddress    string    `json:"ipAddress,omitempty"`
	Status       string    `json:"status,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	LastActivity time.Time `json:"lastActivity"`
}

// Credit stores a user's current credit balance.
type Credit struct {
	UserID      string    `json:"userID"`
	Credits     int       `json:"credits"`
	LastUpdated time.Time `json:"lastUpdated"`
}

// AuditLog stores structured operational events.
type AuditLog struct {
	ID        string            `json:"id"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Context   map[string]string `json:"context,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

// ProxyRecord stores proxy metadata and health information.
type ProxyRecord struct {
	ID           string    `json:"id"`
	Address      string    `json:"address"`
	Enabled      bool      `json:"enabled"`
	FailCount    int       `json:"failCount"`
	LastFailedAt time.Time `json:"lastFailedAt,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
