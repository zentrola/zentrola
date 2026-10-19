package gatewaycache

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestCacheReadFailuresDoNotScheduleInvalidation(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name        string
		ctx         context.Context
		unavailable bool
	}{
		{name: "caller cancelled", ctx: cancelled},
		{name: "redis unreachable", ctx: context.Background(), unavailable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := New(Config{
				Host: "127.0.0.1", Port: 1, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			defer cache.Close()
			key := cache.RouteKey(appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}, "model", "OPENAI_CHAT")
			if _, _, ok := cache.GetRoutes(test.ctx, key); ok {
				t.Fatal("unreachable cache returned a hit")
			}
			unavailable := time.Now().UnixMilli() < cache.unavailableUntil.Load()
			if cache.invalidationPending.Load() || unavailable != test.unavailable {
				t.Fatalf("pending=%v unavailable=%v", cache.invalidationPending.Load(), unavailable)
			}
		})
	}
}
