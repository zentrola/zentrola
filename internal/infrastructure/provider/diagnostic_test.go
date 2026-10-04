package provider

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestProviderDiagnosticsRedactSecrets(t *testing.T) {

	raw := "https://user:password@example.com/redirect?token=redirect-secret"
	diagnostic := DiagnoseNetworkError(&url.Error{Op: "Post", URL: raw, Err: errors.New("credential=network-secret")})
	if !diagnostic.Redacted || strings.Contains(diagnostic.Detail, "network-secret") ||
		strings.Contains(diagnostic.Detail, "redirect-secret") || diagnostic.Detail != "upstream transport failed" {
		t.Fatalf("production diagnostic exposed details: %+v", diagnostic)
	}
	location, redacted := RedirectLocationForLog(raw)
	if !redacted || location != "https://example.com" {
		t.Fatalf("production redirect was not reduced to origin: location=%q redacted=%v", location, redacted)
	}
	headers := HeadersForLog(http.Header{
		"Authorization":      {"Bearer authorization-secret"},
		"ChatGPT-Account-Id": {"account-secret"},
		"X-Api-Key":          {"api-key-secret"},
		"X-Credential-Id":    {"credential-secret"},
		"X-Custom-Context":   {"custom-secret"},
		"Content-Type":       {"application/json"},
	})
	if headers.Get("Authorization") != "******" || headers.Get("ChatGPT-Account-Id") != "******" ||
		headers.Get("X-Api-Key") != "******" ||
		headers.Get("X-Credential-Id") != "******" || headers.Get("X-Custom-Context") != "******" ||
		headers.Get("Content-Type") != "application/json" {
		t.Fatalf("production headers have incorrect redaction: %+v", headers)
	}
}
