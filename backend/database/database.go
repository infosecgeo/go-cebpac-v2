package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/logger"
)

const (
	UsersCollection        = "users"
	LicensesCollection     = "licenses"
	TransactionsCollection = "transactions"
	CreditsCollection      = "credits"
	LogsCollection         = "logs"
	SessionsCollection     = "sessions"
	ProxiesCollection      = "proxies"
)

var (
	ErrCollectionUnsupported = errors.New("unsupported collection")
	ErrRecordNotFound        = errors.New("record not found")
	ErrRecordExists          = errors.New("record already exists")
)

var supportedCollections = map[string]string{
	UsersCollection:        "users.json",
	LicensesCollection:     "licenses.json",
	TransactionsCollection: "transactions.json",
	CreditsCollection:      "credits.json",
	LogsCollection:         "logs.json",
	SessionsCollection:     "sessions.json",
	ProxiesCollection:      "proxies.json",
}

// JSONDatabase manages JSON-backed collections with locking, backups, and recovery.
type JSONDatabase struct {
	cfg           *config.Config
	logger        *logger.Logger
	basePath      string
	backupPath    string
	backupEnabled bool
	lockTimeout   time.Duration
	mu            sync.RWMutex
	locks         map[string]*sync.RWMutex
}

// Repository exposes generic CRUD operations for a collection.
type Repository[T any] struct {
	db          *JSONDatabase
	collection  string
	keySelector func(T) string
}

// NewJSONDatabase initializes the file-backed database and ensures all supported files exist.
func NewJSONDatabase(cfg *config.Config) (*JSONDatabase, error) {
	if cfg == nil {
		cfg = config.GetConfig()
	}

	db := &JSONDatabase{
		cfg:           cfg,
		logger:        logger.GetLogger(),
		basePath:      cfg.Database.StoragePath,
		backupPath:    filepath.Join(cfg.Database.StoragePath, "backups"),
		backupEnabled: cfg.Database.BackupEnabled,
		lockTimeout:   10 * time.Second,
		locks:         make(map[string]*sync.RWMutex, len(supportedCollections)),
	}

	for collection := range supportedCollections {
		db.locks[collection] = &sync.RWMutex{}
	}

	if err := db.EnsureCollections(context.Background()); err != nil {
		return nil, err
	}

	return db, nil
}

// EnsureCollections creates the storage and backup paths and initializes known files.
func (db *JSONDatabase) EnsureCollections(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(db.basePath, 0o755); err != nil {
		return fmt.Errorf("create storage path: %w", err)
	}
	if db.backupEnabled {
		if err := os.MkdirAll(db.backupPath, 0o755); err != nil {
			return fmt.Errorf("create backup path: %w", err)
		}
	}
	for collection := range supportedCollections {
		if err := db.ensureCollectionFile(ctx, collection); err != nil {
			return err
		}
	}
	return nil
}

// NewRepository returns a typed repository for a supported collection.
func NewRepository[T any](db *JSONDatabase, collection string, keySelector func(T) string) (*Repository[T], error) {
	if _, err := db.collectionPath(collection); err != nil {
		return nil, err
	}
	if keySelector == nil {
		return nil, errors.New("key selector is required")
	}
	return &Repository[T]{db: db, collection: collection, keySelector: keySelector}, nil
}

// Users returns the typed users repository.
func (db *JSONDatabase) Users() (*Repository[User], error) {
	return NewRepository(db, UsersCollection, func(user User) string { return user.ID })
}

// Licenses returns the typed licenses repository.
func (db *JSONDatabase) Licenses() (*Repository[License], error) {
	return NewRepository(db, LicensesCollection, func(license License) string { return license.ID })
}

// Transactions returns the typed transactions repository.
func (db *JSONDatabase) Transactions() (*Repository[Transaction], error) {
	return NewRepository(db, TransactionsCollection, func(transaction Transaction) string { return transaction.ID })
}

// Credits returns the typed credits repository.
func (db *JSONDatabase) Credits() (*Repository[Credit], error) {
	return NewRepository(db, CreditsCollection, func(credit Credit) string { return credit.UserID })
}

// Sessions returns the typed sessions repository.
func (db *JSONDatabase) Sessions() (*Repository[Session], error) {
	return NewRepository(db, SessionsCollection, func(session Session) string { return session.ID })
}

