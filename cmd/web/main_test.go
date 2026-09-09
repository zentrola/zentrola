package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseWebOptions(t *testing.T) {
	options, err := parseWebOptions([]string{"--api", "https://api.example.com", "--gateway", "https://gateway.example.com"})
	if err != nil || options.APIBaseURL != "https://api.example.com" || options.GatewayBaseURL != "https://gateway.example.com" {
		t.Fatalf("options=%+v err=%v", options, err)
	}
	if _, err := parseWebOptions([]string{"--unknown"}); err == nil {
		t.Fatal("unknown option accepted")
	}
	var help bytes.Buffer
	writeHelp(&help)
	if !strings.Contains(help.String(), "--api") || !strings.Contains(help.String(), "9528") {
		t.Fatal("help does not describe runtime configuration")
	}
}

func TestCommandLineAPIOverridesEnvironmentAndGateway(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(file, []byte("WEB_API_BASE_URL=https://old-api.example.com\nWEB_GATEWAY_BASE_URL=https://old-gateway.example.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(file, webOptions{APIBaseURL: "https://api.example.com/base/"})
	if err != nil {
		t.Fatal(err)
	}
	if config.APIBaseURL != "https://api.example.com/base" || config.APIOrigin != "https://api.example.com" || config.GatewayBaseURL != config.APIBaseURL {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestCommandLineAPIWorksWithoutEnvFile(t *testing.T) {
	config, err := loadConfig(filepath.Join(t.TempDir(), "missing.env"), webOptions{APIBaseURL: "https://api.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Address != ":9528" || config.APIBaseURL != "https://api.example.com" || config.GatewayBaseURL != config.APIBaseURL {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestHandlerServesRuntimeConfigAndOnlyPublicFiles(t *testing.T) {
	files := fstest.MapFS{
		"index.html":          {Data: []byte("<html>zentrola</html>")},
		"favicon.svg":         {Data: []byte("<svg/>")},
		"assets/app-123.js":   {Data: []byte("export const ready = true")},
		"private/config.json": {Data: []byte("secret")},
	}
	server := httptest.NewServer(handler(webConfig{
		APIBaseURL:     "https://api.example.com/base",
		APIOrigin:      "https://api.example.com",
		GatewayBaseURL: "https://gateway.example.com",
	}, files))
	defer server.Close()

	for _, test := range []struct {
		path, contains, cache string
		status                int
	}{
		{"/", "zentrola", "no-cache", 200},
		{"/config.js", `"apiBaseUrl":"https://api.example.com/base"`, "no-store", 200},
		{"/config.js", `"gatewayBaseUrl":"https://gateway.example.com"`, "no-store", 200},
		{"/assets/app-123.js", "ready", "immutable", 200},
		{"/private/config.json", "", "", 404},
		{"/assets/../private/config.json", "", "", 404},
	} {
		t.Run(test.path, func(t *testing.T) {
			response, err := http.Get(server.URL + test.path)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != test.status || !strings.Contains(string(body), test.contains) {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
			if test.cache != "" && !strings.Contains(response.Header.Get("Cache-Control"), test.cache) {
				t.Fatalf("cache=%q", response.Header.Get("Cache-Control"))
			}
			if response.StatusCode == 200 &&
				(!strings.Contains(response.Header.Get("Content-Security-Policy"), "https://api.example.com") ||
					response.Header.Get("X-Content-Type-Options") != "nosniff") {
				t.Fatal("缺少安全响应头")
			}
		})
	}
}

func TestDisplayURL(t *testing.T) {
	tests := map[string]string{
		":9528":          "http://127.0.0.1:9528",
		"0.0.0.0:9528":   "http://127.0.0.1:9528",
		"127.0.0.1:9528": "http://127.0.0.1:9528",
		"[::1]:9528":     "http://[::1]:9528",
	}
	for input, want := range tests {
		if got := displayURL(input); got != want {
			t.Fatalf("displayURL(%q) = %q, want %q", input, got, want)
		}
	}
}
