package billing

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type RatingCoordinator interface {
	AcquireAPIKeyRating(context.Context, time.Duration) (func(), bool, error)
}

type RatingWorker struct {
	service           *Service
	coordinator       RatingCoordinator
	logger            *slog.Logger
	interval, timeout time.Duration
	pageSize          int32
	cancel            context.CancelFunc
	done              chan struct{}
	closeOnce         sync.Once
}

func NewRatingWorker(service *Service, coordinator RatingCoordinator, logger *slog.Logger,
	interval, timeout time.Duration, pageSize int32,
) (*RatingWorker, error) {
	if service == nil || coordinator == nil || interval <= 0 || timeout <= 0 || pageSize <= 0 || pageSize > 5000 {
		return nil, errors.New("invalid api key rating worker options")
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := &RatingWorker{
		service: service, coordinator: coordinator, logger: logger, interval: interval,
		timeout: timeout, pageSize: pageSize, cancel: cancel, done: make(chan struct{}),
	}
	go worker.run(ctx)
	return worker, nil
}

func (w *RatingWorker) run(ctx context.Context) {
	defer close(w.done)
	w.rate(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.rate(ctx)
		}
	}
}

func (w *RatingWorker) rate(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	defer cancel()
	startedAt := time.Now()
	release, acquired, err := w.coordinator.AcquireAPIKeyRating(ctx, settlementLeaseTTL(w.timeout))
	if err != nil {
		if parent.Err() == nil {
			w.logger.Error("api key usage rating lock failed", "error_code", "API_KEY_RATING_LOCK_FAILED")
		}
		return
	}
	if !acquired {
		w.logger.Debug("api key usage rating skipped", "reason", "scheduler_lock_not_acquired")
		return
	}
	defer release()
	summary, err := w.service.RatePendingAPIKeyUsage(ctx, w.pageSize)
	if err != nil {
		if parent.Err() == nil {
			w.logger.Error("api key usage rating failed", "error_code", "API_KEY_RATING_FAILED")
		}
		return
	}
	if summary.Created > 0 {
		w.logger.Info("api key usage rating completed", "created", summary.Created,
			"skipped", summary.Skipped, "duration_ms", time.Since(startedAt).Milliseconds())
	} else {
		w.logger.Debug("api key usage rating completed", "created", 0,
			"skipped", summary.Skipped, "duration_ms", time.Since(startedAt).Milliseconds())
	}
}

func (w *RatingWorker) Close(ctx context.Context) error {
	w.closeOnce.Do(w.cancel)
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
