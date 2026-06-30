package routes

import (
	"cebupac/backend/auth"
	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/logger"
	"cebupac/backend/middleware"
	"cebupac/backend/websocket"
	"crypto/rand"
	"math/big"
	"net/http"
	"time"

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
		
		// Configuration
		admin.GET("/config", handleAdminGetConfig)
		admin.PUT("/config", handleAdminUpdateConfig)
		
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
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		DeviceID string `json:"device_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	db := database.GetDatabase()
	
	// Get user
	user, err := db.GetUserByUsername(req.Username)
	if err != nil {
		logger.LogAuth("login", req.Username, "failed_user_not_found")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Verify password
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		logger.LogAuth("login", req.Username, "failed_wrong_password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Check status
	if user.Status != "active" {
		logger.LogAuth("login", req.Username, "failed_inactive")
		c.JSON(http.StatusForbidden, gin.H{"error": "Account is not active"})
		return
	}

	// Terminate previous session if exists
	if user.SessionID != "" {
		db.DeleteSession(user.SessionID)
		websocket.GetHub().SendToUser(user.ID, websocket.TypeUserUpdate, map[string]interface{}{
			"action":  "session_terminated",
			"message": "New login detected. Your session has been terminated.",
		})
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

	// Create session
	session := &database.Session{
		ID:           generateSessionID(),
		UserID:       user.ID,
		Token:        token,
		DeviceID:     req.DeviceID,
		IPAddress:    c.ClientIP(),
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		LastActivity: time.Now(),
	}
	db.CreateSession(session)

	// Update user
	user.SessionID = session.ID
	user.LastLogin = time.Now()
	user.LastIP = c.ClientIP()
	user.DeviceID = req.DeviceID
	db.UpdateUser(user)

	logger.LogAuth("login", req.Username, "success")

	c.JSON(http.StatusOK, gin.H{
		"token":         token,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"credits":  user.Credits,
		},
	})
}

func handleRegister(c *gin.Context) {
	var req struct {
		Username   string `json:"username" binding:"required"`
		Password   string `json:"password" binding:"required"`
		LicenseKey string `json:"license_key" binding:"required"`
		DeviceID   string `json:"device_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	db := database.GetDatabase()

	// Check if username exists
	if _, err := db.GetUserByUsername(req.Username); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		return
	}

	// Validate password
	if err := auth.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash password
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}

	// Create user
	user := &database.User{
		ID:           generateUserID(),
		Username:     req.Username,
		PasswordHash: hash,
		LicenseKey:   req.LicenseKey,
		Credits:      0,
		Role:         "user",
		Status:       "active",
		CreatedAt:    time.Now(),
		DeviceID:     req.DeviceID,
	}

	if err := db.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	logger.LogAuth("register", req.Username, "success")

	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully",
		"user_id": user.ID,
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
			// Fallback to less secure method if crypto fails
			b[i] = letters[i%len(letters)]
		} else {
			b[i] = letters[idx.Int64()]
		}
	}
	return string(b)
}
