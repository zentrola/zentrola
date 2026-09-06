package http

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWebBuildOnlyAndSecurityHeaders(t *testing.T) {
	files := fstest.MapFS{
		"index.html":        {Data: []byte("<html>zentrola</html>")},
		"assets/app-123.js": {Data: []byte("export const ready = true")},
		".env":              {Data: []byte("private configuration")},
	}
	for _, tc := range []struct {
		url, fixed string
		status     int
	}{
		{"/", "index.html", 200},
		{"/assets/app-123.js", "", 200},
		{"/assets/missing.js", "", 404},
		{"/assets/../.env", "", 404},
		{"/assets/%2e%2e/.env", "", 404},
		{"/.env", "", 404},
		{"/assets/", "", 404},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			webFile(files, tc.fixed)(w, httptest.NewRequest("GET", tc.url, nil))
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private configuration") {
				t.Fatalf("unexpected response: %d", w.Code)
			}
			if w.Code == 200 {
				if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
					t.Fatal("missing security headers")
				}
				if tc.fixed == "index.html" && w.Header().Get("Cache-Control") != "no-cache" {
					t.Fatal("HTML must revalidate")
				}
			}
		})
	}
}
