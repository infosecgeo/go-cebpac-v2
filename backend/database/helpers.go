package database

import (
	"cebupac/backend/config"
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	dbInstance *Database
	dbOnce     sync.Once
)

// Database wraps the JSON database with simpler methods
type Database struct {
	json *JSONDatabase
}

// GetDatabase returns the singleton database instance
func GetDatabase() *Database {
	dbOnce.Do(func() {
		cfg := config.GetConfig()
		json, err := NewJSONDatabase(cfg)
		if err != nil {
			panic(fmt.Sprintf("Failed to initialize database: %v", err))
		}
		
		// Ensure collections exist
		ctx := context.Background()
		if err := json.EnsureCollections(ctx); err != nil {
			panic(fmt.Sprintf("Failed to ensure collections: %v", err))
		}
		
		dbInstance = &Database{json: json}
	})
	return dbInstance
}

// User operations
func (db *Database) GetUser(userID string) (*User, error) {
	repo, err := db.json.Users()
	if err != nil {
		return nil, err
	}
	user, found, err := repo.Get(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("user not found")
	}
	return &user, nil
}

func (db *Database) GetUserByUsername(username string) (*User, error) {
	repo, err := db.json.Users()
	if err != nil {
		return nil, err
	}
	user, found, err := repo.FindOne(context.Background(), func(u User) bool {
		return u.Username == username
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("user not found")
	}
	return &user, nil
}

func (db *Database) CreateUser(user *User) error {
	repo, err := db.json.Users()
	if err != nil {
		return err
	}
	return repo.Create(context.Background(), *user)
}

func (db *Database) UpdateUser(user *User) error {
	repo, err := db.json.Users()
	if err != nil {
		return err
	}
	return repo.Upsert(context.Background(), *user)
}

func (db *Database) DeleteUser(userID string) error {
	repo, err := db.json.Users()
	if err != nil {
		return err
	}
	return repo.Delete(context.Background(), userID)
}

func (db *Database) GetAllUsers() ([]*User, error) {
	repo, err := db.json.Users()
	if err != nil {
		return nil, err
	}
	users, err := repo.List(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]*User, len(users))
	for i := range users {
		result[i] = &users[i]
	}
	return result, nil
}

// Session operations
func (db *Database) GetSession(sessionID string) (*Session, error) {
	repo, err := db.json.Sessions()
	if err != nil {
		return nil, err
	}
	session, found, err := repo.Get(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("session not found")
	}
	return &session, nil
}

func (db *Database) CreateSession(session *Session) error {
	repo, err := db.json.Sessions()
	if err != nil {
		return err
	}
	return repo.Create(context.Background(), *session)
}

func (db *Database) DeleteSession(sessionID string) error {
	repo, err := db.json.Sessions()
	if err != nil {
		return err
	}
	return repo.Delete(context.Background(), sessionID)
}

// Transaction operations
func (db *Database) CreateTransaction(tx *Transaction) error {
	repo, err := db.json.Transactions()
	if err != nil {
		return err
	}
	return repo.Create(context.Background(), *tx)
}

func (db *Database) GetTransaction(txID string) (*Transaction, error) {
	repo, err := db.json.Transactions()
	if err != nil {
		return nil, err
	}
	tx, found, err := repo.Get(context.Background(), txID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("transaction not found")
	}
	return &tx, nil
}

func (db *Database) GetTransactionsByUser(userID string) ([]*Transaction, error) {
	repo, err := db.json.Transactions()
	if err != nil {
		return nil, err
	}
	txs, err := repo.Filter(context.Background(), func(t Transaction) bool {
		return t.UserID == userID
	})
	if err != nil {
		return nil, err
	}
	result := make([]*Transaction, len(txs))
	for i := range txs {
		result[i] = &txs[i]
	}
	return result, nil
}

// Stats operations
func (db *Database) CountActiveUsers() int {
	repo, err := db.json.Users()
	if err != nil {
		return 0
	}
	users, err := repo.Filter(context.Background(), func(u User) bool {
		return u.Status == "active"
	})
	if err != nil {
		return 0
	}
	return len(users)
}

func (db *Database) CountActiveSessions() int {
	repo, err := db.json.Sessions()
	if err != nil {
		return 0
	}
	sessions, err := repo.Filter(context.Background(), func(s Session) bool {
		return s.ExpiresAt.After(time.Now())
	})
	if err != nil {
		return 0
	}
	return len(sessions)
}

// Close closes the database
func (db *Database) Close() error {
	// JSON database doesn't need explicit closing
	return nil
}

// StartBackupRoutine starts the automatic backup routine
func (db *Database) StartBackupRoutine() {
	cfg := config.GetConfig()
	interval := time.Duration(cfg.Database.BackupIntervalMin) * time.Minute
	
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			// Backup logic would go here
			// For now, the JSONDatabase handles backups automatically on write
		}
	}()
}

// Repository accessor methods
func (db *Database) Users() (*Repository[User], error) {
	return db.json.Users()
}

func (db *Database) Licenses() (*Repository[License], error) {
	return db.json.Licenses()
}

func (db *Database) AdminSettings() (*Repository[AdminSettings], error) {
	return db.json.AdminSettings()
}

func (db *Database) Proxies() (*Repository[ProxyRecord], error) {
	return db.json.Proxies()
}

func (db *Database) TopupRequests() (*Repository[TopupRequest], error) {
	return db.json.TopupRequests()
}

func (db *Database) Transactions() (*Repository[Transaction], error) {
	return db.json.Transactions()
}

func (db *Database) Sessions() (*Repository[Session], error) {
	return db.json.Sessions()
}

func (db *Database) Credits() (*Repository[Credit], error) {
	return db.json.Credits()
}

func (db *Database) Logs() (*Repository[AuditLog], error) {
	return db.json.Logs()
}
