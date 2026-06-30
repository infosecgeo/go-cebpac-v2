package routes

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cebupac/backend/auth"
	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/logger"
	"cebupac/backend/middleware"
	"cebupac/backend/websocket"
	"crypto/rand"
	"math/big"

	"github.com/gin-gonic/gin"
)

// SetupRouter initializes all routes
func SetupRouter() *gin.Engine {
	cfg := config.GetConfig()
	
	// Set Gin mode based on environment
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	
	// Middleware
	r.Use(gin.Recovery())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS())
	
	// Rate limiting
	if cfg.RateLimit.Enabled {
		r.Use(middleware.RateLimitMiddleware(cfg.RateLimit.RequestsPerMin))
	}

	// Static files
	r.Static("/assets", "./frontend/assets")
	r.Static("/public", "./public")
	
	// Main page
	r.GET("/", func(c *gin.Context) {
		c.File("./public/index.html")
	})

	// Public routes
	public := r.Group("/api/v1")
	{
		public.POST("/auth/login", handleLogin)
		public.POST("/auth/register", handleRegister)
		public.POST("/auth/refresh", handleRefreshToken)
		public.POST("/auth/admin/login", handleAdminLogin)
	}

	// Protected routes
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware())
	{
		// User routes
		protected.GET("/user/profile", handleGetProfile)
		protected.PUT("/user/profile", handleUpdateProfile)
		protected.GET("/user/credits", handleGetCredits)
		
		// Payment routes
		protected.POST("/payment/process", handlePaymentProcess)
		protected.GET("/payment/history", handlePaymentHistory)
		protected.GET("/payment/status/:id", handlePaymentStatus)
		
		// License routes
		protected.POST("/license/validate", handleLicenseValidate)
		protected.GET("/license/status", handleLicenseStatus)
		
		// WebSocket
		protected.GET("/ws", handleWebSocket)
		
		// Logs
		protected.GET("/logs", handleGetLogs)
	}

	// Admin routes
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RoleMiddleware("admin"))
	{
		// Settings management
		admin.GET("/settings", handleAdminGetSettings)
		admin.PUT("/settings", handleAdminUpdateSettings)
		admin.POST("/settings/qr", handleAdminUploadQR)
		
		// User management
		admin.GET("/users", handleAdminGetUsers)
		admin.GET("/users/:id", handleAdminGetUser)
		admin.PUT("/users/:id", handleAdminUpdateUser)
		admin.DELETE("/users/:id", handleAdminDeleteUser)
		admin.POST("/users/:id/credits", handleAdminAddCredits)
		
		// License management
		admin.GET("/licenses", handleAdminGetLicenses)
		admin.POST("/licenses", handleAdminCreateLicense)
		admin.PUT("/licenses/:id", handleAdminUpdateLicense)
		admin.DELETE("/licenses/:id", handleAdminRevokeLicense)
		
		// Proxy management
		admin.GET("/proxies", handleAdminGetProxies)
		admin.POST("/proxies", handleAdminAddProxy)
		admin.DELETE("/proxies/:id", handleAdminDeleteProxy)
		
		// Topup management
		admin.GET("/topups", handleAdminGetTopups)
		
		// System stats
		admin.GET("/stats", handleAdminGetStats)
		admin.GET("/sessions", handleAdminGetSessions)
		admin.DELETE("/sessions/:id", handleAdminTerminateSession)
	}

	return r
}

