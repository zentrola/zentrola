package openaicodex

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

func TestCodexEnvironmentKeepsOnlyAllowedVariables(t *testing.T) {
	source := []string{
		"PATH=/usr/bin", "Path=C:\\Windows", "HOME=/home/zentrola", "USERPROFILE=C:\\Users\\z", "APPDATA=C:\\Users\\z\\AppData\\Roaming",
		"LOCALAPPDATA=C:\\Users\\z\\AppData\\Local", "TMP=C:\\Temp", "TEMP=C:\\Temp", "TMPDIR=/tmp",
		"SystemRoot=C:\\Windows", "ComSpec=C:\\Windows\\system32\\cmd.exe", "PATHEXT=.COM;.EXE;.CMD",
		"SSL_CERT_FILE=/etc/ssl/cert.pem", "SSL_CERT_DIR=/etc/ssl/certs", "LANG=C.UTF-8", "LC_ALL=C.UTF-8",
		"HTTP_PROXY=http://proxy.example:8080", "https_proxy=http://proxy.example:8080", "no_proxy=localhost",
		"POSTGRES_PASSWORD=database-secret", "MASTER_KEY=master-secret", "ADMIN_JWT_SECRET=jwt-secret",
		"CODEX_HOME=/home/zentrola/.codex", "OPENAI_API_KEY=sk-secret", "=C:=C:\\work", "MALFORMED",
	}
	got := codexEnvironment(source)
	for _, want := range []string{
		"PATH=/usr/bin", "Path=C:\\Windows", "HOME=/home/zentrola", "USERPROFILE=C:\\Users\\z", "APPDATA=C:\\Users\\z\\AppData\\Roaming",
		"LOCALAPPDATA=C:\\Users\\z\\AppData\\Local", "TMP=C:\\Temp", "TEMP=C:\\Temp", "TMPDIR=/tmp",
		"SystemRoot=C:\\Windows", "ComSpec=C:\\Windows\\system32\\cmd.exe", "PATHEXT=.COM;.EXE;.CMD",
		"SSL_CERT_FILE=/etc/ssl/cert.pem", "SSL_CERT_DIR=/etc/ssl/certs", "LANG=C.UTF-8", "LC_ALL=C.UTF-8",
		"HTTP_PROXY=http://proxy.example:8080", "https_proxy=http://proxy.example:8080", "no_proxy=localhost",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("allowed variable %q missing from %q", want, got)
		}
	}
	joined := strings.Join(got, "\n")
	for _, forbidden := range []string{"database-secret", "master-secret", "jwt-secret", "CODEX_HOME", "sk-secret", "MALFORMED"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("forbidden variable %q inherited: %q", forbidden, got)
		}
	}
	if slices.Contains(got, "=C:=C:\\work") {
		t.Fatalf("drive working directory variable inherited: %q", got)
	}
}

func TestCodexEnvironmentProviderProxyOverridesInheritedProxy(t *testing.T) {
	environment, err := provider.EnvironmentWithProxy(context.Background(), codexEnvironment([]string{
		"PATH=/usr/bin", "https_proxy=http://global.example:8080", "NO_PROXY=chatgpt.com",
	}), &catalog.OutboundProxy{URL: "http://provider.example:3128"}, provider.ProxyRequestLog{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "global.example") || strings.Contains(strings.ToUpper(joined), "NO_PROXY=") ||
		!slices.Contains(environment, "HTTPS_PROXY=http://provider.example:3128") || !slices.Contains(environment, "PATH=/usr/bin") {
		t.Fatalf("unexpected environment: %q", environment)
	}
}
