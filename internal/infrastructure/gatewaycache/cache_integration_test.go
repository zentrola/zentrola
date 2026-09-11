package gatewaycache

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	appconfig "github.com/zentrola/zentrola/internal/infrastructure/config"
)

func TestRedisCacheRoundTripAndGenerationInvalidation(t *testing.T) {
	if os.Getenv("ZENTROLA_REDIS_INTEGRATION") != "1" {
		t.Skip("set ZENTROLA_REDIS_INTEGRATION=1 to run Redis integration tests")
	}
	host, password := os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PASSWORD")
	port, database := integerEnvironment("REDIS_PORT", 6379), integerEnvironment("REDIS_DB", 2)
	if configured, err := appconfig.Load("../../../.env"); err == nil {
		if host == "" {
			host = configured.Redis.Host
		}
		if os.Getenv("REDIS_PORT") == "" {
			port = configured.Redis.Port
		}
		if os.Getenv("REDIS_DB") == "" {
			database = configured.Redis.Database
		}
		if password == "" {
			password = configured.Redis.Password
		}
	}
	if host == "" {
		host = "127.0.0.1"
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	namespace := Namespace("test") + ":" + suffix
	generationNamespace := BaseNamespace("test") + ":" + suffix
	cache := New(Config{Host: host, Port: port, Database: database, Password: password, Namespace: namespace, GenerationNamespace: generationNamespace})
	defer cache.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cache.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	identityKey := cache.IdentityKey([]byte("digest"))
	routeIdentity := appsec.PrincipalIdentity{ID: 12, AccessKeyID: 34}
	routeKey := cache.RouteKey(routeIdentity, "model-a", gw.OpenAIProtocol)
	var dataKeys []string
	defer func() {
		keys := append([]string{cache.generationKey()}, dataKeys...)
		_ = cache.client.Del(context.Background(), keys...).Err()
	}()

	_, initialGeneration, ok := cache.GetIdentity(ctx, identityKey)
	if ok || initialGeneration == "" {
		t.Fatalf("unexpected initial cache state: generation=%q hit=%v", initialGeneration, ok)
	} else {
		dataKeys = append(dataKeys, identityKey+":"+initialGeneration)
		cache.SetIdentity(ctx, identityKey, initialGeneration, routeIdentity)
	}
	if identity, _, ok := cache.GetIdentity(ctx, identityKey); !ok || identity.ID != routeIdentity.ID {
		t.Fatal("identity cache round trip failed")
	}
	_, routeGeneration, ok := cache.GetRoutes(ctx, routeKey)
	if ok || routeGeneration != initialGeneration {
		t.Fatalf("unexpected initial route cache state: generation=%q hit=%v", routeGeneration, ok)
	} else {
		dataKeys = append(dataKeys, routeKey+":"+routeGeneration)
		cache.SetRoutes(ctx, routeKey, routeGeneration, []gw.Route{{ModelID: 1, ResourceID: 2}})
	}
	if routes, _, ok := cache.GetRoutes(ctx, routeKey); !ok || len(routes) != 1 || routes[0].ResourceID != 2 {
		t.Fatal("route cache round trip failed")
	}

	cache.Clear(ctx, "test")
	_, clearedGeneration, ok := cache.GetIdentity(ctx, identityKey)
	if ok || clearedGeneration == "" || clearedGeneration == initialGeneration {
		t.Fatalf("identity cache survived invalidation: generation=%q hit=%v", clearedGeneration, ok)
	}
	if _, generation, ok := cache.GetRoutes(ctx, routeKey); ok || generation != clearedGeneration {
		t.Fatalf("route cache survived invalidation: generation=%q hit=%v", generation, ok)
	}

	if err := cache.client.Del(ctx, cache.generationKey()).Err(); err != nil {
		t.Fatal(err)
	}
	_, recoveredGeneration, ok := cache.GetIdentity(ctx, identityKey)
	if ok || recoveredGeneration == "" || recoveredGeneration == initialGeneration || recoveredGeneration == clearedGeneration {
		t.Fatalf("missing generation resurrected stale cache: generation=%q hit=%v", recoveredGeneration, ok)
	}

	if err := cache.client.Del(ctx, cache.generationKey()).Err(); err != nil {
		t.Fatal(err)
	}
	if err := cache.client.LPush(ctx, cache.generationKey(), "wrong-type").Err(); err != nil {
		t.Fatal(err)
	}
	if !cache.Clear(ctx, "repair_wrong_type") {
		t.Fatal("cache invalidation did not repair wrong-type generation key")
	}
	if _, generation, ok := cache.GetIdentity(ctx, identityKey); ok || generation == "" {
		t.Fatalf("unexpected initial cache state: generation=%q hit=%v", generation, ok)
	}
}

func integerEnvironment(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