// Auth handlers
func handleLogin(c *gin.Context) {
	var req struct {
		LicenseKey string `json:"license_key" binding:"required"`
		DeviceID   string `json:"device_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	db := database.GetDatabase()
	ctx := context.TODO()
	
	// Get user by license key
	usersRepo, err := db.Users()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	user, found, err := usersRepo.FindOne(ctx, func(u database.User) bool {
		return u.LicenseKey == req.LicenseKey
	})
	
	if err != nil || !found {
		logger.LogAuth("login", req.LicenseKey, "failed_license_not_found")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid license key"})
		return
	}

	// Check status
	if user.Status != "active" {
		logger.LogAuth("login", req.LicenseKey, "failed_inactive")
		c.JSON(http.StatusForbidden, gin.H{"error": "Account is not active"})
		return
	}

	// Check if terms accepted
	if !user.TermsAccepted {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Terms and conditions not accepted",
			"requires_terms": true,
		})
		return
	}

	// Check session conflict - terminate previous session if exists
	if user.SessionID != "" {
		sessionsRepo, _ := db.Sessions()
		oldSession, found, _ := sessionsRepo.Get(ctx, user.SessionID)
		if found && oldSession.Status == database.SessionStatusActive {
			// Check if processing
			if oldSession.ProcessingStatus == database.ProcessingStatusActive {
				// Send immediate halt signal
				websocket.GetHub().SendToUser(user.ID, websocket.TypeTaskError, map[string]interface{}{
					"action":  "halt",
					"message": "Processing halted due to new login from another device.",
				})
			}
			
			// Send kicked out message
			websocket.GetHub().SendKickedOut(user.ID, "New login detected from another device")
			
			// Delete old session
			sessionsRepo.Delete(ctx, user.SessionID)
		}
	}

	// Generate tokens
	token, err := auth.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		logger.GetLogger().Error("Failed to generate token", map[string]string{"error": err.Error()})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	refreshToken, err := auth.GenerateRefreshToken(user.ID)
	if err != nil {
		logger.GetLogger().Error("Failed to generate refresh token", map[string]string{"error": err.Error()})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// Create new session
	sessionsRepo, _ := db.Sessions()
	session := database.Session{
		ID:               generateSessionID(),
		UserID:           user.ID,
		Token:            token,
		DeviceID:         req.DeviceID,
		IPAddress:        c.ClientIP(),
		Status:           database.SessionStatusActive,
		CreatedAt:        time.Now(),
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		LastActivity:     time.Now(),
		LastHeartbeat:    time.Now(),
		ProcessingStatus: database.ProcessingStatusIdle,
		ActiveDeviceID:   req.DeviceID,
	}
	sessionsRepo.Create(ctx, session)

	// Update user
	user.SessionID = session.ID
	user.LastLogin = time.Now()
	user.LastIP = c.ClientIP()
	user.DeviceID = req.DeviceID
	usersRepo.Upsert(ctx, user)

	logger.LogAuth("login", req.LicenseKey, "success")

	c.JSON(http.StatusOK, gin.H{
		"token":         token,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":                user.ID,
			"username":          user.Username,
			"role":              user.Role,
			"credits":           user.Credits,
			"telegram_linked":   user.TelegramID != 0,
			"telegram_username": user.TelegramUsername,
		},
	})
}

func handleRegister(c *gin.Context) {
	var req struct {
		LicenseKey    string `json:"license_key" binding:"required"`
		TermsAccepted bool   `json:"terms_accepted" binding:"required"`
		DeviceID      string `json:"device_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if !req.TermsAccepted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You must accept the terms and conditions"})
		return
	}

	db := database.GetDatabase()
	ctx := context.TODO()

	// Validate license exists and is available
	licensesRepo, err := db.Licenses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	license, found, err := licensesRepo.FindOne(ctx, func(l database.License) bool {
		return l.Key == req.LicenseKey
	})

	if err != nil || !found {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid license key"})
		return
	}

	// Check if license is already used
	if license.UserID != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "License key already in use"})
		return
	}

	// Check if license is expired or revoked
	if license.Status != database.LicenseStatusActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "License is not active"})
		return
	}

	// Check if license already linked to a Telegram account (enforce 1 license per TG account)
	if license.LinkedTelegramID != 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "License already linked to a Telegram account"})
		return
	}

	// Create user
	usersRepo, _ := db.Users()
	user := database.User{
		ID:                 generateUserID(),
		Username:           "user_" + req.LicenseKey[:8], // Temp username
		PasswordHash:       "", // No password for license-based auth
		LicenseKey:         req.LicenseKey,
		Credits:            0, // Starts with 0, requires topup
		Role:               database.UserRoleUser,
		Status:             database.UserStatusPending, // Pending until first topup approved
		RegistrationStatus: database.RegistrationStatusPending,
		CreatedAt:          time.Now(),
		DeviceID:           req.DeviceID,
		TermsAccepted:      true,
	}

	if err := usersRepo.Create(ctx, user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Update license with user ID
	license.UserID = user.ID
	licensesRepo.Upsert(ctx, license)

	logger.LogAuth("register", req.LicenseKey, "success_pending_topup")

	c.JSON(http.StatusCreated, gin.H{
		"message": "Registration successful! Please link your Telegram account and top up at least 1 credit to activate your account.",
		"user_id": user.ID,
		"status":  "pending",
		"requires_topup": true,
		"telegram_bot": "Search for @YourBotName on Telegram and use /link " + req.LicenseKey,
	})
}

func handleRefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Validate refresh token
	claims, err := auth.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	db := database.GetDatabase()
	user, err := db.GetUser(claims.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}

	// Generate new access token
	token, err := auth.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// User handlers
func handleGetProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	db := database.GetDatabase()
	
	user, err := db.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          user.ID,
		"username":    user.Username,
		"license_key": user.LicenseKey,
		"credits":     user.Credits,
		"role":        user.Role,
		"status":      user.Status,
		"created_at":  user.CreatedAt,
		"last_login":  user.LastLogin,
	})
}

func handleUpdateProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		LicenseKey string `json:"license_key"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	db := database.GetDatabase()
	user, err := db.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if req.LicenseKey != "" {
		user.LicenseKey = req.LicenseKey
	}

	if err := db.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated"})
}

func handleGetCredits(c *gin.Context) {
	userID := c.GetString("user_id")
	db := database.GetDatabase()
	
	user, err := db.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"credits": user.Credits})
}

// WebSocket handler
func handleWebSocket(c *gin.Context) {
	userID := c.GetString("user_id")
	hub := websocket.GetHub()
	websocket.ServeWs(hub, c.Writer, c.Request, userID)
}

// Placeholder handlers for license (to be implemented)
func handleLicenseValidate(c *gin.Context)       { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleLicenseStatus(c *gin.Context)         { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminGetLicenses(c *gin.Context)      { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminCreateLicense(c *gin.Context)    { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminUpdateLicense(c *gin.Context)    { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminRevokeLicense(c *gin.Context)    { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminGetProxies(c *gin.Context)       { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminAddProxy(c *gin.Context)         { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminDeleteProxy(c *gin.Context)      { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminGetConfig(c *gin.Context)        { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminUpdateConfig(c *gin.Context)     { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminGetSessions(c *gin.Context)      { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminTerminateSession(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }

// Helper functions
func generateUserID() string {
	return "user_" + time.Now().Format("20060102150405") + "_" + randomString(8)
}

func generateSessionID() string {
	return "session_" + time.Now().Format("20060102150405") + "_" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			// Return error instead of insecure fallback
			panic(fmt.Sprintf("Failed to generate random string: %v", err))
		}
		b[i] = letters[idx.Int64()]
	}
	return string(b)
}
