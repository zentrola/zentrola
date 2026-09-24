package gateway

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

const subscriptionRefreshPageSize int32 = 100

type SubscriptionRefreshFailure struct {
	ResourceID int64
	ErrorCode  string
}

type SubscriptionRefreshSummary struct {
	Checked, Refreshed int
	Failures           []SubscriptionRefreshFailure
	Skipped            bool
}

// RefreshSubscriptions performs a synchronous refresh. The background worker
// uses the bounded variant below so one slow credential cannot serialize the
// whole scan.
func (s *Service) RefreshSubscriptions(ctx context.Context) (SubscriptionRefreshSummary, error) {
	return s.refreshSubscriptions(ctx, 1, 0)
}

func (s *Service) refreshSubscriptions(ctx context.Context, concurrency int, credentialTimeout time.Duration) (SubscriptionRefreshSummary, error) {
	lister, ok := s.store.(SubscriptionCredentialLister)
	if !ok || len(s.refresh) == 0 {
		return SubscriptionRefreshSummary{}, ErrUnavailable
	}
	if concurrency < 1 {
		concurrency = 1
	}

	var summary SubscriptionRefreshSummary
	var releaseScheduler func()
	if coordinator := s.refreshCoordinator; coordinator != nil {
		var acquired bool
		var err error
		releaseScheduler, acquired, err = coordinator.AcquireSubscriptionRefreshScheduler(ctx)
		if err != nil {
			// Redis is the coordination source of truth for the background path;
			// fail closed and let the next scheduled scan retry.
			summary.Skipped = true
			return summary, nil
		}
		if !acquired {
			summary.Skipped = true
			return summary, nil
		}
	}

	var candidates []SubscriptionCredential
	var after int64
	refreshBefore := time.Now().UTC()
	for _, refresh := range s.refresh {
		if planner, ok := refresh.(SubscriptionRefreshPlanner); ok {
			candidate := planner.RefreshBefore(time.Now().UTC())
			if candidate.After(refreshBefore) {
				refreshBefore = candidate
			}
		}
	}
	for {
		page, err := lister.ListSubscriptionCredentials(ctx, after, subscriptionRefreshPageSize, refreshBefore)
		if err != nil {
			if releaseScheduler != nil {
				releaseScheduler()
			}
			return summary, ErrUnavailable
		}
		candidates = append(candidates, page...)
		if len(page) < int(subscriptionRefreshPageSize) {
			break
		}
		next := page[len(page)-1].ResourceID
		if next <= after {
			if releaseScheduler != nil {
				releaseScheduler()
			}
			return summary, errors.New("subscription refresh pagination did not advance")
		}
		after = next
	}
	// Do not hold the scheduler lease while network calls are in flight. The
	// resource leases below prevent duplicate work when the next scan starts.
	if releaseScheduler != nil {
		releaseScheduler()
	}
	return s.processSubscriptionCandidates(ctx, candidates, concurrency, credentialTimeout, summary)
}

type subscriptionRefreshResult struct {
	checked   bool
	refreshed bool
	failure   *SubscriptionRefreshFailure
}

func (s *Service) processSubscriptionCandidates(ctx context.Context, candidates []SubscriptionCredential, concurrency int, credentialTimeout time.Duration, summary SubscriptionRefreshSummary) (SubscriptionRefreshSummary, error) {
	jobs := make(chan SubscriptionCredential)
	results := make(chan subscriptionRefreshResult, len(candidates))
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for candidate := range jobs {
				results <- s.refreshSubscriptionCandidate(ctx, candidate, credentialTimeout)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case jobs <- candidate:
			case <-ctx.Done():
				return
			}
		}
	}()
	workers.Wait()
	close(results)
	for result := range results {
		if result.checked {
			summary.Checked++
		}
		if result.refreshed {
			summary.Refreshed++
		}
		if result.failure != nil {
			summary.Failures = append(summary.Failures, *result.failure)
		}
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	return summary, nil
}

