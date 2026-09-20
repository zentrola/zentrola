package gatewaycache

import (
	"context"
	"errors"
	"strings"
	"testing"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
)

func TestActiveModelKeyEncodesModelCode(t *testing.T) {
	cache := &Cache{namespace: "zentrola:test:gateway:v1"}
	key := cache.activeModelKey("vendor:model/preview")
	if !strings.HasPrefix(key, cache.namespace+":active-models:") || strings.Contains(strings.TrimPrefix(key, cache.namespace+":active-models:"), ":") {
		t.Fatalf("unexpected active model key: %q", key)
	}
}

func TestRecordActiveRouteRejectsIncompleteRoute(t *testing.T) {
	cache := &Cache{}
	for _, route := range []gw.Route{
		{ModelCode: "gpt-6", ProviderName: "OpenAI"},
		{ModelName: "GPT 6", ProviderName: "OpenAI"},
		{ModelName: "GPT 6", ModelCode: "gpt-6"},
	} {
		if err := cache.RecordActiveRoute(context.Background(), route); !errors.Is(err, gw.ErrInvalid) {
			t.Fatalf("RecordActiveRoute(%+v) error = %v, want ErrInvalid", route, err)
		}
	}
}
