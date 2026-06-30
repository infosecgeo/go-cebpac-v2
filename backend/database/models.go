package database

import "time"

const (
	UserRoleAdmin = "admin"
	UserRoleUser  = "user"

	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
	UserStatusLocked   = "locked"
	UserStatusPending  = "pending"

	RegistrationStatusPending  = "pending"
	RegistrationStatusApproved = "approved"
	RegistrationStatusRejected = "rejected"

	LicenseStatusActive    = "active"
	LicenseStatusExpired   = "expired"
	LicenseStatusRevoked   = "revoked"
	LicenseStatusSuspended = "suspended"

	TransactionStatusPending   = "pending"
	TransactionStatusSucceeded = "succeeded"
	TransactionStatusFailed    = "failed"

	SessionStatusActive  = "active"
	SessionStatusExpired = "expired"

	TopupStatusPending  = "pending"
	TopupStatusApproved = "approved"
	TopupStatusDenied   = "denied"
	TopupStatusExpired  = "expired"

	ProcessingStatusIdle       = "idle"
	ProcessingStatusActive     = "active"
	ProcessingStatusTerminated = "terminated"
)

// User stores authentication, licensing, and device/session metadata.
type User struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"passwordHash"`
	LicenseKey         string    `json:"licenseKey"`
	Credits            int       `json:"credits"`
	Role               string    `json:"role"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"createdAt"`
	LastLogin          time.Time `json:"lastLogin,omitempty"`
	LastIP             string    `json:"lastIP,omitempty"`
	DeviceID           string    `json:"deviceID,omitempty"`
	SessionID          string    `json:"sessionID,omitempty"`
	TelegramID         int64     `json:"telegramID,omitempty"`
	TelegramUsername   string    `json:"telegramUsername,omitempty"`
	TermsAccepted      bool      `json:"termsAccepted"`
	RegistrationStatus string    `json:"registrationStatus,omitempty"`
	Email              string    `json:"email,omitempty"`
}

// License stores a customer's entitlement and device binding.
type License struct {
	ID                string    `json:"id"`
	Key               string    `json:"key"`
	UserID            string    `json:"userID,omitempty"`
	ExpiresAt         time.Time `json:"expiresAt"`
	DeviceID          string    `json:"deviceID,omitempty"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"createdAt"`
	LinkedTelegramID  int64     `json:"linkedTelegramID,omitempty"`
	MaxDevices        int       `json:"maxDevices"`
	CurrentDeviceUsed bool      `json:"currentDeviceUsed"`
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
	ID               string    `json:"id"`
	UserID           string    `json:"userID"`
	Token            string    `json:"token"`
	DeviceID         string    `json:"deviceID,omitempty"`
	IPAddress        string    `json:"ipAddress,omitempty"`
	Status           string    `json:"status,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
	LastActivity     time.Time `json:"lastActivity"`
	LastHeartbeat    time.Time `json:"lastHeartbeat,omitempty"`
	ProcessingStatus string    `json:"processingStatus,omitempty"`
	ActiveDeviceID   string    `json:"activeDeviceID,omitempty"`
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

// TopupRequest stores credit topup requests from users via Telegram bot.
type TopupRequest struct {
	ID                    string    `json:"id"`
	UserID                string    `json:"userID"`
	TelegramID            int64     `json:"telegramID"`
	Amount                int       `json:"amount"`
	PaymentReceiptURL     string    `json:"paymentReceiptURL,omitempty"`
	Status                string    `json:"status"` // pending, approved, denied, expired
	QRCodeRef             string    `json:"qrCodeRef,omitempty"`
	TelegramMessageID     int       `json:"telegramMessageID,omitempty"`
	AdminChannelMessageID int       `json:"adminChannelMessageID,omitempty"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
	ProcessedAt           time.Time `json:"processedAt,omitempty"`
	ProcessedBy           string    `json:"processedBy,omitempty"`
	Notes                 string    `json:"notes,omitempty"`
}

// AdminSettings stores admin-configurable application settings.
type AdminSettings struct {
	ID                  string    `json:"id"`
	ProxyURL            string    `json:"proxyURL,omitempty"`
	APIKey              string    `json:"apiKey,omitempty"`
	QRCodeImage         string    `json:"qrCodeImage,omitempty"`
	PaymentInstructions string    `json:"paymentInstructions,omitempty"`
	TelegramBotToken    string    `json:"telegramBotToken,omitempty"`
	NotificationChannel string    `json:"notificationChannel,omitempty"`
	ApprovalChannel     string    `json:"approvalChannel,omitempty"`
	MinTopupAmount      int       `json:"minTopupAmount"`
	UpdatedAt           time.Time `json:"updatedAt"`
	UpdatedBy           string    `json:"updatedBy,omitempty"`
}
