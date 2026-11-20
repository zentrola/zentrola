package modelcatalog

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestUseCodexClientVersionSelectsCatalogVersion(t *testing.T) {
	for _, test := range []struct {
		name, configured, version, source string
	}{
		{"configured", "0.172.0", "0.172.0", "configured"},
		{"trimmed", " 0.172.0 ", "0.172.0", "configured"},
		{"default", "", defaultCodexClientVersion, "default"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
			discoverer.UseCodexClientVersion(context.Background(), test.configured)
			adapter := discoverer.adapters[catalog.OpenAIOfficialCode]
			for i := 0; i < 2; i++ {
				request, err := adapter.Request(codexSubscriptionSource, 1, "")
				if err != nil || request.URL != codexCatalogBaseURL+"?client_version="+test.version {
					t.Fatalf("catalog request=%+v err=%v", request, err)
				}
			}
			request, err := adapter.Request(mgmt.ModelDiscoverySource{AuthType: mgmt.AuthTypeAPIKey}, 1, "")
			if err != nil || request.URL != openAICatalogURL {
				t.Fatalf("API Key catalog request=%+v err=%v", request, err)
			}
			if !strings.Contains(logs.String(), `"client_version":"`+test.version+`"`) ||
				!strings.Contains(logs.String(), `"version_source":"`+test.source+`"`) {
				t.Fatalf("missing version selection log: %s", logs.String())
			}
		})
	}
}
