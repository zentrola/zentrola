package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestDuplicateMemberNameReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	securityError(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/me/manage/members", nil), appsec.ErrMemberNameExists)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"MEMBER_NAME_ALREADY_EXISTS"`) {
		t.Fatalf("duplicate member response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDisabledMemberReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	securityError(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/me/manage/members/1/keys", nil), appsec.ErrMemberDisabled)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"MEMBER_DISABLED"`) {
		t.Fatalf("disabled member response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestManagedMemberAccessKeyRequiredReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	securityError(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/me/manage/members/1/status", nil), appsec.ErrMemberAccessKeyRequired)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"MEMBER_ACCESS_KEY_REQUIRED"`) {
		t.Fatalf("missing key response = %d %s", recorder.Code, recorder.Body.String())
	}
}

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
