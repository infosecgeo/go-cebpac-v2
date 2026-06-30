package workers

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/services"
)

// PaymentProcessor is implemented by the payment service.
type PaymentProcessor interface {
	ProcessPayment(context.Context, services.PaymentRequest, services.ProgressTracker) (*services.PaymentResult, error)
}

// PaymentTask runs a payment request with worker-friendly retry and callbacks.
type PaymentTask struct {
	TaskID         string
	Request        services.PaymentRequest
	Processor      PaymentProcessor
	Tracker        services.ProgressTracker
	OnProgress     func(services.PaymentProgress)
	OnSuccess      func(*services.PaymentResult)
	OnFailure      func(error)
	MaxAttempts    int
	InitialBackoff time.Duration
	cfg            *config.Config
	random         *rand.Rand
}

// NewPaymentTask creates a payment task with sane retry defaults.
func NewPaymentTask(cfg *config.Config, taskID string, processor PaymentProcessor, request services.PaymentRequest) *PaymentTask {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	return &PaymentTask{
		TaskID:         taskID,
		Request:        request,
		Processor:      processor,
		MaxAttempts:    maxInt(1, cfg.Retry.MaxAttempts),
		InitialBackoff: time.Duration(maxInt(500, cfg.Retry.InitialDelayMs)) * time.Millisecond,
		cfg:            cfg,
		random:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ID returns the worker task identifier.
func (t *PaymentTask) ID() string {
	return t.TaskID
}

// Execute processes a payment with retry, progress callbacks, and success/failure hooks.
func (t *PaymentTask) Execute(ctx context.Context) (any, error) {
	if t.Processor == nil {
		return nil, fmt.Errorf("payment processor is required")
	}
	tracker := t.Tracker
	if tracker == nil && t.OnProgress != nil {
		tracker = progressCallback(func(progress services.PaymentProgress) {
			t.OnProgress(progress)
		})
	} else if tracker != nil && t.OnProgress != nil {
		tracker = chainedTracker{primary: tracker, secondary: progressCallback(t.OnProgress)}
	}

	var lastErr error
	for attempt := 1; attempt <= maxInt(1, t.MaxAttempts); attempt++ {
		result, err := t.Processor.ProcessPayment(ctx, t.Request, tracker)
		if err == nil && result != nil && result.Success {
			if t.OnSuccess != nil {
				t.OnSuccess(result)
			}
			return result, nil
		}
		if err != nil {
			lastErr = err
		} else if result != nil {
			lastErr = fmt.Errorf("%s", result.Message)
		}
		if attempt >= t.MaxAttempts {
			break
		}
		if err := sleepWithContext(ctx, t.backoff(attempt)); err != nil {
			lastErr = err
			break
		}
	}
	if t.OnFailure != nil && lastErr != nil {
		t.OnFailure(lastErr)
	}
	return nil, lastErr
}

func (t *PaymentTask) backoff(attempt int) time.Duration {
	multiplier := float64(maxInt(2, t.cfg.Retry.BackoffMultiplier))
	maxDelay := time.Duration(maxInt(1000, t.cfg.Retry.MaxDelayMs)) * time.Millisecond
	base := float64(t.InitialBackoff) * math.Pow(multiplier, float64(attempt-1))
	delay := time.Duration(base)
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay/2 + time.Duration(t.random.Int63n(int64(delay/2)+1))
}

type progressCallback func(services.PaymentProgress)

func (p progressCallback) Publish(_ context.Context, progress services.PaymentProgress) error {
	p(progress)
	return nil
}

type chainedTracker struct {
	primary   services.ProgressTracker
	secondary services.ProgressTracker
}

func (c chainedTracker) Publish(ctx context.Context, progress services.PaymentProgress) error {
	if c.primary != nil {
		if err := c.primary.Publish(ctx, progress); err != nil {
			return err
		}
	}
	if c.secondary != nil {
		return c.secondary.Publish(ctx, progress)
	}
	return nil
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