// Logs returns the typed logs repository.
func (db *JSONDatabase) Logs() (*Repository[AuditLog], error) {
	return NewRepository(db, LogsCollection, func(log AuditLog) string { return log.ID })
}

// Proxies returns the typed proxies repository.
func (db *JSONDatabase) Proxies() (*Repository[ProxyRecord], error) {
	return NewRepository(db, ProxiesCollection, func(proxy ProxyRecord) string { return proxy.ID })
}

// List loads every record from the repository.
func (r *Repository[T]) List(ctx context.Context) ([]T, error) {
	return readCollection[T](ctx, r.db, r.collection)
}

// Get returns a single record by repository key.
func (r *Repository[T]) Get(ctx context.Context, key string) (T, bool, error) {
	var zero T
	records, err := r.List(ctx)
	if err != nil {
		return zero, false, err
	}
	for _, record := range records {
		if r.keySelector(record) == key {
			return record, true, nil
		}
	}
	return zero, false, nil
}

// FindOne returns the first record matching the predicate.
func (r *Repository[T]) FindOne(ctx context.Context, predicate func(T) bool) (T, bool, error) {
	var zero T
	records, err := r.List(ctx)
	if err != nil {
		return zero, false, err
	}
	for _, record := range records {
		if predicate(record) {
			return record, true, nil
		}
	}
	return zero, false, nil
}

// Filter returns all records matching the predicate.
func (r *Repository[T]) Filter(ctx context.Context, predicate func(T) bool) ([]T, error) {
	records, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]T, 0, len(records))
	for _, record := range records {
		if predicate(record) {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

// Create inserts a new record and fails if the key already exists.
func (r *Repository[T]) Create(ctx context.Context, record T) error {
	return updateCollection[T](ctx, r.db, r.collection, func(records []T) ([]T, error) {
		key := r.keySelector(record)
		if key == "" {
			return nil, errors.New("record key cannot be empty")
		}
		for _, existing := range records {
			if r.keySelector(existing) == key {
				return nil, ErrRecordExists
			}
		}
		return append(records, record), nil
	})
}

// Upsert inserts or replaces a record by key.
func (r *Repository[T]) Upsert(ctx context.Context, record T) error {
	return updateCollection[T](ctx, r.db, r.collection, func(records []T) ([]T, error) {
		key := r.keySelector(record)
		if key == "" {
			return nil, errors.New("record key cannot be empty")
		}
		for index, existing := range records {
			if r.keySelector(existing) == key {
				records[index] = record
				return records, nil
			}
		}
		return append(records, record), nil
	})
}

// Update mutates a single record by key.
func (r *Repository[T]) Update(ctx context.Context, key string, mutate func(*T) error) (T, error) {
	var updated T
	err := updateCollection[T](ctx, r.db, r.collection, func(records []T) ([]T, error) {
		for index := range records {
			if r.keySelector(records[index]) != key {
				continue
			}
			candidate := records[index]
			if err := mutate(&candidate); err != nil {
				return nil, err
			}
			if r.keySelector(candidate) == "" {
				return nil, errors.New("updated record key cannot be empty")
			}
			records[index] = candidate
			updated = candidate
			return records, nil
		}
		return nil, ErrRecordNotFound
	})
	return updated, err
}

// Delete removes a record by key.
func (r *Repository[T]) Delete(ctx context.Context, key string) error {
	return updateCollection[T](ctx, r.db, r.collection, func(records []T) ([]T, error) {
		filtered := records[:0]
		found := false
		for _, record := range records {
			if r.keySelector(record) == key {
				found = true
				continue
			}
			filtered = append(filtered, record)
		}
		if !found {
			return nil, ErrRecordNotFound
		}
		return filtered, nil
	})
}

func (db *JSONDatabase) ensureCollectionFile(ctx context.Context, collection string) error {
	path, err := db.collectionPath(collection)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat collection %s: %w", collection, err)
	}
	return os.WriteFile(path, []byte("[]\n"), 0o644)
}

func (db *JSONDatabase) collectionPath(collection string) (string, error) {
	fileName, ok := supportedCollections[collection]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrCollectionUnsupported, collection)
	}
	return filepath.Join(db.basePath, fileName), nil
}

