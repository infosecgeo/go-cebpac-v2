package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/logger"
	"cebupac/backend/routes"
	"cebupac/backend/telegram"
	"cebupac/backend/websocket"
	"cebupac/backend/workers"
)

func main() {
	// Initialize configuration
	cfg := config.GetConfig()
	
	// Initialize logger
	log := logger.GetLogger()
	defer log.Close()

	log.Info("Starting CebuPacific Payment Processor v2.0", map[string]string{
		"environment": cfg.Server.Environment,
		"port":        cfg.GetServerPort(),
	})

	// Initialize database
	db := database.GetDatabase()
	log.Info("Database initialized", map[string]string{
		"storage_path": cfg.Database.StoragePath,
	})

	// Start database backup if enabled
	if cfg.Database.BackupEnabled {
		go db.StartBackupRoutine()
		log.Info("Database backup routine started", map[string]string{
			"interval_min": fmt.Sprintf("%d", cfg.Database.BackupIntervalMin),
		})
	}

	// Initialize WebSocket hub
	hub := websocket.GetHub()
	log.Info("WebSocket hub initialized")

	// Initialize worker pool
	pool := workers.GetPool()
	pool.Start()
	log.Info("Worker pool started", map[string]string{
		"pool_size": fmt.Sprintf("%d", cfg.Workers.PoolSize),
	})

	// Initialize Telegram bot (optional - will warn if not configured)
	bot, err := telegram.GetBot()
	if err == nil && bot != nil {
		if err := bot.Start(); err != nil {
			log.Warn("Failed to start Telegram bot", map[string]string{
				"error": err.Error(),
			})
		} else {
			log.Info("Telegram bot started")
		}
	} else {
		log.Info("Telegram bot not configured, skipping initialization")
	}

	// Setup router
	router := routes.SetupRouter()
	log.Info("Routes configured")

	// Create HTTP server
	addr := ":" + cfg.GetServerPort()
	srv := &http.Server{
		Addr:           addr,
		Handler:        router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Start server in goroutine
	go func() {
		log.Info("Server starting", map[string]string{
			"address": addr,
		})
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server", map[string]string{
				"error": err.Error(),
			})
		}
	}()

	fmt.Println("╔════════════════════════════════════════════════╗")
	fmt.Println("║   CebuPacific Payment Processor v2.0          ║")
	fmt.Println("║   Production-Ready System                     ║")
	fmt.Println("╠════════════════════════════════════════════════╣")
	fmt.Printf("║   Server:      http://localhost:%s           ║\n", cfg.GetServerPort())
	fmt.Printf("║   Environment: %-30s ║\n", cfg.Server.Environment)
	fmt.Printf("║   Workers:     %-30d ║\n", cfg.Workers.PoolSize)
	fmt.Println("╠════════════════════════════════════════════════╣")
	fmt.Println("║   Press Ctrl+C to shutdown                     ║")
	fmt.Println("╚════════════════════════════════════════════════╝")

	// Start system stats broadcaster
	go broadcastSystemStats(hub)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")
	fmt.Println("\n⏳ Graceful shutdown initiated...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeoutSecs)*time.Second)
	defer cancel()

	// Stop accepting new requests
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", map[string]string{
			"error": err.Error(),
		})
	}

	// Stop worker pool
	if err := pool.Shutdown(ctx); err != nil {
		log.Error("Error stopping worker pool", map[string]string{
			"error": err.Error(),
		})
	}
	log.Info("Worker pool stopped")

	// Stop Telegram bot
	bot, err := telegram.GetBot()
	if err == nil && bot != nil && bot.IsRunning() {
		if err := bot.Stop(ctx); err != nil {
			log.Error("Error stopping Telegram bot", map[string]string{
				"error": err.Error(),
			})
		}
		log.Info("Telegram bot stopped")
	}

	// Close database connections
	if err := db.Close(); err != nil {
		log.Error("Error closing database", map[string]string{
			"error": err.Error(),
		})
	}

	log.Info("Server exited successfully")
	fmt.Println("✅ Server shutdown complete")
}

// broadcastSystemStats sends system statistics to all connected clients
func broadcastSystemStats(hub *websocket.Hub) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Get database stats
		db := database.GetDatabase()
		activeUsers := db.CountActiveUsers()
		activeSessions := db.CountActiveSessions()

		// In a real implementation, you would get actual system metrics
		// For now, we'll use placeholder values
		hub.BroadcastStats(
			activeUsers,
			activeSessions,
			0.0,  // Memory usage (would use actual metrics)
			0.0,  // CPU usage (would use actual metrics)
			50,   // Network latency ms (would measure actual)
		)
	}
}
