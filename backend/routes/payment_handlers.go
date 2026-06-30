package routes

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cebupac/backend/database"
	"cebupac/backend/logger"
	"cebupac/backend/services"
	"cebupac/backend/websocket"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PaymentRequest represents a payment processing request
type PaymentRequest struct {
	Mode         string `json:"mode" binding:"required"`
	Card         string `json:"card" binding:"required"`
	XAuthToken   string `json:"xAuthToken"`
	BearerToken  string `json:"bearerToken"`
	HPPContent   string `json:"hpp"`
}

// PaymentResponse represents a payment processing response
type PaymentResponse struct {
	Success          bool                 `json:"success"`
	Message          string               `json:"message"`
	RecordLocator    string               `json:"record_locator,omitempty"`
	LocCode          string               `json:"loc_code,omitempty"`
	LocSubCode       string               `json:"loc_sub_code,omitempty"`
	FraudStatus      string               `json:"fraud_status,omitempty"`
	Itinerary        *services.Itinerary  `json:"itinerary,omitempty"`
	TransactionID    string               `json:"transaction_id,omitempty"`
	CreditsUsed      int                  `json:"credits_used,omitempty"`
	CreditsRemaining int                  `json:"credits_remaining,omitempty"`
}

// handlePaymentProcess processes payment requests
func handlePaymentProcess(c *gin.Context) {
	userID := c.GetString("user_id")
	username := c.GetString("username")

	var req PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Get user from database
	db := database.GetDatabase()
	user, err := db.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Check if user has credits
	if user.Credits <= 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Insufficient credits",
			"credits": user.Credits,
		})
		return
	}

	// Validate card format
	cardParts := strings.Split(strings.ReplaceAll(req.Card, " ", ""), "|")
	if len(cardParts) < 3 || len(cardParts) > 4 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid card format. Use: number|month|year or number|month|year|cvv",
		})
		return
	}

	cardNumber := cardParts[0]
	month := cardParts[1]
	year := cardParts[2]

	// Log payment attempt
	logger.GetLogger().Info("Payment processing started", map[string]string{
		"user_id":   userID,
		"username":  username,
		"card_last4": getLastFourDigits(cardNumber),
		"mode":      req.Mode,
	})

	// Notify via WebSocket
	hub := websocket.GetHub()
	hub.SendToUser(userID, websocket.TypeTaskStart, map[string]interface{}{
		"message": "Payment processing started",
		"card": maskCard(cardNumber),
	})

	// Create payment task
	taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Process payment using service
	paymentService := services.GetPaymentService()
	result, err := paymentService.ProcessPayment(taskCtx, services.PaymentRequest{
		UserID:       userID,
		CardNumber:   cardNumber,
		Month:        month,
		Year:         year,
		XAuthToken:   req.XAuthToken,
		BearerToken:  req.BearerToken,
		HPPContent:   req.HPPContent,
	}, services.NoopProgressTracker{})

	if err != nil {
		logger.GetLogger().Error("Payment processing failed", map[string]string{
			"user_id": userID,
			"error":   err.Error(),
		})

		hub.SendToUser(userID, websocket.TypeTaskError, map[string]interface{}{
			"message": "Payment processing failed: " + err.Error(),
		})

		c.JSON(http.StatusInternalServerError, PaymentResponse{
			Success: false,
			Message: "Payment processing failed: " + err.Error(),
		})
		return
	}

	// Create transaction record
	transaction := &database.Transaction{
		ID:            generateTransactionID(),
		UserID:        userID,
		CardLast4:     getLastFourDigits(cardNumber),
		Amount:        0, // Would be populated from HPP or response
		Status:        getStatus(result.Success),
		LocCode:       result.LocCode,
		LocSubCode:    result.LocSubCode,
		FraudStatus:   result.FraudStatus,
		RecordLocator: result.RecordLocator,
		Timestamp:     time.Now(),
	}
	db.CreateTransaction(transaction)

	// Deduct credits only on success
	creditsUsed := 0
	if result.Success {
		creditsUsed = 1
		// Double-check credits (prevents race condition)
		if user.Credits < creditsUsed {
			logger.GetLogger().Warn("Insufficient credits after payment", map[string]string{
				"user_id":      userID,
				"credits":      fmt.Sprintf("%d", user.Credits),
				"required":     fmt.Sprintf("%d", creditsUsed),
			})
		} else {
			user.Credits -= creditsUsed
			db.UpdateUser(user)
			logger.GetLogger().Info("Credits deducted", map[string]string{
				"user_id":        userID,
				"credits_used":   fmt.Sprintf("%d", creditsUsed),
				"credits_remain": fmt.Sprintf("%d", user.Credits),
			})
			
			// Send real-time credit update
			hub.BroadcastCreditUpdate(userID, user.Credits)
		}
		
		// Send Telegram notification for successful transaction
		go func() {
			// Import telegram package at the top of the file
			// Format itinerary details
			itineraryDetails := "N/A"
			if result.Itinerary != nil {
				itineraryDetails = fmt.Sprintf("Passenger: %s\nRoute: %s\nFlight: %s",
					result.Itinerary.PassengerName,
					result.Itinerary.Route,
					result.Itinerary.FlightNumber,
				)
			}
			
			// Try to send notification (non-blocking)
			// bot, err := telegram.GetBot()
			// if err == nil {
			//     bot.SendTransactionNotification(context.Background(), transaction, itineraryDetails)
			// }
		}()
	}

	// Notify via WebSocket
	hub.SendToUser(userID, websocket.TypeTaskComplete, map[string]interface{}{
		"success":         result.Success,
		"message":         result.Message,
		"record_locator":  result.RecordLocator,
		"credits_used":    creditsUsed,
		"credits_remain":  user.Credits,
	})

	// Log payment result
	logger.LogPayment(getLastFourDigits(cardNumber), getStatus(result.Success), result.Message)

	// Return response
	response := PaymentResponse{
		Success:          result.Success,
		Message:          result.Message,
		RecordLocator:    result.RecordLocator,
		LocCode:          result.LocCode,
		LocSubCode:       result.LocSubCode,
		FraudStatus:      result.FraudStatus,
		Itinerary:        result.Itinerary,
		TransactionID:    transaction.ID,
		CreditsUsed:      creditsUsed,
		CreditsRemaining: user.Credits,
	}

	c.JSON(http.StatusOK, response)
}

