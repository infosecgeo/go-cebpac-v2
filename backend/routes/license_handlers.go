package routes

import (
"context"
"net/http"
"time"

"cebupac/backend/database"
"cebupac/backend/logger"

"github.com/gin-gonic/gin"
"github.com/google/uuid"
"crypto/rand"
"fmt"
"math/big"
)

// handleAdminGetLicenses gets all licenses
func handleAdminGetLicenses(c *gin.Context) {
db := database.GetDatabase()
ctx := context.TODO()

licensesRepo, err := db.Licenses()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

licenses, err := licensesRepo.List(ctx)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get licenses"})
return
}

c.JSON(http.StatusOK, gin.H{
"licenses": licenses,
"count": len(licenses),
})
}

// handleAdminCreateLicense creates a new license
func handleAdminCreateLicense(c *gin.Context) {
var req struct {
ExpiresInDays int    `json:"expires_in_days"`
MaxDevices    int    `json:"max_devices"`
}

if err := c.ShouldBindJSON(&req); err != nil {
c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
return
}

// Set defaults
if req.ExpiresInDays <= 0 {
req.ExpiresInDays = 365 // Default 1 year
}
if req.MaxDevices <= 0 {
req.MaxDevices = 1
}

db := database.GetDatabase()
ctx := context.TODO()
adminID := c.GetString("user_id")

licensesRepo, err := db.Licenses()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

// Generate unique license key
licenseKey := generateLicenseKey()

license := database.License{
ID:        "license_" + uuid.New().String(),
Key:       licenseKey,
Status:    database.LicenseStatusActive,
ExpiresAt: time.Now().AddDate(0, 0, req.ExpiresInDays),
MaxDevices: req.MaxDevices,
CreatedAt: time.Now(),
}

err = licensesRepo.Create(ctx, license)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create license"})
return
}

logger.GetLogger().Info("License created", map[string]string{
"admin_id": adminID,
"license_key": licenseKey,
"expires_at": license.ExpiresAt.Format("2006-01-02"),
})

c.JSON(http.StatusCreated, gin.H{
"message": "License created successfully",
"license": license,
})
}

// handleAdminUpdateLicense updates a license
func handleAdminUpdateLicense(c *gin.Context) {
licenseID := c.Param("id")
var req struct {
Status     string `json:"status"`
ExpiresAt  string `json:"expires_at"`
MaxDevices int    `json:"max_devices"`
}

if err := c.ShouldBindJSON(&req); err != nil {
c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
return
}

db := database.GetDatabase()
ctx := context.TODO()
adminID := c.GetString("user_id")

licensesRepo, err := db.Licenses()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

license, found, err := licensesRepo.Get(ctx, licenseID)
if err != nil || !found {
c.JSON(http.StatusNotFound, gin.H{"error": "License not found"})
return
}

// Update fields
if req.Status != "" {
license.Status = req.Status
}
if req.ExpiresAt != "" {
expiresAt, err := time.Parse("2006-01-02", req.ExpiresAt)
if err == nil {
license.ExpiresAt = expiresAt
}
}
if req.MaxDevices > 0 {
license.MaxDevices = req.MaxDevices
}

err = licensesRepo.Upsert(ctx, license)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update license"})
return
}

logger.GetLogger().Info("License updated", map[string]string{
"admin_id": adminID,
"license_id": licenseID,
})

c.JSON(http.StatusOK, gin.H{"message": "License updated successfully"})
}

// handleAdminRevokeLicense revokes a license
func handleAdminRevokeLicense(c *gin.Context) {
licenseID := c.Param("id")

db := database.GetDatabase()
ctx := context.TODO()
adminID := c.GetString("user_id")

licensesRepo, err := db.Licenses()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
return
}

license, found, err := licensesRepo.Get(ctx, licenseID)
if err != nil || !found {
c.JSON(http.StatusNotFound, gin.H{"error": "License not found"})
return
}

license.Status = database.LicenseStatusRevoked
err = licensesRepo.Upsert(ctx, license)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke license"})
return
}

logger.GetLogger().Info("License revoked", map[string]string{
"admin_id": adminID,
"license_id": licenseID,
})

c.JSON(http.StatusOK, gin.H{"message": "License revoked successfully"})
}

// generateLicenseKey generates a random license key
func generateLicenseKey() string {
const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const keyLength = 20
const segments = 4
const segmentLength = 5

key := make([]byte, keyLength)
for i := range key {
n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
key[i] = chars[n.Int64()]
}

// Format as XXXXX-XXXXX-XXXXX-XXXXX
formatted := ""
for i := 0; i < segments; i++ {
if i > 0 {
formatted += "-"
}
formatted += string(key[i*segmentLength : (i+1)*segmentLength])
}

return formatted
}

// Placeholder implementations
func handleAdminGetProxies(c *gin.Context)       { c.JSON(http.StatusOK, gin.H{"proxies": []interface{}{}}) }
func handleAdminAddProxy(c *gin.Context)         { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminDeleteProxy(c *gin.Context)      { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
func handleAdminGetSessions(c *gin.Context)      { c.JSON(http.StatusOK, gin.H{"sessions": []interface{}{}}) }
func handleAdminTerminateSession(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "Not implemented yet"}) }
