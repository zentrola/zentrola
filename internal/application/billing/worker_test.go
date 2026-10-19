package billing

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	domain "github.com/zentrola/zentrola/internal/domain/billing"
)

type settlementCoordinatorStub struct {
	acquired bool
	err      error
	calls    int
	released bool
	ttl      time.Duration
}

func (c *settlementCoordinatorStub) AcquireBillingSettlement(_ context.Context, ttl time.Duration) (func(), bool, error) {
	c.calls++
	c.ttl = ttl
	if c.err != nil || !c.acquired {
		return nil, false, c.err
	}
	return func() { c.released = true }, true, nil
}

func TestBillingWorkerRequiresCoordinator(t *testing.T) {
	service := New(&billingStoreStub{}, &billingIDs{})
	if _, err := NewWorker(service, nil, nil, time.Hour, 0, time.Minute); err == nil {
		t.Fatal("worker accepted a missing settlement coordinator")
	}
}

func TestBillingWorkerSkipsWhenDistributedLockIsNotAcquired(t *testing.T) {
	coordinator := &settlementCoordinatorStub{}
	worker := &Worker{
		coordinator: coordinator,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		timeout:     time.Minute,
	}
	worker.settle(context.Background())
	if coordinator.calls != 1 || coordinator.released {
		t.Fatalf("calls=%d released=%v", coordinator.calls, coordinator.released)
	}
}

func TestBillingWorkerFailsClosedWhenDistributedLockErrors(t *testing.T) {
	coordinator := &settlementCoordinatorStub{err: errors.New("redis unavailable")}
	worker := &Worker{
		coordinator: coordinator,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		timeout:     time.Minute,
	}
	worker.settle(context.Background())
	if coordinator.calls != 1 || coordinator.released {
		t.Fatalf("calls=%d released=%v", coordinator.calls, coordinator.released)
	}
}

func TestBillingWorkerHoldsDistributedLockForSettlement(t *testing.T) {
	coordinator := &settlementCoordinatorStub{acquired: true}
	service := New(&billingStoreStub{}, &billingIDs{})
	worker := &Worker{
		service: service, coordinator: coordinator,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), timeout: time.Minute,
	}
	worker.settle(context.Background())
	if coordinator.calls != 1 || !coordinator.released || coordinator.ttl != 2*time.Minute {
		t.Fatalf("calls=%d released=%v ttl=%s", coordinator.calls, coordinator.released, coordinator.ttl)
	}
}

func TestSettlementLeaseTTLDoesNotOverflow(t *testing.T) {
	maximum := time.Duration(1<<63 - 1)
	if got := settlementLeaseTTL(maximum); got != maximum {
		t.Fatalf("ttl=%s want=%s", got, maximum)
	}
}

type failingSubscriptionStore struct{ billingStoreStub }

func (*failingSubscriptionStore) SubscriptionPrices(context.Context) ([]SubscriptionPrice, error) {
	return nil, &StoreError{Operation: "subscription_prices", SQLState: "57014", Message: "canceling statement due to statement timeout"}
}

func TestBillingWorkerSettlesAPIKeysWhenSubscriptionSettlementFails(t *testing.T) {
	period := APIKeyPeriod{
		CredentialID: 30, Currency: "CNY",
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	store := &failingSubscriptionStore{billingStoreStub{
		apiKeyPeriods: []APIKeyPeriod{period},
		apiKeyUsage:   map[APIKeyPeriod][]domain.RatedShare{period: {{PrincipalID: 1, Tokens: 10, Amount: "1"}}},
		apiKeyCurrent: map[APIKeyPeriod]APIKeyCurrent{},
	}}
	var output bytes.Buffer
	worker := &Worker{
		service: New(store, &billingIDs{}), coordinator: &settlementCoordinatorStub{acquired: true},
		logger: slog.New(slog.NewJSONHandler(&output, nil)), timeout: time.Minute,
	}
	worker.settle(context.Background())
	if len(store.apiKeyDocuments) != 1 {
		t.Fatalf("api key settlement did not run: %+v", store.apiKeyDocuments)
	}
	logs := output.String()
	if !strings.Contains(logs, "SUBSCRIPTION_BILLING_FAILED") || !strings.Contains(logs, "sqlstate=57014") ||
		!strings.Contains(logs, "billing settlement completed") {
		t.Fatalf("unexpected logs: %s", logs)
	}
}
