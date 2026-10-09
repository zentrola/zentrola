package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestGatewayModelErrorsIncludeRequestedModel(t *testing.T) {
	unknownMessage, routeMessage := gw.ErrModelUnknown.Message, gw.ErrRoute.Message
	for _, test := range []struct {
		name, path, model string
		failure           *gw.Failure
		openAI            bool
	}{
		{"openai unknown model", "/v1/responses", "unlisted-model", gw.ErrModelUnknown, true},
		{"openai missing mapping", "/v1/responses", "codex-auto-review", gw.ErrRoute, true},
		{"anthropic unknown model", "/anthropic/v1/messages", "unlisted-model", gw.ErrModelUnknown, false},
		{"anthropic missing mapping", "/anthropic/v1/messages", "claude-sonnet", gw.ErrRoute, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &selfProviderStore{err: test.failure}
			service := gw.New(store, nil, nil)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			options := GatewayOptions{MaxBodyBytes: 1 << 20, BodyReadTimeout: time.Second, RequestTimeout: time.Second}
			handler := NewGatewayHandler(service, options, logger)
			if test.openAI {
				handler = NewOpenAIGatewayHandler(service, options, logger, nil)
			}
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"model":"`+test.model+`"}`))
			request.Header.Set("Content-Type", "application/json")
			request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7}))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != test.failure.Status || recorder.Header().Get("X-Zentrola-Error-Code") != test.failure.Code {
				t.Fatalf("unexpected response: status=%d header=%q body=%s", recorder.Code, recorder.Header().Get("X-Zentrola-Error-Code"), recorder.Body.String())
			}
			var response struct {
				Error struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(response.Error.Message, strconv.Quote(test.model)) || store.model != test.model {
				t.Fatalf("requested model missing: response=%+v resolved=%q", response, store.model)
			}
			if test.openAI && response.Error.Code != test.failure.Code {
				t.Fatalf("OpenAI error code=%q, want %q", response.Error.Code, test.failure.Code)
			}
			if !test.openAI && response.Error.Code != "" {
				t.Fatalf("Anthropic error gained a code field: %s", recorder.Body.String())
			}
		})
	}
	if gw.ErrModelUnknown.Message != unknownMessage || gw.ErrRoute.Message != routeMessage {
		t.Fatal("shared gateway failures were modified")
	}
}
