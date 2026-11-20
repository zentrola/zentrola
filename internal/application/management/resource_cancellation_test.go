package management

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type cancellationSubscriptionAdapter struct {
	subscriptionAdapterStub
	operation func(context.Context) (SubscriptionProbe, error)
}

func (s cancellationSubscriptionAdapter) Probe(ctx context.Context, _ []byte, _ *catalog.OutboundProxy) (SubscriptionProbe, error) {
	return s.operation(ctx)
}

func (s cancellationSubscriptionAdapter) ConsumeResetCredit(ctx context.Context, _ []byte, _ *catalog.OutboundProxy, _, _ string) (ResetCreditConsume, error) {
	probe, err := s.operation(ctx)
	return ResetCreditConsume{Outcome: "reset", Probe: probe}, err
}

type cancellationResourceStore struct {
	lockingResourceStore
	t *testing.T
}

func (s cancellationResourceStore) Write(ctx context.Context, actor admin.Identity, fn func(Writer) error) error {
	if ctx.Err() != nil {
		s.t.Fatal("credential persistence was canceled with the caller")
	}
	*s.events = append(*s.events, "write")
	return s.lockingResourceStore.Write(ctx, actor, fn)
}

func TestSubscriptionOperationsPersistRotationAfterCallerCancellation(t *testing.T) {
	actor, meta := admin.Identity{ID: 1}, appsec.RequestMeta{}
	for _, operation := range []struct {
		name         string
		run          func(context.Context, *Service) error
		failureError error
	}{
		{"connection test", func(ctx context.Context, service *Service) error {
			_, err := service.TestResource(ctx, actor, 48, meta)
			return err
		}, nil},
		{"quota refresh", func(ctx context.Context, service *Service) error {
			_, err := service.RefreshResourceQuota(ctx, actor, 48, meta)
			return err
		}, nil},
		{"reset credit", func(ctx context.Context, service *Service) error {
			_, err := service.ConsumeResourceResetCredit(ctx, actor, 48, "request-1", "", meta)
			return err
		}, appsec.ErrUnavailable},
	} {
		for _, fail := range []bool{false, true} {
			name := operation.name + "/success"
			if fail {
				name = operation.name + "/failure after rotation"
			}
			t.Run(name, func(t *testing.T) {
				type contextKey struct{}
				ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "request-context"))
				defer cancel()
				writer := subscriptionRefreshWriter()
				events := []string{}
				var operationCtx context.Context
				adapter := cancellationSubscriptionAdapter{operation: func(ctx context.Context) (SubscriptionProbe, error) {
					events = append(events, "operation")
					operationCtx = ctx
					cancel() // 模拟上游已轮换凭据后，浏览器请求断开。
					if ctx.Err() != nil || ctx.Value(contextKey{}) != "request-context" {
						t.Fatal("account operation lost its context values or followed caller cancellation")
					}
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > subscriptionOperationTimeout {
						t.Fatal("account operation is not bounded by its own timeout")
					}
					if fail {
						return SubscriptionProbe{}, &rotatedProbeError{credential: []byte("rotated-auth-cache")}
					}
					return SubscriptionProbe{Credential: []byte("rotated-auth-cache"), Quotas: []ResourceQuota{{Code: "codex", Status: QuotaAvailable}}}, nil
				}}
				store := cancellationResourceStore{lockingResourceStore{writer: writer, events: &events}, t}
				service := New(store, nil, providerTestCipher{}, nil, WithSubscriptionAdapter(adapter))
				err := operation.run(ctx, service)
				var wantErr error
				if fail {
					wantErr = operation.failureError
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("error=%v, want %v", err, wantErr)
				}
				if string(writer.resource.Sealed.Ciphertext) != "rotated-auth-cache" {
					t.Fatal("rotated credential was not persisted")
				}
				sequence := strings.Join(events, ",")
				if !strings.Contains(sequence, "operation,write") || events[len(events)-1] != "unlock" {
					t.Fatalf("refresh lock was not held through persistence: %s", sequence)
				}
				if operationCtx == nil || operationCtx.Err() == nil {
					t.Fatal("account operation context was not released")
				}
			})
		}
	}
}

func TestCanceledSubscriptionOperationDoesNotStart(t *testing.T) {
	events := []string{}
	service := New(lockingResourceStore{writer: subscriptionRefreshWriter(), events: &events}, nil, providerTestCipher{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	operationCtx, release, err := service.beginSubscriptionOperation(ctx, 48)
	if !errors.Is(err, context.Canceled) || operationCtx != nil || release != nil {
		t.Fatalf("canceled caller started an operation: %v", err)
	}
	if strings.Join(events, ",") != "lock,unlock" {
		t.Fatalf("refresh lock leaked: %v", events)
	}
}
