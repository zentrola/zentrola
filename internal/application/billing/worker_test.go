package billing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
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
