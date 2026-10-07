package billing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type ratingCoordinatorStub struct {
	acquired bool
	err      error
	calls    int
	released bool
	ttl      time.Duration
}

func (c *ratingCoordinatorStub) AcquireAPIKeyRating(_ context.Context, ttl time.Duration) (func(), bool, error) {
	c.calls++
	c.ttl = ttl
	if c.err != nil || !c.acquired {
		return nil, false, c.err
	}
	return func() { c.released = true }, true, nil
}

func TestRatingWorkerRequiresCoordinator(t *testing.T) {
	service := New(&billingStoreStub{}, &billingIDs{})
	if _, err := NewRatingWorker(service, nil, nil, time.Minute, 50*time.Second, 500); err == nil {
		t.Fatal("worker accepted a missing rating coordinator")
	}
}

func TestRatingWorkerFailsClosedWhenDistributedLockErrors(t *testing.T) {
	coordinator := &ratingCoordinatorStub{err: errors.New("redis unavailable")}
	worker := &RatingWorker{
		coordinator: coordinator, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), timeout: time.Minute,
	}
	worker.rate(context.Background())
	if coordinator.calls != 1 || coordinator.released {
		t.Fatalf("calls=%d released=%v", coordinator.calls, coordinator.released)
	}
}

func TestRatingWorkerHoldsDistributedLockForRating(t *testing.T) {
	coordinator := &ratingCoordinatorStub{acquired: true}
	service := New(&billingStoreStub{}, &billingIDs{})
	worker := &RatingWorker{
		service: service, coordinator: coordinator,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), timeout: time.Minute, pageSize: 100,
	}
	worker.rate(context.Background())
	if coordinator.calls != 1 || !coordinator.released || coordinator.ttl != 2*time.Minute {
		t.Fatalf("calls=%d released=%v ttl=%s", coordinator.calls, coordinator.released, coordinator.ttl)
	}
}
