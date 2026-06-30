package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"cebupac/backend/auth"
	"cebupac/backend/config"
	"cebupac/backend/database"
	
	"github.com/google/uuid"
)

func main() {
	fmt.Println("🚀 Initializing CebuPacific Payment Processor Database")
	fmt.Println("================================================")

	// Initialize config
	cfg := config.GetConfig()
	fmt.Println("✓ Config loaded")

	// Initialize database
	db, err := database.NewJSONDatabase(cfg)
	if err != nil {
		fmt.Printf("❌ Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Database initialized")

	// Ensure collections exist
	ctx := context.Background()
	if err := db.EnsureCollections(ctx); err != nil {
		fmt.Printf("❌ Failed to ensure collections: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Collections ensured")

	// Create admin user
	fmt.Println("\n📝 Creating default admin user...")
	
	passwordManager := auth.NewPasswordManager(cfg)
	hashedPassword, err := passwordManager.HashPassword(ctx, "Change-Me-123!")
	if err != nil {
		fmt.Printf("❌ Failed to hash password: %v\n", err)
		os.Exit(1)
	}

	adminUser := database.User{
		ID:           uuid.New().String(),
		Username:     "admin",
		PasswordHash: hashedPassword,
		LicenseKey:   "ADMIN-LICENSE-KEY",
		Credits:      1000,
		Role:         "admin",
		Status:       "active",
		CreatedAt:    time.Now(),
	}

	usersRepo, err := db.Users()
	if err != nil {
		fmt.Printf("❌ Failed to get users repository: %v\n", err)
		os.Exit(1)
	}

	// Check if admin already exists
	existingAdmin, found, err := usersRepo.FindOne(ctx, func(u database.User) bool {
		return u.Username == "admin"
	})
	if err != nil {
		fmt.Printf("❌ Failed to check for existing admin: %v\n", err)
		os.Exit(1)
	}

	if found {
		fmt.Printf("⚠️  Admin user already exists (ID: %s)\n", existingAdmin.ID)
		fmt.Println("   To reset password, delete the existing user first or use the password reset tool.")
		return
	}

	// Create admin user
	if err := usersRepo.Create(ctx, adminUser); err != nil {
		fmt.Printf("❌ Failed to create admin user: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Admin user created successfully!")
	fmt.Println("\n📋 Admin Credentials:")
	fmt.Println("   Username: admin")
	fmt.Println("   Password: Change-Me-123!")
	fmt.Println("   Credits:  1000")
	fmt.Println("\n⚠️  IMPORTANT: Change the admin password immediately after first login!")
	fmt.Println("\n🎉 Database initialization complete!")
}
