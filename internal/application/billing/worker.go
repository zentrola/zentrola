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
	// 订阅与 API Key 结算互不依赖：任一方全局失败只记录日志，另一方仍继续结算。
	subscriptionSummary, subscriptionErr := w.service.SettleDueSubscriptions(ctx, cutoff)
	if subscriptionErr != nil && parent.Err() == nil {
		w.logger.Error("subscription billing settlement failed", "error_code", "SUBSCRIPTION_BILLING_FAILED",
			"error", SafeErrorText(subscriptionErr))
	}
	if parent.Err() != nil {
		return
	}
	apiKeySummary, apiKeyErr := w.service.SettleDueAPIKeys(ctx, cutoff)
	if apiKeyErr != nil && parent.Err() == nil {
		w.logger.Error("api key billing settlement failed", "error_code", "API_KEY_BILLING_FAILED",
			"error", SafeErrorText(apiKeyErr))
	}
	summary := Summary{
		Created:  subscriptionSummary.Created + apiKeySummary.Created,
		Adjusted: subscriptionSummary.Adjusted + apiKeySummary.Adjusted,
		Skipped:  subscriptionSummary.Skipped + apiKeySummary.Skipped,
		Failed:   subscriptionSummary.Failed + apiKeySummary.Failed,
	}
	samples := append(append([]string(nil), subscriptionSummary.FailureSamples...), apiKeySummary.FailureSamples...)
	if len(samples) > maxFailureSamples {
		samples = samples[:maxFailureSamples]
	}
	switch {
	case summary.Failed > 0:
		w.logger.Warn("billing settlement completed with failures",
			"error_code", "BILLING_SETTLEMENT_PARTIAL_FAILURE",
			"created", summary.Created, "adjusted", summary.Adjusted, "skipped", summary.Skipped,
			"failed", summary.Failed, "failure_samples", samples, "duration_ms", time.Since(startedAt).Milliseconds())
	case summary.Created > 0 || summary.Adjusted > 0:
		w.logger.Info("billing settlement completed",
			"created", summary.Created, "adjusted", summary.Adjusted, "skipped", summary.Skipped,
			"failed", 0, "duration_ms", time.Since(startedAt).Milliseconds())
	default:
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