func readCollection[T any](ctx context.Context, db *JSONDatabase, collection string) ([]T, error) {
	path, err := db.collectionPath(collection)
	if err != nil {
		return nil, err
	}
	lock := db.collectionLock(collection)
	lock.RLock()
	defer lock.RUnlock()
	return readCollectionFile[T](ctx, db, collection, path)
}

func updateCollection[T any](ctx context.Context, db *JSONDatabase, collection string, mutate func([]T) ([]T, error)) error {
	path, err := db.collectionPath(collection)
	if err != nil {
		return err
	}
	lock := db.collectionLock(collection)
	lock.Lock()
	defer lock.Unlock()

	release, err := db.acquireFileLock(ctx, path)
	if err != nil {
		return err
	}
	defer release()

	records, err := readCollectionFile[T](ctx, db, collection, path)
	if err != nil {
		return err
	}
	updated, err := mutate(records)
	if err != nil {
		return err
	}
	return writeCollectionFile(ctx, db, collection, path, updated)
}

func readCollectionFile[T any](ctx context.Context, db *JSONDatabase, collection, path string) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []T{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", collection, err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return []T{}, nil
	}
	var records []T
	if err := json.Unmarshal(data, &records); err == nil {
		return records, nil
	}

	db.logger.Warn("Database corruption detected, attempting recovery", map[string]string{
		"collection": collection,
		"path":       path,
	})
	if recoverErr := db.recoverCollection(path); recoverErr != nil {
		return nil, fmt.Errorf("recover %s: %w", collection, recoverErr)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recovered %s: %w", collection, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []T{}, nil
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("unmarshal recovered %s: %w", collection, err)
	}
	return records, nil
}

func writeCollectionFile[T any](ctx context.Context, db *JSONDatabase, collection, path string, records []T) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", collection, err)
	}
	payload = append(payload, '\n')

	if db.backupEnabled {
		if err := db.createBackup(path); err != nil {
			return err
		}
	}

	tempPath := fmt.Sprintf("%s.tmp-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tempPath, payload, 0o644); err != nil {
		return fmt.Errorf("write temp %s: %w", collection, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("atomic rename %s: %w", collection, err)
	}
	return nil
}

func (db *JSONDatabase) createBackup(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) || info == nil || info.Size() == 0 {
			return nil
		}
		return fmt.Errorf("stat backup source: %w", err)
	}

	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer source.Close()

	backupName := fmt.Sprintf("%s.%s.bak", filepath.Base(path), time.Now().UTC().Format("20060102T150405.000000000Z"))
	backupFile := filepath.Join(db.backupPath, backupName)
	target, err := os.OpenFile(backupFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("copy backup: %w", err)
	}
	return nil
}

func (db *JSONDatabase) recoverCollection(path string) error {
	corruptPath := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano())
	if err := os.Rename(path, corruptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("archive corrupt file: %w", err)
	}

	backups, err := db.backupsFor(path)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return os.WriteFile(path, []byte("[]\n"), 0o644)
	}

	source, err := os.Open(backups[0])
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("restore backup target: %w", err)
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("restore backup copy: %w", err)
	}
	return nil
}

func (db *JSONDatabase) backupsFor(path string) ([]string, error) {
	entries, err := os.ReadDir(db.backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	prefix := filepath.Base(path) + "."
	matches := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".bak") {
			continue
		}
		matches = append(matches, filepath.Join(db.backupPath, entry.Name()))
	}
	sort.Slice(matches, func(i, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] > matches[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})
	return matches, nil
}

func (db *JSONDatabase) collectionLock(collection string) *sync.RWMutex {
	db.mu.RLock()
	lock, ok := db.locks[collection]
	db.mu.RUnlock()
	if ok {
		return lock
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if lock, ok = db.locks[collection]; ok {
		return lock
	}
	lock = &sync.RWMutex{}
	db.locks[collection] = lock
	return lock
}

func (db *JSONDatabase) acquireFileLock(ctx context.Context, path string) (func(), error) {
	lockPath := path + ".lock"
	deadline := time.Now().Add(db.lockTimeout)

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(file, "pid=%d\ncreated_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = file.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create lock file: %w", err)
		}

		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > db.lockTimeout {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out acquiring lock for %s", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
