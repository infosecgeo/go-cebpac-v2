package services

import (
	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/proxy"
	"fmt"
	"sync"
)

var (
	paymentServiceInstance *PaymentService
	paymentServiceOnce     sync.Once
	akamaiServiceInstance  *AkamaiService
	akamaiServiceOnce      sync.Once
	proxyManagerInstance   *proxy.Manager
	proxyManagerOnce       sync.Once
)

// GetProxyManager returns the singleton proxy manager instance
func GetProxyManager() *proxy.Manager {
	proxyManagerOnce.Do(func() {
		cfg := config.GetConfig()
		
		// Get database
		json, err := database.NewJSONDatabase(cfg)
		if err != nil {
			panic(fmt.Sprintf("Failed to initialize database for proxy manager: %v", err))
		}
		
		// Create proxy manager
		proxyManagerInstance, err = proxy.NewManager(cfg, json)
		if err != nil {
			panic(fmt.Sprintf("Failed to initialize proxy manager: %v", err))
		}
	})
	return proxyManagerInstance
}

// GetAkamaiService returns the singleton Akamai service instance
func GetAkamaiService() *AkamaiService {
	akamaiServiceOnce.Do(func() {
		cfg := config.GetConfig()
		proxyManager := GetProxyManager()
		akamaiServiceInstance = NewAkamaiService(cfg, proxyManager)
	})
	return akamaiServiceInstance
}

// GetPaymentService returns the singleton payment service instance
func GetPaymentService() *PaymentService {
	paymentServiceOnce.Do(func() {
		cfg := config.GetConfig()
		
		// Get database
		json, err := database.NewJSONDatabase(cfg)
		if err != nil {
			panic(fmt.Sprintf("Failed to initialize database for payment service: %v", err))
		}
		
		// Get Akamai service
		akamaiService := GetAkamaiService()
		
		// Create payment service
		paymentServiceInstance, err = NewPaymentService(cfg, json, akamaiService)
		if err != nil {
			panic(fmt.Sprintf("Failed to initialize payment service: %v", err))
		}
	})
	return paymentServiceInstance
}
