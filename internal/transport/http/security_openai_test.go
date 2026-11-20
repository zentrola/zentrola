package http

import (
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestOpenAIAuthAcceptsMatchingCredentialHeaders(t *testing.T) {
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	other := "vk-" + base64.RawURLEncoding.EncodeToString([]byte{
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	})
	security := &SecurityHandlers{
		Keys:   appsec.NewKeys(&selfUsageKeyStore{identity: appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7}}, nil),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := security.openaiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if identity := principalFrom(r); identity.ID != 42 || identity.AccessKeyID != 7 {
			t.Errorf("unexpected principal identity: %+v", identity)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, test := range []struct {
		name          string
		authorization []string
		apiKeys       []string
		wantStatus    int
	}{
		{name: "bearer only", authorization: []string{"Bearer " + key}, wantStatus: http.StatusNoContent},
		{name: "matching headers", authorization: []string{"Bearer " + key}, apiKeys: []string{key}, wantStatus: http.StatusNoContent},
		{name: "conflicting headers", authorization: []string{"Bearer " + key}, apiKeys: []string{other}, wantStatus: http.StatusUnauthorized},
		{name: "x-api-key only", apiKeys: []string{key}, wantStatus: http.StatusUnauthorized},
		{name: "malformed bearer", authorization: []string{key}, apiKeys: []string{key}, wantStatus: http.StatusUnauthorized},
		{name: "duplicate authorization", authorization: []string{"Bearer " + key, "Bearer " + key}, apiKeys: []string{key}, wantStatus: http.StatusUnauthorized},
		{name: "duplicate x-api-key", authorization: []string{"Bearer " + key}, apiKeys: []string{key, key}, wantStatus: http.StatusUnauthorized},
		{name: "missing credentials", wantStatus: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			for _, authorization := range test.authorization {
				request.Header.Add("Authorization", authorization)
			}
			for _, apiKey := range test.apiKeys {
				request.Header.Add("X-Api-Key", apiKey)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d, want=%d, body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
		})
	}
}
