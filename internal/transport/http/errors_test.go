package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

func TestEveryPublicManagementErrorHasHTTPMapping(t *testing.T) {
	publicErrors := mgmt.PublicErrors()
	if len(publicErrors) != len(managementErrorMappings) {
		t.Fatalf("public management errors and HTTP mappings differ: errors=%d mappings=%d", len(publicErrors), len(managementErrorMappings))
	}
	for _, public := range publicErrors {
		mapped := false
		for _, mapping := range managementErrorMappings {
			if errors.Is(public, mapping.err) {
				mapped = true
				break
			}
		}
		if !mapped {
			t.Fatalf("public error %q has no HTTP mapping", public)
		}
		recorder := httptest.NewRecorder()
		securityError(recorder, httptest.NewRequest(http.MethodGet, "/", nil), public)
		if recorder.Code == http.StatusServiceUnavailable {
			t.Fatalf("public error %q fell through to SERVICE_UNAVAILABLE", public)
		}
	}
}
