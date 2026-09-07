package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesRuntimeConfigAndOnlyPublicFiles(t *testing.T) {
	files := fstest.MapFS{
		"index.html":          {Data: []byte("<html>zentrola</html>")},
		"favicon.svg":         {Data: []byte("<svg/>")},
		"assets/app-123.js":   {Data: []byte("export const ready = true")},
		"private/config.json": {Data: []byte("secret")},
	}
	server := httptest.NewServer(handler(webConfig{
		APIBaseURL: "https://api.example.com/base",
		APIOrigin:  "https://api.example.com",
	}, files))
	defer server.Close()

	for _, test := range []struct {
		path, contains, cache string
		status                int
	}{
		{"/", "zentrola", "no-cache", 200},
		{"/config.js", `"apiBaseUrl":"https://api.example.com/base"`, "no-store", 200},
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
		":3000":          "http://127.0.0.1:3000",
		"0.0.0.0:3000":   "http://127.0.0.1:3000",
		"127.0.0.1:3000": "http://127.0.0.1:3000",
		"[::1]:3000":     "http://[::1]:3000",
	}
	for input, want := range tests {
		if got := displayURL(input); got != want {
			t.Fatalf("displayURL(%q) = %q, want %q", input, got, want)
		}
	}
}
