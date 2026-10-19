package redisbreaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBreakerRecordFor(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	redisErr := errors.New("dial tcp: i/o timeout")
	tests := []struct {
		name     string
		ctx      context.Context
		err      error
		wantOpen bool
	}{
		{name: "redis failure opens", ctx: context.Background(), err: redisErr, wantOpen: true},
		// 拨号超时同样满足 context.DeadlineExceeded，只能依据调用方上下文判定。
		{name: "redis dial deadline opens", ctx: context.Background(), err: context.DeadlineExceeded, wantOpen: true},
		{name: "caller cancellation keeps closed", ctx: cancelled, err: context.Canceled},
		{name: "success keeps closed", ctx: context.Background()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var breaker Breaker
			if got := breaker.RecordFor(tt.ctx, now, tt.err); !errors.Is(got, tt.err) {
				t.Fatalf("err=%v want %v", got, tt.err)
			}
			if breaker.Open(now) != tt.wantOpen {
				t.Fatalf("open=%v want %v", breaker.Open(now), tt.wantOpen)
			}
		})
	}
}

func TestBreakerClosesAfterRetryOrSuccess(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	var breaker Breaker
	_ = breaker.Record(now, errors.New("redis unavailable"))
	if !breaker.Open(now.Add(RetryAfter-time.Millisecond)) || breaker.Open(now.Add(RetryAfter)) {
		t.Fatal("breaker did not open for exactly RetryAfter")
	}
	if err := breaker.Record(now, nil); err != nil || breaker.Open(now) {
		t.Fatalf("success did not close breaker: err=%v", err)
	}
}
