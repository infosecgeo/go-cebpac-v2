package workers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/logger"
)

// Task is the unit of work executed by the worker pool.
type Task interface {
	ID() string
	Execute(context.Context) (any, error)
}

// TaskResult records task completion metadata and output.
type TaskResult struct {
	TaskID      string    `json:"taskID"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	Attempts    int       `json:"attempts"`
	Value       any       `json:"value,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type taskRequest struct {
	task     Task
	attempts int
}

// Pool executes submitted tasks with configurable concurrency and graceful shutdown.
type Pool struct {
	cfg     *config.Config
	logger  *logger.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	workers int
	queue   chan taskRequest
	wg      sync.WaitGroup
	mu      sync.RWMutex
	results map[string]TaskResult
	started bool
	stopped bool
}

// NewPool constructs a worker pool using config defaults when needed.
func NewPool(parent context.Context, cfg *config.Config, poolSize, queueSize int) *Pool {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	if poolSize <= 0 {
		poolSize = cfg.Workers.PoolSize
	}
	if poolSize <= 0 {
		poolSize = 1
	}
	if queueSize <= 0 {
		queueSize = cfg.Workers.QueueSize
	}
	if queueSize <= 0 {
		queueSize = poolSize * 4
	}
	return &Pool{
		cfg:     cfg,
		logger:  logger.GetLogger(),
		ctx:     ctx,
		cancel:  cancel,
		workers: poolSize,
		queue:   make(chan taskRequest, queueSize),
		results: make(map[string]TaskResult),
	}
}

// Start launches worker goroutines once.
func (p *Pool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	p.started = true
	for index := 0; index < p.workers; index++ {
		p.wg.Add(1)
		go p.runWorker(index)
	}
}

// Submit queues a task for asynchronous execution.
func (p *Pool) Submit(ctx context.Context, task Task) error {
	if task == nil {
		return errors.New("task is required")
	}
	p.mu.RLock()
	stopped := p.stopped
	p.mu.RUnlock()
	if stopped {
		return errors.New("worker pool is shut down")
	}
	p.Start()
	request := taskRequest{task: task, attempts: 1}
	select {
	case <-p.ctx.Done():
		return errors.New("worker pool is shutting down")
	case <-ctx.Done():
		return ctx.Err()
	case p.queue <- request:
		return nil
	}
}

// GetResult returns the stored task result when available.
func (p *Pool) GetResult(taskID string) (TaskResult, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result, ok := p.results[taskID]
	return result, ok
}

// Results returns a snapshot of all tracked task results.
func (p *Pool) Results() []TaskResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	results := make([]TaskResult, 0, len(p.results))
	for _, result := range p.results {
		results = append(results, result)
	}
	return results
}

// Shutdown stops intake, waits for workers, and respects the supplied context.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	p.mu.Unlock()
	p.cancel()
	close(p.queue)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (p *Pool) runWorker(index int) {
	defer p.wg.Done()
	for request := range p.queue {
		if request.task == nil {
			continue
		}
		startedAt := time.Now().UTC()
		workerCtx, cancel := p.contextWithTimeout()
		resultValue, err := request.task.Execute(workerCtx)
		cancel()
		result := TaskResult{
			TaskID:      request.task.ID(),
			StartedAt:   startedAt,
			CompletedAt: time.Now().UTC(),
			Attempts:    request.attempts,
			Value:       resultValue,
		}
		if err != nil {
			result.Error = err.Error()
			p.logger.Error("Worker task failed", map[string]string{"worker": fmt.Sprintf("%d", index), "task_id": request.task.ID(), "error": err.Error()})
		}
		p.mu.Lock()
		p.results[result.TaskID] = result
		p.mu.Unlock()
	}
}

func (p *Pool) contextWithTimeout() (context.Context, context.CancelFunc) {
	timeout := time.Duration(p.cfg.Workers.TaskTimeoutSeconds) * time.Second
	if timeout <= 0 {
		return p.ctx, func() {}
	}
	return context.WithTimeout(p.ctx, timeout)
}
