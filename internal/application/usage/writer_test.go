package usage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/zentrola/zentrola/internal/domain/usage"
)

type testIDs struct{ n atomic.Int64 }

func (i *testIDs) NextID(context.Context) (int64, error) { return i.n.Add(1), nil }

type storeFunc func(context.Context, []domain.Event) error

func (f storeFunc) WriteBatch(c context.Context, e []domain.Event) error { return f(c, e) }
func attemptEvent(requestID string) domain.Event {
	return domain.Event{RequestID: requestID, Attempt: &domain.Attempt{}}
}
func writerFor(t *testing.T, s Store, q, b int) *Writer {
	t.Helper()
	w, err := NewWriter(s, &testIDs{}, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{QueueSize: q, BatchSize: b, FlushInterval: 10 * time.Millisecond, WriteTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = w.Close(ctx)
	})
	return w
}
func TestQueueFullSynchronousFallback(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	w := writerFor(t, storeFunc(func(ctx context.Context, e []domain.Event) error {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}), 1, 1)
	if err := w.Submit(attemptEvent("one")); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := w.Submit(attemptEvent("two")); err != nil {
		t.Fatal(err)
	}
	if err := w.Submit(attemptEvent("three")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || w.Metrics().Fallback != 1 || w.Metrics().Persisted != 1 {
		t.Fatal("full queue did not synchronously persist")
	}
	close(release)
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m := w.Metrics(); m.Persisted != 3 || m.Pending != 0 || m.Failed != 0 {
		t.Fatal(m)
	}
}
func TestSynchronousFailureIsReported(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	w := writerFor(t, storeFunc(func(ctx context.Context, events []domain.Event) error {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if events[0].RequestID == "fail-sync" {
			return errors.New("unavailable")
		}
		return nil
	}), 1, 1)
	_ = w.Submit(attemptEvent("worker"))
	<-started
	_ = w.Submit(attemptEvent("queued"))
	if w.Submit(attemptEvent("fail-sync")) == nil {
		t.Fatal("synchronous failure hidden")
	}
	if m := w.Metrics(); m.Fallback != 1 || m.Failed != 1 || m.Persisted != 0 {
		t.Fatal(m)
	}
	close(release)
	if w.Close(context.Background()) == nil {
		t.Fatal("failed writer closed as healthy")
	}
}

func TestBatchFailureIsolationAndFailedMetric(t *testing.T) {
	w := writerFor(t, storeFunc(func(ctx context.Context, e []domain.Event) error {
		if len(e) > 1 || e[0].RequestID == "bad" {
			return errors.New("secret database diagnostic")
		}
		return nil
	}), 10, 10)
	_ = w.Submit(attemptEvent("good"))
	_ = w.Submit(attemptEvent("bad"))
	if w.Close(context.Background()) == nil {
		t.Fatal("failed persistence reported success")
	}
	if m := w.Metrics(); m.Failed != 1 || m.Persisted != 1 || m.Pending != 0 || m.WriteErrors != 2 {
		t.Fatal(m)
	}
}
func TestTimerFlushAndSnapshot(t *testing.T) {
	received := make(chan []domain.Event, 1)
	w := writerFor(t, storeFunc(func(ctx context.Context, e []domain.Event) error {
		copy := append([]domain.Event(nil), e...)
		received <- copy
		return nil
	}), 10, 10)
	n := int64(12)
	e := domain.Event{RequestID: "copy", Attempt: &domain.Attempt{InputTokens: &n}}
	_ = w.Submit(e)
	n = 99
	e.Attempt.ResourceID = 666
	select {
	case got := <-received:
		if *got[0].Attempt.InputTokens != 12 || got[0].Attempt.ResourceID != 0 {
			t.Fatal("queued event changed")
		}
	case <-time.After(time.Second):
		t.Fatal("timer did not flush")
	}
}
func TestEveryFailoverAttemptGetsAnIDAndSequence(t *testing.T) {
	received := make(chan []domain.Event, 1)
	w := writerFor(t, storeFunc(func(_ context.Context, events []domain.Event) error {
		copyEvents := append([]domain.Event(nil), events...)
		copyEvents[0].Attempts = append([]domain.Attempt(nil), events[0].Attempts...)
		if events[0].Attempt != nil {
			attempt := *events[0].Attempt
			copyEvents[0].Attempt = &attempt
		}
		received <- copyEvents
		return nil
	}), 10, 10)
	event := domain.Event{
		RequestID: "failover",
		Attempts:  []domain.Attempt{{ResourceID: 100}},
		Attempt:   &domain.Attempt{ResourceID: 200},
	}
	if err := w.Submit(event); err != nil {
		t.Fatal(err)
	}
	select {
	case events := <-received:
		got := events[0]
		if len(got.Attempts) != 1 || got.Attempts[0].ID <= 0 || got.Attempts[0].AttemptNo != 1 || got.Attempt == nil || got.Attempt.ID <= got.Attempts[0].ID || got.Attempt.AttemptNo != 2 {
			t.Fatalf("attempts=%+v active=%+v", got.Attempts, got.Attempt)
		}
	case <-time.After(time.Second):
		t.Fatal("timer did not flush failover attempts")
	}
}
func TestConcurrentCloseAndTimeout(t *testing.T) {
	w := writerFor(t, storeFunc(func(ctx context.Context, e []domain.Event) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			return nil
		}
	}), 5, 2)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = w.Submit(attemptEvent("concurrent")) }()
	}
	_ = w.Close(context.Background())
	wg.Wait()
	if w.Metrics().Pending != 0 {
		t.Fatal("events left pending")
	}
	blocked := writerFor(t, storeFunc(func(ctx context.Context, e []domain.Event) error { <-ctx.Done(); return ctx.Err() }), 10, 1)
	_ = blocked.Submit(attemptEvent("blocked"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if blocked.Close(ctx) == nil || blocked.Metrics().Failed != 1 || blocked.Metrics().Pending != 0 {
		t.Fatal("shutdown timeout not reported/drained")
	}
}
