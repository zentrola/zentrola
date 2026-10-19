package redisstate

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/zentrola/zentrola/internal/application/gateway"
)

func TestRouteStateKeysAreResourceScopedAndSeparated(t *testing.T) {
	if got := cooldownKey(34); got != "zentrola:route:cooldown:v3:34" {
		t.Fatal(got)
	}
	if got := probeKey(34); got != "zentrola:route:probe:v1:34" {
		t.Fatal(got)
	}
}

func TestRedisFailureTemporarilySuppressesCalls(t *testing.T) {
	state := &State{}
	marker := errors.New("redis unavailable")
	if got := state.record(marker); !errors.Is(got, marker) || !state.redisUnavailable() {
		t.Fatal("Redis failure was not suppressed")
	}
	if err := state.record(nil); err != nil || state.redisUnavailable() {
		t.Fatal("successful Redis call did not restore availability")
	}
}

func TestSubscriptionLockFailsClosedWithoutRedisClient(t *testing.T) {
	state := &State{}
	_, acquired, err := state.AcquireSubscriptionRefreshScheduler(context.Background())
	if err == nil || acquired {
		t.Fatalf("acquired=%v err=%v", acquired, err)
	}
}

func TestBillingSettlementLockFailsClosedWithoutRedisClient(t *testing.T) {
	state := &State{}
	_, acquired, err := state.AcquireBillingSettlement(context.Background(), time.Minute)
	if err == nil || acquired {
		t.Fatalf("acquired=%v err=%v", acquired, err)
	}
}

func TestAPIKeyRatingLockFailsClosedWithoutRedisClient(t *testing.T) {
	state := &State{}
	_, acquired, err := state.AcquireAPIKeyRating(context.Background(), time.Minute)
	if err == nil || acquired {
		t.Fatalf("acquired=%v err=%v", acquired, err)
	}
}

func TestLockRejectsInvalidTTLBeforeCallingRedis(t *testing.T) {
	state := &State{client: redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})}
	defer state.Close()
	_, acquired, err := state.AcquireBillingSettlement(context.Background(), 0)
	if err == nil || acquired {
		t.Fatalf("acquired=%v err=%v", acquired, err)
	}
}

func TestCallerContextEndDoesNotTripRedisBreaker(t *testing.T) {
	marker := errors.New("connection refused")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name        string
		ctx         context.Context
		err         error
		unavailable bool
	}{
		{name: "redis failure", ctx: context.Background(), err: marker, unavailable: true},
		{name: "caller cancelled", ctx: cancelled, err: marker},
		{name: "wrapped cancellation", ctx: context.Background(), err: fmt.Errorf("pool: %w", context.Canceled)},
		{name: "caller deadline", ctx: context.Background(), err: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := &State{}
			if got := state.recordFor(test.ctx, test.err); !errors.Is(got, test.err) || state.redisUnavailable() != test.unavailable {
				t.Fatalf("err=%v unavailable=%v", got, state.redisUnavailable())
			}
		})
	}
}

func TestCancelledRequestsDoNotSuppressRouteState(t *testing.T) {
	state := &State{client: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})}
	defer state.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	route := gateway.Route{ResourceID: 7}
	calls := map[string]func() error{
		"acquire":  func() error { _, err := state.Acquire(ctx, route); return err },
		"cooldown": func() error { return state.Cooldown(ctx, route, time.Second) },
		"healthy":  func() error { return state.Healthy(ctx, route) },
		"get":      func() error { _, err := state.Get(ctx, "key"); return err },
		"assign":   func() error { _, err := state.Assign(ctx, "key", 7); return err },
		"move":     func() error { _, err := state.Move(ctx, "key", 7, 8); return err },
	}
	for name, call := range calls {
		if err := call(); err == nil || state.redisUnavailable() {
			t.Fatalf("%s: err=%v unavailable=%v", name, err, state.redisUnavailable())
		}
	}
	if _, err := state.Acquire(context.Background(), route); err == nil || !state.redisUnavailable() {
		t.Fatalf("real Redis failure was not recorded: err=%v", err)
	}
}
