package redisstate

import (
	"context"
	"errors"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
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