func (s *Service) refreshSubscriptionCandidate(parent context.Context, candidate SubscriptionCredential, credentialTimeout time.Duration) subscriptionRefreshResult {
	refresh := s.subscriptionRefresher(candidate.AuthAdapter)
	if refresh == nil {
		return subscriptionRefreshResult{}
	}
	result := subscriptionRefreshResult{checked: true}
	ctx := parent
	if credentialTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, credentialTimeout)
		defer cancel()
	}
	if coordinator := s.refreshCoordinator; coordinator != nil {
		release, acquired, err := coordinator.AcquireSubscriptionRefreshResource(ctx, candidate.ResourceID)
		if err != nil || !acquired {
			return result
		}
		defer release()
		ctx = withSubscriptionRefreshLease(ctx)
	}
	route := Route{
		ProviderID: candidate.ProviderID, ResourceID: candidate.ResourceID,
		AuthType: "SUBSCRIPTION", AuthAdapter: candidate.AuthAdapter,
		CredentialRefreshedAt: candidate.CredentialRefreshedAt,
		CredentialExpiresAt:   candidate.CredentialExpiresAt,
		ProxyEnabled:          candidate.ProxyEnabled, ProxyURL: candidate.ProxyURL,
		ProxyHeaders: candidate.ProxyHeaders,
	}
	credential, err := s.cipher.Decrypt(candidate.Credential, catalog.CredentialOwner{ProviderID: candidate.ProviderID, ResourceID: candidate.ResourceID})
	if err != nil {
		result.failure = &SubscriptionRefreshFailure{ResourceID: candidate.ResourceID, ErrorCode: ErrCredential.Code}
		return result
	}
	route.Proxy, err = s.decryptProxy(route)
	if err != nil {
		clear(credential)
		result.failure = &SubscriptionRefreshFailure{ResourceID: candidate.ResourceID, ErrorCode: ErrProxy.Code}
		return result
	}
	if route.CredentialExpiresAt == nil {
		if inspector, ok := refresh.(SubscriptionRefreshMetadataInspector); ok {
			refreshedAt, expiresAt, inspectErr := inspector.CredentialRefreshMetadata(credential)
			if inspectErr == nil && expiresAt != nil {
				route.CredentialRefreshedAt, route.CredentialExpiresAt = refreshedAt, expiresAt
				if updater, ok := s.store.(CredentialRefreshMetadataUpdater); ok {
					_ = updater.UpdateResourceCredentialRefreshMetadata(ctx, route)
				}
			}
		}
	}
	updated, changed, refreshErr := s.refreshCredential(ctx, route, credential)
	clear(updated)
	if refreshErr != nil {
		code := ErrSubscription.Code
		var classified *SubscriptionRefreshError
		if errors.As(refreshErr, &classified) && classified.Code != "" {
			code = classified.Code
		}
		if errors.As(refreshErr, &classified) && classified.Permanent {
			s.blockSystemResource(ctx, candidate.ResourceID, ResourceBlock{Reason: subscriptionRefreshBlockReason(classified.Code), ErrorCode: classified.Code})
		}
		result.failure = &SubscriptionRefreshFailure{ResourceID: candidate.ResourceID, ErrorCode: code}
		return result
	}
	result.refreshed = changed
	return result
}

type SubscriptionRefreshWorkerOption func(*SubscriptionRefreshWorker)

func WithSubscriptionRefreshConcurrency(concurrency int) SubscriptionRefreshWorkerOption {
	return func(worker *SubscriptionRefreshWorker) {
		if concurrency > 0 {
			worker.concurrency = concurrency
		}
	}
}

func WithSubscriptionRefreshCredentialTimeout(timeout time.Duration) SubscriptionRefreshWorkerOption {
	return func(worker *SubscriptionRefreshWorker) {
		if timeout > 0 {
			worker.credentialTimeout = timeout
		}
	}
}

type SubscriptionRefreshWorker struct {
	service           *Service
	logger            *slog.Logger
	interval          time.Duration
	runTimeout        time.Duration
	concurrency       int
	credentialTimeout time.Duration
	cancel            context.CancelFunc
	done              chan struct{}
	closeOnce         sync.Once
}

func NewSubscriptionRefreshWorker(service *Service, logger *slog.Logger, interval, runTimeout time.Duration, options ...SubscriptionRefreshWorkerOption) (*SubscriptionRefreshWorker, error) {
	if service == nil || interval <= 0 || runTimeout <= 0 {
		return nil, errors.New("invalid subscription refresh worker options")
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := &SubscriptionRefreshWorker{
		service: service, logger: logger, interval: interval, runTimeout: runTimeout,
		concurrency: 1, credentialTimeout: 45 * time.Second,
		cancel: cancel, done: make(chan struct{}),
	}
	for _, option := range options {
		option(worker)
	}
	go worker.run(ctx)
	return worker, nil
}

func (w *SubscriptionRefreshWorker) run(ctx context.Context) {
	defer close(w.done)
	w.refresh(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.refresh(ctx)
		}
	}
}

func (w *SubscriptionRefreshWorker) refresh(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, w.runTimeout)
	defer cancel()
	startedAt := time.Now()
	summary, err := w.service.refreshSubscriptions(ctx, w.concurrency, w.credentialTimeout)
	if err != nil {
		if parent.Err() == nil {
			w.logger.Error("subscription refresh scan failed", "error_code", "SUBSCRIPTION_REFRESH_SCAN_FAILED")
		}
		return
	}
	if summary.Skipped {
		w.logger.Debug("subscription refresh scan skipped", "reason", "scheduler_lock_not_acquired")
		return
	}
	for _, failure := range summary.Failures {
		w.logger.Warn("subscription credential refresh failed", "error_code", failure.ErrorCode, "resource_id", failure.ResourceID)
	}
	if summary.Refreshed > 0 || len(summary.Failures) > 0 {
		w.logger.Info("subscription refresh scan completed", "checked", summary.Checked, "refreshed", summary.Refreshed, "failed", len(summary.Failures), "duration_ms", time.Since(startedAt).Milliseconds())
	} else {
		w.logger.Debug("subscription refresh scan completed", "checked", summary.Checked, "refreshed", 0, "failed", 0, "duration_ms", time.Since(startedAt).Milliseconds())
	}
}

func (w *SubscriptionRefreshWorker) Close(ctx context.Context) error {
	w.closeOnce.Do(w.cancel)
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
