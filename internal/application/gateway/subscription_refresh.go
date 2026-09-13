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
}

// RefreshSubscriptions 扫描启用的个人订阅，并复用请求链路的刷新、加密与缓存失效逻辑。
func (s *Service) RefreshSubscriptions(ctx context.Context) (SubscriptionRefreshSummary, error) {
	lister, ok := s.store.(SubscriptionCredentialLister)
	if !ok || len(s.refresh) == 0 {
		return SubscriptionRefreshSummary{}, ErrUnavailable
	}

	var summary SubscriptionRefreshSummary
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
		candidates, err := lister.ListSubscriptionCredentials(ctx, after, subscriptionRefreshPageSize, refreshBefore)
		if err != nil {
			return summary, ErrUnavailable
		}
		for _, candidate := range candidates {
			if err := ctx.Err(); err != nil {
				return summary, err
			}
			refresh := s.subscriptionRefresher(candidate.AuthAdapter)
			if refresh == nil {
				continue
			}
			summary.Checked++
			route := Route{
				ProviderID: candidate.ProviderID, ResourceID: candidate.ResourceID,
				AuthType: "SUBSCRIPTION", AuthAdapter: candidate.AuthAdapter,
				CredentialRefreshedAt: candidate.CredentialRefreshedAt,
				CredentialExpiresAt:   candidate.CredentialExpiresAt,
				ProxyEnabled:          candidate.ProxyEnabled,
				ProxyURL:              candidate.ProxyURL,
				ProxyHeaders:          candidate.ProxyHeaders,
			}
			credential, err := s.cipher.Decrypt(candidate.Credential, catalog.CredentialOwner{
				ProviderID: candidate.ProviderID, ResourceID: candidate.ResourceID,
			})
			if err != nil {
				summary.Failures = append(summary.Failures, SubscriptionRefreshFailure{
					ResourceID: candidate.ResourceID, ErrorCode: ErrCredential.Code,
				})
				continue
			}
			route.Proxy, err = s.decryptProxy(route)
			if err != nil {
				clear(credential)
				summary.Failures = append(summary.Failures, SubscriptionRefreshFailure{
					ResourceID: candidate.ResourceID, ErrorCode: ErrProxy.Code,
				})
				continue
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
				summary.Failures = append(summary.Failures, SubscriptionRefreshFailure{
					ResourceID: candidate.ResourceID, ErrorCode: ErrSubscription.Code,
				})
				continue
			}
			if changed {
				summary.Refreshed++
			}
		}
		if len(candidates) < int(subscriptionRefreshPageSize) {
			return summary, nil
		}
		next := candidates[len(candidates)-1].ResourceID
		if next <= after {
			return summary, errors.New("subscription refresh pagination did not advance")
		}
		after = next
	}
}

type SubscriptionRefreshWorker struct {
	service    *Service
	logger     *slog.Logger
	interval   time.Duration
	runTimeout time.Duration
	cancel     context.CancelFunc
	done       chan struct{}
	closeOnce  sync.Once
}

func NewSubscriptionRefreshWorker(service *Service, logger *slog.Logger, interval, runTimeout time.Duration) (*SubscriptionRefreshWorker, error) {
	if service == nil || interval <= 0 || runTimeout <= 0 {
		return nil, errors.New("invalid subscription refresh worker options")
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := &SubscriptionRefreshWorker{
		service: service, logger: logger, interval: interval, runTimeout: runTimeout,
		cancel: cancel, done: make(chan struct{}),
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
	summary, err := w.service.RefreshSubscriptions(ctx)
	if err != nil {
		if parent.Err() == nil {
			w.logger.Error("subscription refresh scan failed", "error_code", "SUBSCRIPTION_REFRESH_SCAN_FAILED")
		}
		return
	}
	for _, failure := range summary.Failures {
		w.logger.Warn("subscription credential refresh failed",
			"error_code", failure.ErrorCode, "resource_id", failure.ResourceID)
	}
	if summary.Refreshed > 0 || len(summary.Failures) > 0 {
		w.logger.Info("subscription refresh scan completed",
			"checked", summary.Checked, "refreshed", summary.Refreshed, "failed", len(summary.Failures),
			"duration_ms", time.Since(startedAt).Milliseconds())
	} else {
		w.logger.Debug("subscription refresh scan completed",
			"checked", summary.Checked, "refreshed", 0, "failed", 0,
			"duration_ms", time.Since(startedAt).Milliseconds())
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
