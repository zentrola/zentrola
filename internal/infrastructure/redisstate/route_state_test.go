package redisstate

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	tests := []struct {
		name        string
		ctx         context.Context
		err         error
		unavailable bool
	}{
		{name: "redis failure", ctx: context.Background(), err: marker, unavailable: true},
		{name: "caller cancelled", ctx: cancelled, err: marker},
		{name: "caller deadline", ctx: expired, err: context.DeadlineExceeded},
		// net 拨号超时满足 errors.Is(err, context.DeadlineExceeded)，但调用方并未结束，必须触发熔断。
		{name: "dial timeout without caller end", ctx: context.Background(), err: fmt.Errorf("dial tcp: %w", context.DeadlineExceeded), unavailable: true},
		{name: "wrapped cancellation without caller end", ctx: context.Background(), err: fmt.Errorf("pool: %w", context.Canceled), unavailable: true},
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

func TestRedisDialTimeoutTripsRedisBreaker(t *testing.T) {
	route := gateway.Route{ResourceID: 7}
	tests := []struct {
		name string
		call func(*State) error
	}{
		{name: "acquire", call: func(s *State) error { _, err := s.Acquire(context.Background(), route); return err }},
		{name: "cooldown", call: func(s *State) error { return s.Cooldown(context.Background(), route, time.Second) }},
		{name: "get", call: func(s *State) error { _, err := s.Get(context.Background(), "key"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := &State{client: dialTimeoutClient()}
			defer state.Close()
			err := test.call(state)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected dial timeout error, got %v", err)
			}
			if !state.redisUnavailable() {
				t.Fatalf("dial timeout did not trip Redis breaker: %v", err)
			}
		})
	}
}

// dialTimeoutClient 返回每次拨号都以 net 包真实拨号超时错误失败的客户端；调用方上下文保持未结束。
func dialTimeoutClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:1", MaxRetries: -1, DialerRetries: 1,
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// 已过期的拨号期限让 net.Dialer 确定性地返回拨号超时，与 DialTimeout 到期时的错误一致。
			dialCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer cancel()
			var dialer net.Dialer
			return dialer.DialContext(dialCtx, network, addr)
		},
	})
}
