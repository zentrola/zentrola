package gatewaycache

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
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
			unavailable := cache.redisUnavailable()
			if cache.invalidationPending.Load() || unavailable != test.unavailable {
				t.Fatalf("pending=%v unavailable=%v", cache.invalidationPending.Load(), unavailable)
			}
		})
	}
}

func TestCacheDialTimeoutTripsBreaker(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Cache)
	}{
		{name: "read", call: func(ctx context.Context, cache *Cache) {
			key := cache.RouteKey(appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}, "model", "OPENAI_CHAT")
			if _, _, ok := cache.GetRoutes(ctx, key); ok {
				t.Fatal("unreachable cache returned a hit")
			}
		}},
		{name: "ping", call: func(ctx context.Context, cache *Cache) {
			if err := cache.Ping(ctx); err == nil {
				t.Fatal("unreachable cache ping succeeded")
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := New(Config{Host: "127.0.0.1", Port: 1, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			defer cache.Close()
			_ = cache.client.Close()
			cache.client = dialTimeoutClient()
			test.call(context.Background(), cache)
			if !cache.redisUnavailable() {
				t.Fatal("dial timeout did not trip Redis breaker")
			}
		})
	}
}

// dialTimeoutClient 返回每次拨号都以 net 包真实拨号超时错误失败的客户端；调用方上下文保持未结束。
// 该错误满足 errors.Is(err, context.DeadlineExceeded)，与 DialTimeout 到期时一致。
func dialTimeoutClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:1", MaxRetries: -1, DialerRetries: 1,
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer cancel()
			var dialer net.Dialer
			return dialer.DialContext(dialCtx, network, addr)
		},
	})
}
