package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"cebupac/backend/auth"
	"cebupac/backend/config"
	"cebupac/backend/logger"

	"github.com/gin-gonic/gin"
)

// Simple in-memory rate limiter
type rateLimiterBucket struct {
	tokens    int
	lastCheck time.Time
}

var (
	rateLimiterMap  = make(map[string]*rateLimiterBucket)
	rateLimiterLock sync.Mutex
)

// RateLimitMiddleware limits requests per IP
func RateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		
		rateLimiterLock.Lock()
		bucket, exists := rateLimiterMap[ip]
		now := time.Now()
		
		if !exists {
			bucket = &rateLimiterBucket{
				tokens:    requestsPerMinute - 1,
				lastCheck: now,
			}
			rateLimiterMap[ip] = bucket
			rateLimiterLock.Unlock()
			c.Next()
			return
		}
		
		// Refill tokens based on elapsed time
		elapsed := now.Sub(bucket.lastCheck)
		tokensToAdd := int(elapsed.Seconds() / 60.0 * float64(requestsPerMinute))
		if tokensToAdd > 0 {
			bucket.tokens = minInt(requestsPerMinute, bucket.tokens+tokensToAdd)
			bucket.lastCheck = now
		}
		
		// Check if request is allowed
		if bucket.tokens > 0 {
			bucket.tokens--
			rateLimiterLock.Unlock()
			c.Next()
			return
		}
		
		rateLimiterLock.Unlock()
		logger.LogSecurity("rate_limit_exceeded", fmt.Sprintf("IP: %s", ip))
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
		c.Abort()
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// AuthMiddleware validates JWT tokens for Gin
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix if present
		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		}

		// Validate token
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			logger.LogSecurity("invalid_token", fmt.Sprintf("IP: %s, Error: %s", c.ClientIP(), err.Error()))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Set user info in context
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// RoleMiddleware checks if user has required role
func RoleMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "Role not found"})
			c.Abort()
			return
		}

		if role != requiredRole {
			logger.LogSecurity("unauthorized_access", fmt.Sprintf("User: %s, Required: %s, Has: %s", c.GetString("user_id"), requiredRole, role))
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// LoggingMiddleware logs all requests
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		statusCode := c.Writer.Status()
		userID := c.GetString("user_id")
		if userID == "" {
			userID = "anonymous"
		}

		logger.LogAPIRequest(method, path, userID, statusCode)
	}
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders() gin.HandlerFunc {
	cfg := config.GetConfig()
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; object-src 'none'; img-src 'self' data:; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("X-XSS-Protection", "1; mode=block")
		
		if cfg.Server.Environment != "development" {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		c.Next()
	}
}

// CORS handles CORS headers
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*") // Configure properly in production
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,Accept,Origin,X-Requested-With")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Vary", "Origin")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
