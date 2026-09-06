package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestLoginLockResponse(t *testing.T) {
	until := time.Now().UTC().Add(90 * time.Second)
	rec := httptest.NewRecorder()
	err := fmt.Errorf("login: %w", &appsec.AccountLockedError{LockedUntil: until})
	securityError(rec, httptest.NewRequest("POST", "/api/v1/auth/login", nil), err)
	var body struct {
		Code string            `json:"code"`
		Data LoginLockResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusTooManyRequests || body.Code != "ACCOUNT_LOCKED" ||
		!body.Data.LockedUntil.Equal(until) || body.Data.RetryAfterSeconds < 89 || body.Data.RetryAfterSeconds > 90 ||
		rec.Header().Get("Retry-After") != strconv.FormatInt(body.Data.RetryAfterSeconds, 10) {
		t.Fatalf("unexpected lock response: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	securityError(rec, httptest.NewRequest("POST", "/api/v1/auth/login", nil), appsec.ErrUnauthenticated)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("Retry-After") != "" {
		t.Fatal("ordinary authentication failure must not expose a lock")
	}
}
