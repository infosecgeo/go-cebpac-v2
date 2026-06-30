package workers

import (
	"cebupac/backend/config"
	"context"
	"sync"
)

var (
	poolInstance *Pool
	poolOnce     sync.Once
)

// GetPool returns the singleton worker pool instance
func GetPool() *Pool {
	poolOnce.Do(func() {
		cfg := config.GetConfig()
		ctx := context.Background()
		poolInstance = NewPool(ctx, cfg, cfg.Workers.PoolSize, cfg.Workers.QueueSize)
		poolInstance.Start()
	})
	return poolInstance
}
