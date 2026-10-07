package billing

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const settlementLeaseMargin = time.Minute

// SettlementCoordinator coordinates one settlement scan across application instances.
// The lease must fail closed when the coordination backend is unavailable.
type SettlementCoordinator interface {
	AcquireBillingSettlement(context.Context, time.Duration) (func(), bool, error)
}

type Worker struct {
	service                  *Service
	coordinator              SettlementCoordinator
	logger                   *slog.Logger
	interval, grace, timeout time.Duration
	cancel                   context.CancelFunc
	done                     chan struct{}
	closeOnce                sync.Once
}

func NewWorker(service *Service, coordinator SettlementCoordinator, logger *slog.Logger, interval, grace, timeout time.Duration) (*Worker, error) {
	if service == nil || coordinator == nil || interval <= 0 || grace < 0 || timeout <= 0 {
		return nil, errors.New("invalid billing worker options")
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := &Worker{
		service: service, coordinator: coordinator, logger: logger, interval: interval, grace: grace, timeout: timeout,
		cancel: cancel, done: make(chan struct{}),
	}
	go worker.run(ctx)
	return worker, nil
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	w.settle(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.settle(ctx)
		}
	}
}

func (w *Worker) settle(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	defer cancel()
	startedAt := time.Now()
	release, acquired, err := w.coordinator.AcquireBillingSettlement(ctx, settlementLeaseTTL(w.timeout))
	if err != nil {
		if parent.Err() == nil {
			w.logger.Error("billing settlement lock failed", "error_code", "BILLING_SETTLEMENT_LOCK_FAILED")
		}
		return
	}
	if !acquired {
		w.logger.Debug("billing settlement skipped", "reason", "scheduler_lock_not_acquired")
		return
	}
	defer release()
	cutoff := time.Now().UTC().Add(-w.grace)
	subscriptionSummary, err := w.service.SettleDueSubscriptions(ctx, cutoff)
	if err != nil {
		if parent.Err() == nil {
			w.logger.Error("subscription billing settlement failed", "error_code", "SUBSCRIPTION_BILLING_FAILED")
		}
		return
	}
	apiKeySummary, err := w.service.SettleDueAPIKeys(ctx, cutoff)
	if err != nil {
		if parent.Err() == nil {
			w.logger.Error("api key billing settlement failed", "error_code", "API_KEY_BILLING_FAILED")
		}
		return
	}
	summary := Summary{
		Created:  subscriptionSummary.Created + apiKeySummary.Created,
		Adjusted: subscriptionSummary.Adjusted + apiKeySummary.Adjusted,
		Skipped:  subscriptionSummary.Skipped + apiKeySummary.Skipped,
		Failed:   subscriptionSummary.Failed + apiKeySummary.Failed,
	}
	if summary.Created > 0 || summary.Adjusted > 0 || summary.Failed > 0 {
		w.logger.Info("billing settlement completed",
			"created", summary.Created, "adjusted", summary.Adjusted, "skipped", summary.Skipped,
			"failed", summary.Failed, "duration_ms", time.Since(startedAt).Milliseconds())
	} else {
		w.logger.Debug("billing settlement completed",
			"created", 0, "adjusted", 0, "skipped", summary.Skipped, "failed", 0,
			"duration_ms", time.Since(startedAt).Milliseconds())
	}
}

func settlementLeaseTTL(timeout time.Duration) time.Duration {
	if timeout > time.Duration(1<<63-1)-settlementLeaseMargin {
		return time.Duration(1<<63 - 1)
	}
	return timeout + settlementLeaseMargin
}

func (w *Worker) Close(ctx context.Context) error {
	w.closeOnce.Do(w.cancel)
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
