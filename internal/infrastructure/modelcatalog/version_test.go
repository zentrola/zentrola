package modelcatalog

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.172.0-alpha.2")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestUseCodexExecutableSelectsCatalogVersion(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, executable, version, source string
	}{
		{"detected", executable, "0.172.0", "executable"},
		{"fallback", filepath.Join(t.TempDir(), "missing-codex"), defaultCodexClientVersion, "fallback"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
			discoverer.UseCodexExecutable(context.Background(), test.executable)
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
