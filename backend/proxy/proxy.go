package proxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/logger"
)

// Manager rotates proxies using configuration and optional persisted proxy metadata.
type Manager struct {
	cfg      *config.Config
	logger   *logger.Logger
	repo     *database.Repository[database.ProxyRecord]
	mu       sync.Mutex
	proxies  []database.ProxyRecord
	index    int
	random   *rand.Rand
	cooldown time.Duration
}

// NewManager builds a proxy manager and loads proxy records immediately.
func NewManager(cfg *config.Config, db *database.JSONDatabase) (*Manager, error) {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	var repo *database.Repository[database.ProxyRecord]
	if db != nil {
		var err error
		repo, err = db.Proxies()
		if err != nil {
			return nil, err
		}
	}
	manager := &Manager{
		cfg:      cfg,
		logger:   logger.GetLogger(),
		repo:     repo,
		random:   rand.New(rand.NewSource(time.Now().UnixNano())),
		cooldown: 2 * time.Minute,
	}
	if err := manager.Load(context.Background()); err != nil {
		return nil, err
	}
	return manager, nil
}

// Load refreshes the effective proxy list from config and the JSON database.
func (m *Manager) Load(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	merged := make([]database.ProxyRecord, 0, len(m.cfg.Proxy.ProxyList))
	seen := make(map[string]struct{})
	for index, address := range m.cfg.Proxy.ProxyList {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		seen[address] = struct{}{}
		merged = append(merged, database.ProxyRecord{
			ID:        fmt.Sprintf("config-%d", index),
			Address:   address,
			Enabled:   true,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
	}
	if m.repo != nil {
		records, err := m.repo.List(ctx)
		if err != nil {
			return err
		}
		for _, record := range records {
			if _, exists := seen[record.Address]; exists {
				continue
			}
			merged = append(merged, record)
		}
	}
	m.proxies = merged
	if m.index >= len(m.proxies) {
		m.index = 0
	}
	return nil
}

// NextProxy returns the next eligible proxy record according to rotation policy.
func (m *Manager) NextProxy(ctx context.Context) (database.ProxyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nextProxyLocked(ctx)
}

// MarkFailed records a proxy failure and updates persisted health metadata when available.
func (m *Manager) MarkFailed(ctx context.Context, proxyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.proxies {
		if m.proxies[index].ID != proxyID {
			continue
		}
		m.proxies[index].FailCount++
		m.proxies[index].LastFailedAt = time.Now().UTC()
		m.proxies[index].UpdatedAt = time.Now().UTC()
		m.persistProxyState(ctx, m.proxies[index])
		return
	}
}

// MarkSucceeded clears transient failure counters for a proxy.
func (m *Manager) MarkSucceeded(ctx context.Context, proxyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.proxies {
		if m.proxies[index].ID != proxyID {
			continue
		}
		m.proxies[index].FailCount = 0
		m.proxies[index].UpdatedAt = time.Now().UTC()
		m.persistProxyState(ctx, m.proxies[index])
		return
	}
}

// AvailableCount reports the number of configured proxies.
func (m *Manager) AvailableCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.proxies)
}

// WithRetry executes a callback and automatically retries against subsequent proxies.
func (m *Manager) WithRetry(ctx context.Context, attempts int, fn func(database.ProxyRecord) error) error {
	if attempts <= 0 {
		attempts = len(m.proxies)
		if attempts == 0 {
			attempts = 1
		}
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		proxyRecord, err := m.NextProxy(ctx)
		if err != nil {
			return err
		}
		if err := fn(proxyRecord); err != nil {
			lastErr = err
			if proxyRecord.ID != "" {
				m.MarkFailed(ctx, proxyRecord.ID)
			}
			continue
		}
		if proxyRecord.ID != "" {
			m.MarkSucceeded(ctx, proxyRecord.ID)
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("proxy retry attempts exhausted")
	}
	return lastErr
}

func (m *Manager) nextProxyLocked(ctx context.Context) (database.ProxyRecord, error) {
	if err := ctx.Err(); err != nil {
		return database.ProxyRecord{}, err
	}
	if !m.cfg.Proxy.Enabled || len(m.proxies) == 0 {
		return database.ProxyRecord{}, nil
	}
	eligible := make([]database.ProxyRecord, 0, len(m.proxies))
	now := time.Now().UTC()
	for _, record := range m.proxies {
		if !record.Enabled {
			continue
		}
		if !record.LastFailedAt.IsZero() && now.Sub(record.LastFailedAt) < m.cooldown {
			continue
		}
		eligible = append(eligible, record)
	}
	if len(eligible) == 0 {
		eligible = append(eligible, m.proxies...)
	}
	if strings.EqualFold(m.cfg.Proxy.RotationMode, "random") {
		return eligible[m.random.Intn(len(eligible))], nil
	}
	record := eligible[m.index%len(eligible)]
	m.index = (m.index + 1) % len(eligible)
	return record, nil
}

func (m *Manager) persistProxyState(ctx context.Context, record database.ProxyRecord) {
	if m.repo == nil || record.ID == "" || strings.HasPrefix(record.ID, "config-") {
		return
	}
	if err := m.repo.Upsert(ctx, record); err != nil {
		m.logger.Warn("Failed to persist proxy state", map[string]string{
			"proxy_id": record.ID,
			"error":    err.Error(),
		})
	}
}
