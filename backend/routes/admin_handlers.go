package routes

import (
"context"
"net/http"
"time"

"cebupac/backend/auth"
"cebupac/backend/database"
"cebupac/backend/logger"

"github.com/gin-gonic/gin"
"github.com/google/uuid"
)

// handleAdminLogin handles admin login with username/password
func handleAdminLogin(c *gin.Context) {
var req struct {
Username string `json:"username" binding:"required"`
Password string `json:"password" binding:"required"`
}

if err := c.ShouldBindJSON(&req); err != nil {
c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
return
}

db := database.GetDatabase()
ctx := context.TODO()

// Get admin user by username
usersRepo, err := db.Users()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

user, found, err := usersRepo.FindOne(ctx, func(u database.User) bool {
return u.Username == req.Username && u.Role == database.UserRoleAdmin
})

if err != nil || !found {
logger.LogAuth("admin_login", req.Username, "failed_user_not_found")
c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
return
}

// Verify password
if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
logger.LogAuth("admin_login", req.Username, "failed_wrong_password")
c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
return
}

// Check status
if user.Status != "active" {
logger.LogAuth("admin_login", req.Username, "failed_inactive")
c.JSON(http.StatusForbidden, gin.H{"error": "Account is not active"})
return
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

// Update user login info
user.LastLogin = time.Now()
user.LastIP = c.ClientIP()
usersRepo.Upsert(ctx, user)

logger.LogAuth("admin_login", req.Username, "success")

c.JSON(http.StatusOK, gin.H{
"token":         token,
"refresh_token": refreshToken,
"user": gin.H{
"id":       user.ID,
"username": user.Username,
"role":     user.Role,
},
})
}

// handleAdminGetSettings gets admin settings
func handleAdminGetSettings(c *gin.Context) {
db := database.GetDatabase()
ctx := context.TODO()

settingsRepo, err := db.AdminSettings()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

settings, err := settingsRepo.List(ctx)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get settings"})
return
}

if len(settings) == 0 {
// Return default empty settings
c.JSON(http.StatusOK, gin.H{
"proxy_url": "",
"api_key": "",
"qr_code_image": "",
"payment_instructions": "Please scan the QR code and send payment proof.",
"telegram_bot_token": "",
"notification_channel": "",
"approval_channel": "",
"min_topup_amount": 1,
})
return
}

c.JSON(http.StatusOK, gin.H{
"proxy_url": settings[0].ProxyURL,
"api_key": settings[0].APIKey,
"qr_code_image": settings[0].QRCodeImage,
"payment_instructions": settings[0].PaymentInstructions,
"telegram_bot_token": settings[0].TelegramBotToken,
"notification_channel": settings[0].NotificationChannel,
"approval_channel": settings[0].ApprovalChannel,
"min_topup_amount": settings[0].MinTopupAmount,
})
}

// handleAdminUpdateSettings updates admin settings
func handleAdminUpdateSettings(c *gin.Context) {
var req struct {
ProxyURL            string `json:"proxy_url"`
APIKey              string `json:"api_key"`
PaymentInstructions string `json:"payment_instructions"`
TelegramBotToken    string `json:"telegram_bot_token"`
NotificationChannel string `json:"notification_channel"`
ApprovalChannel     string `json:"approval_channel"`
MinTopupAmount      int    `json:"min_topup_amount"`
}

if err := c.ShouldBindJSON(&req); err != nil {
c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
return
}

db := database.GetDatabase()
ctx := context.TODO()
adminID := c.GetString("user_id")

settingsRepo, err := db.AdminSettings()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

settings, err := settingsRepo.List(ctx)
var setting database.AdminSettings

if err == nil && len(settings) > 0 {
setting = settings[0]
} else {
setting = database.AdminSettings{
ID: "settings_" + uuid.New().String(),
}
}

// Update fields
if req.ProxyURL != "" {
setting.ProxyURL = req.ProxyURL
}
if req.APIKey != "" {
setting.APIKey = req.APIKey
}
if req.PaymentInstructions != "" {
setting.PaymentInstructions = req.PaymentInstructions
}
if req.TelegramBotToken != "" {
setting.TelegramBotToken = req.TelegramBotToken
}
if req.NotificationChannel != "" {
setting.NotificationChannel = req.NotificationChannel
}
if req.ApprovalChannel != "" {
setting.ApprovalChannel = req.ApprovalChannel
}
if req.MinTopupAmount > 0 {
setting.MinTopupAmount = req.MinTopupAmount
}

setting.UpdatedAt = time.Now()
setting.UpdatedBy = adminID

err = settingsRepo.Upsert(ctx, setting)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
return
}

logger.GetLogger().Info("Admin settings updated", map[string]string{
"admin_id": adminID,
})

c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

// handleAdminUploadQR handles QR code upload
func handleAdminUploadQR(c *gin.Context) {
file, err := c.FormFile("qr_code")
if err != nil {
c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
return
}

// Save file
filename := "qr_code_" + time.Now().Format("20060102150405") + ".png"
filepath := "storage/qr_codes/" + filename

if err := c.SaveUploadedFile(file, filepath); err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
return
}

// Update settings with new QR code path
db := database.GetDatabase()
ctx := context.TODO()
adminID := c.GetString("user_id")

settingsRepo, err := db.AdminSettings()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

settings, err := settingsRepo.List(ctx)
var setting database.AdminSettings

if err == nil && len(settings) > 0 {
setting = settings[0]
} else {
setting = database.AdminSettings{
ID: "settings_" + uuid.New().String(),
}
}

setting.QRCodeImage = filepath
setting.UpdatedAt = time.Now()
setting.UpdatedBy = adminID

settingsRepo.Upsert(ctx, setting)

c.JSON(http.StatusOK, gin.H{
"message": "QR code uploaded successfully",
"path": filepath,
})
}

// handleAdminGetTopups gets topup requests with filters
func handleAdminGetTopups(c *gin.Context) {
statusFilter := c.Query("status") // pending, approved, denied

db := database.GetDatabase()
ctx := context.TODO()

topupRepo, err := db.TopupRequests()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

var topups []database.TopupRequest
if statusFilter != "" {
topups, err = topupRepo.Filter(ctx, func(t database.TopupRequest) bool {
return t.Status == statusFilter
})
} else {
topups, err = topupRepo.List(ctx)
}

if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get topup requests"})
return
}

c.JSON(http.StatusOK, gin.H{
"topups": topups,
"count": len(topups),
})
}
