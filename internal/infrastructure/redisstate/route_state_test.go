package redisstate

import (
	"errors"
	"testing"
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
