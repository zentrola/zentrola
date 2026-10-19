package redisstate

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestSessionRouteRedisIntegration(t *testing.T) {
	address := os.Getenv("ZENTROLA_TEST_REDIS_ADDR")
	if os.Getenv("ZENTROLA_INTEGRATION") != "1" || address == "" {
		t.Skip("set ZENTROLA_INTEGRATION=1 and ZENTROLA_TEST_REDIS_ADDR to test real Redis")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	first, second := New(host, port, 0, ""), New(host, port, 0, "")
	defer first.Close()
	defer second.Close()
	ctx := context.Background()
	if err := first.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("integration:%d:%d", os.Getpid(), time.Now().UnixNano())
	defer first.client.Del(ctx, sessionRouteRedisKey(key))

	var workers sync.WaitGroup
	results := make(chan int64, 32)
	errors := make(chan error, 32)
	for i := range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			state, preferred := first, int64(11)
			if i%2 == 1 {
				state, preferred = second, 12
			}
			assigned, err := state.Assign(ctx, key, preferred)
			results <- assigned
			errors <- err
		}()
	}
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	bound := int64(0)
	for assigned := range results {
		if assigned != 11 && assigned != 12 {
			t.Fatalf("unexpected resource %d", assigned)
		}
		if bound != 0 && assigned != bound {
			t.Fatalf("concurrent instances assigned different resources: %d and %d", bound, assigned)
		}
		bound = assigned
	}
	if got, err := second.Get(ctx, key); err != nil || got != bound {
		t.Fatalf("cross-instance Get=%d, err=%v, want=%d", got, err, bound)
	}
	if err := first.client.Expire(ctx, sessionRouteRedisKey(key), time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(ctx, key); err != nil {
		t.Fatal(err)
	}
	if ttl := first.client.TTL(ctx, sessionRouteRedisKey(key)).Val(); ttl < 29*24*time.Hour {
		t.Fatalf("Get did not refresh idle expiry: %s", ttl)
	}
	other := int64(11)
	if bound == 11 {
		other = 12
	}
	if moved, err := first.Move(ctx, key, other, 99); err != nil || moved {
		t.Fatalf("stale Move: moved=%t err=%v", moved, err)
	}
	if got, _ := second.Get(ctx, key); got != bound {
		t.Fatalf("stale route changed binding to %d", got)
	}
	if moved, err := first.Move(ctx, key, bound, other); err != nil || !moved {
		t.Fatalf("conditional Move: moved=%t err=%v", moved, err)
	}
	if got, err := second.Get(ctx, key); err != nil || got != other {
		t.Fatalf("conditional Move=%d, err=%v, want=%d", got, err, other)
	}
	// 并发故障转移的第二个请求执行相同 Move 时，绑定已指向目标，应幂等成功。
	if moved, err := second.Move(ctx, key, bound, other); err != nil || !moved {
		t.Fatalf("repeated Move: moved=%t err=%v", moved, err)
	}
}