// handlePaymentHistory returns user's payment history
func handlePaymentHistory(c *gin.Context) {
	userID := c.GetString("user_id")
	
	db := database.GetDatabase()
	transactions, err := db.GetTransactionsByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get payment history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
	})
}

// handlePaymentStatus returns status of a specific transaction
func handlePaymentStatus(c *gin.Context) {
	transactionID := c.Param("id")
	userID := c.GetString("user_id")
	
	db := database.GetDatabase()
	transaction, err := db.GetTransaction(transactionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	// Verify user owns this transaction
	if transaction.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, transaction)
}

// Admin handlers implementation
func handleAdminGetUsers(c *gin.Context) {
	db := database.GetDatabase()
	users, err := db.GetAllUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get users"})
		return
	}

	// Remove sensitive data
	sanitized := make([]map[string]interface{}, len(users))
	for i, user := range users {
		sanitized[i] = map[string]interface{}{
			"id":         user.ID,
			"username":   user.Username,
			"credits":    user.Credits,
			"role":       user.Role,
			"status":     user.Status,
			"created_at": user.CreatedAt,
			"last_login": user.LastLogin,
		}
	}

	c.JSON(http.StatusOK, gin.H{"users": sanitized, "count": len(users)})
}

func handleAdminGetUser(c *gin.Context) {
	userID := c.Param("id")
	db := database.GetDatabase()
	
	user, err := db.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          user.ID,
		"username":    user.Username,
		"credits":     user.Credits,
		"role":        user.Role,
		"status":      user.Status,
		"license_key": user.LicenseKey,
		"created_at":  user.CreatedAt,
		"last_login":  user.LastLogin,
		"last_ip":     user.LastIP,
		"device_id":   user.DeviceID,
	})
}

func handleAdminUpdateUser(c *gin.Context) {
	userID := c.Param("id")
	var req struct {
		Credits int    `json:"credits"`
		Role    string `json:"role"`
		Status  string `json:"status"`
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

	// Update fields
	if req.Credits > 0 {
		user.Credits = req.Credits
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Status != "" {
		user.Status = req.Status
	}

	if err := db.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	logger.GetLogger().Info("Admin updated user", map[string]string{
		"admin_id": c.GetString("user_id"),
		"user_id":  userID,
	})

	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})
}

func handleAdminDeleteUser(c *gin.Context) {
	userID := c.Param("id")
	db := database.GetDatabase()
	
	if err := db.DeleteUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	logger.GetLogger().Info("Admin deleted user", map[string]string{
		"admin_id": c.GetString("user_id"),
		"user_id":  userID,
	})

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

func handleAdminAddCredits(c *gin.Context) {
	userID := c.Param("id")
	var req struct {
		Credits int `json:"credits" binding:"required"`
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

	user.Credits += req.Credits
	if err := db.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add credits"})
		return
	}

	logger.GetLogger().Info("Admin added credits", map[string]string{
		"admin_id": c.GetString("user_id"),
		"user_id":  userID,
		"credits":  fmt.Sprintf("%d", req.Credits),
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Credits added successfully",
		"new_balance": user.Credits,
	})
}

func handleAdminGetStats(c *gin.Context) {
	db := database.GetDatabase()
	hub := websocket.GetHub()

	stats := gin.H{
		"users": gin.H{
			"total":  db.CountActiveUsers(),
			"active": db.CountActiveUsers(),
		},
		"sessions": gin.H{
			"active": db.CountActiveSessions(),
		},
		"websocket": gin.H{
			"connected_clients": hub.GetConnectedClients(),
		},
		"workers": gin.H{
			"pool_size":    10, // Static for now - could expose via getter
			"queue_length": 0,  // Static for now - could expose via getter
		},
	}

	c.JSON(http.StatusOK, stats)
}

func handleGetLogs(c *gin.Context) {
	count := 100 // Default
	if countParam := c.Query("count"); countParam != "" {
		fmt.Sscanf(countParam, "%d", &count)
	}

	logs := logger.GetLogger().GetRecentLogs(count)
	c.JSON(http.StatusOK, gin.H{"logs": logs, "count": len(logs)})
}

// Helper functions
func getLastFourDigits(cardNumber string) string {
	if len(cardNumber) < 4 {
		return cardNumber
	}
	return cardNumber[len(cardNumber)-4:]
}

func maskCard(cardNumber string) string {
	if len(cardNumber) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(cardNumber)-4) + getLastFourDigits(cardNumber)
}

func getStatus(success bool) string {
	if success {
		return "success"
	}
	return "failed"
}

func generateTransactionID() string {
	return "txn_" + uuid.New().String()
}
