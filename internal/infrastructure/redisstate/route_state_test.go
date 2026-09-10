package redisstate

import (
	"errors"
	"testing"
)

func TestRouteKeyIsCredentialScoped(t *testing.T) {
	if got := routeKey(34); got != "zentrola:route:cooldown:v2:34" {
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
