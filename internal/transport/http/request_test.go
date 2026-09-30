package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (r *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	r.deadlines = append(r.deadlines, deadline)
	return nil
}

func assertMissingParameter[T any](t *testing.T, method, path, body, field string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if _, ok := decodeRequest[T](recorder, request); ok {
		t.Fatalf("request missing %s was accepted", field)
	}
	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Field string `json:"field"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || result.Code != "MISSING_REQUIRED_PARAMETER" ||
		result.Data.Field != field || !strings.Contains(result.Message, field) {
		t.Fatalf("missing parameter response = %d %+v", recorder.Code, result)
	}
}

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
		ok   bool
	}{
		{name: "valid and trimmed", body: `{"status":"  ACTIVE  "}`, ok: true},
		{name: "invalid value", body: `{"status":"ARCHIVED"}`, ok: false},
		{name: "malformed", body: `{"status":`, ok: false},
		{name: "unknown field", body: `{"status":"ACTIVE","extra":true}`, ok: false},
		{name: "multiple values", body: `{"status":"ACTIVE"} {"status":"DISABLED"}`, ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/models/1/status", strings.NewReader(test.body))
			input, ok := decodeRequest[UpdateStatusRequest](recorder, request)
			if ok != test.ok {
				t.Fatalf("decodeRequest() ok = %v, want %v", ok, test.ok)
			}
			if ok {
				if input.Status != "ACTIVE" || recorder.Body.Len() != 0 {
					t.Fatalf("valid request decoded incorrectly: input=%+v body=%q", input, recorder.Body.String())
				}
				return
			}
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_ARGUMENT"`) {
				t.Fatalf("invalid request response = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestDecodeRequestUsesCredentialSpecificBodyLimit(t *testing.T) {
	credential := strings.Repeat("x", 64<<10)
	body, err := json.Marshal(map[string]any{
		"providerId": "1", "name": "subscription", "credential": credential,
		"authType": "SUBSCRIPTION", "authAdapter": "OPENAI_CODEX",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/resources", strings.NewReader(string(body)))
	input, ok := decodeRequest[CreateResourceRequest](recorder, request)
	if !ok || input.Credential != credential {
		t.Fatalf("valid large credential rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/v1/models/1/status", strings.NewReader(
		`{"status":"ACTIVE","padding":"`+strings.Repeat("x", 20<<10)+`"}`,
	))
	if _, ok := decodeRequest[UpdateStatusRequest](recorder, request); ok || recorder.Code != http.StatusBadRequest {
		t.Fatalf("oversized ordinary request accepted: status=%d", recorder.Code)
	}
}

func TestBodyReadDeadlineIsSetAndCleared(t *testing.T) {
	recorder := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{}`))
	handler := bodyReadDeadline(time.Second)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	handler.ServeHTTP(recorder, request)
	if len(recorder.deadlines) != 2 || recorder.deadlines[0].IsZero() || !recorder.deadlines[1].IsZero() {
		t.Fatalf("read deadlines = %v, want set then clear", recorder.deadlines)
	}
}

func TestConsumeResetCreditRequestValidation(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/resources/1/rate-limit-reset-credit/consume", strings.NewReader(`{"idempotencyKey":" request-1 ","creditId":" credit-1 "}`))
	input, ok := decodeRequest[ConsumeResetCreditRequest](recorder, request)
	if !ok || input.IdempotencyKey != "request-1" || input.CreditID != "credit-1" {
		t.Fatalf("valid reset credit request rejected: %+v status=%d", input, recorder.Code)
	}
	assertMissingParameter[ConsumeResetCreditRequest](
		t, http.MethodPost, "/api/v1/resources/1/rate-limit-reset-credit/consume", `{}`, "idempotencyKey",
	)
}

func TestTimeInputsRequireUTC(t *testing.T) {
	utc := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	offset := time.Date(2099, 1, 1, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	zeroOffset := time.Date(2099, 1, 1, 0, 0, 0, 0, time.FixedZone("UTC+0", 0))

	if !(CreateKeyRequest{Name: "key", ExpiresAt: &utc}).Valid() {
		t.Fatal("UTC access key expiry was rejected")
	}
	for _, value := range []*time.Time{&offset, &zeroOffset} {
		if (CreateKeyRequest{Name: "key", ExpiresAt: value}).Valid() {
			t.Fatalf("non-Z access key expiry was accepted: %s", value.Format(time.RFC3339))
		}
	}

	validResource := CreateResourceRequest{
		ProviderID: 1, Name: "resource", Credential: "secret", AuthType: "API_KEY", AuthAdapter: "API_KEY",
		EffectiveAt: &utc,
	}
	if !validResource.Valid() {
		t.Fatal("UTC resource time was rejected")
	}
	validResource.EffectiveAt = &offset
	if validResource.Valid() {
		t.Fatal("non-UTC resource time was accepted")
	}
}

func TestParseUTCQueryTime(t *testing.T) {
	parsed, err := parseUTCQueryTime("2026-09-15T03:20:00.123Z")
	if err != nil || parsed.Location() != time.UTC {
		t.Fatalf("UTC query time = %s, %v", parsed, err)
	}
	for _, value := range []string{
		"2026-09-15T11:20:00+08:00",
		"2026-09-15T03:20:00+00:00",
		"2026-09-15T03:20:00",
	} {
		if _, err := parseUTCQueryTime(value); err == nil {
			t.Fatalf("non-Z query time was accepted: %s", value)
		}
	}
}

func TestMissingRequiredParameterReturnsField(t *testing.T) {
	t.Run("top level", func(t *testing.T) {
		assertMissingParameter[UpdateStatusRequest](t, http.MethodPatch, "/api/v1/models/1/status", `{}`, "status")
	})
	t.Run("empty required collection", func(t *testing.T) {
		assertMissingParameter[mgmt.ModelInput](t, http.MethodPost, "/api/v1/models",
			`{"code":"model","name":"模型","inputModalities":[],"outputModalities":["TEXT"]}`,
			"inputModalities")
	})
	t.Run("nested field", func(t *testing.T) {
		assertMissingParameter[mgmt.ProviderInput](t, http.MethodPost, "/api/v1/providers",
			`{"name":"服务商","endpoints":[{"protocolType":"OPENAI"}],"mappings":[{"modelId":"1","upstreamModelCode":"model"}]}`,
			"endpoints[0].baseUrl")
	})
}

func TestProviderProxyUpdateAllowsEmptyMappings(t *testing.T) {
	const fields = `"name":"OpenAI","website":"https://openai.com","endpoints":[{"protocolType":"OPENAI","baseUrl":"https://api.openai.com/v1","networkScope":"PUBLIC"}],"proxyEnabled":true,"proxyUrl":"socks5h://proxy.example.com:8001","proxyHeaders":[]`
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			const path = "/api/v1/providers/4"
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, path, strings.NewReader(`{`+fields+`,"mappings":[]}`))
			input, ok := decodeRequest[mgmt.ProviderInput](recorder, request)
			if !ok || input.Mappings == nil || len(input.Mappings) != 0 || !input.ProxyEnabled {
				t.Fatalf("proxy configuration with empty mappings rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			for _, suffix := range []string{"", `,"mappings":null`} {
				assertMissingParameter[mgmt.ProviderInput](t, method, path, `{`+fields+suffix+`}`, "mappings")
			}
			assertMissingParameter[mgmt.ProviderInput](t, method, path, `{`+fields+`,"mappings":[{}]}`, "mappings[0].modelId")
		})
	}
}

func TestDecodeRequestNormalizesTextFields(t *testing.T) {
	t.Run("login preserves password", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"  admin  ","password":"  secret  "}`))
		input, ok := decodeRequest[LoginRequest](recorder, request)
		if !ok || input.Username != "admin" || input.Password != "  secret  " {
			t.Fatalf("login normalization = %+v, ok=%v", input, ok)
		}
	})

	t.Run("member fields and ids", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/members", strings.NewReader(`{"name":"  开发者  ","remark":"  研发成员  ","groupIds":[" 12 "]}`))
		input, ok := decodeRequest[CreateMemberRequest](recorder, request)
		if !ok || input.Name != "开发者" || input.Remark != "研发成员" || len(input.GroupIDs) != 1 || input.GroupIDs[0] != "12" {
			t.Fatalf("member normalization = %+v, ok=%v", input, ok)
		}
	})

	t.Run("model nested fields", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/models", strings.NewReader(`{"code":"  model-code  ","name":"  模型  ","inputModalities":[" TEXT "],"outputModalities":[" TEXT "],"remark":"  备注  "}`))
		input, ok := decodeRequest[mgmt.ModelInput](recorder, request)
		if !ok || input.Code != "model-code" || input.Name != "模型" || input.Remark != "备注" || input.InputModalities[0] != "TEXT" {
			t.Fatalf("model normalization = %+v, ok=%v", input, ok)
		}
	})
}

func TestInvalidStatusStopsBeforeApplication(t *testing.T) {
	called := false
	handler := statusEndpoint(func(context.Context, admin.Identity, int64, string, appsec.RequestMeta) error {
		called = true
		return nil
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/models/1/status", strings.NewReader(`{"status":"ARCHIVED"}`))
	handler.ServeHTTP(recorder, request)
	if called || recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid status called application=%v, status=%d", called, recorder.Code)
	}
}

func TestProviderCredentialRequiredReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/8/status", nil)

	securityError(recorder, request, mgmt.ErrProviderCredentialRequired)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"PROVIDER_CREDENTIAL_REQUIRED"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMemberAccessKeyRequiredReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/members/8/status", nil)

	securityError(recorder, request, mgmt.ErrMemberAccessKeyRequired)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"MEMBER_ACCESS_KEY_REQUIRED"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSubscriptionAccountAlreadyExistsReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/resources", nil)

	securityError(recorder, request, mgmt.ErrSubscriptionAccountExists)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"SUBSCRIPTION_ACCOUNT_ALREADY_EXISTS"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProviderModelMappingRequiredReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/providers/8/status", nil)

	securityError(recorder, request, mgmt.ErrProviderModelMappingRequired)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"PROVIDER_MODEL_MAPPING_REQUIRED"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestModelSyncCredentialRequiredReturnsConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/providers/8/sync-models", nil)

	securityError(recorder, request, mgmt.ErrModelSyncCredentialRequired)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"MODEL_SYNC_CREDENTIAL_REQUIRED"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInvalidPathIDStopsBeforeApplication(t *testing.T) {
	called := false
	handler := statusEndpoint(func(context.Context, admin.Identity, int64, string, appsec.RequestMeta) error {
		called = true
		return nil
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/models/not-a-number/status", strings.NewReader(`{"status":"ACTIVE"}`))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", "not-a-number")
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	handler.ServeHTTP(recorder, request)
	if called || recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid path id called application=%v, status=%d", called, recorder.Code)
	}
}

func TestOptionalQueryValueRejectsDuplicates(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/models?status=ACTIVE&status=DISABLED", nil)
	if _, err := optionalQueryValue(request, "status"); err == nil {
		t.Fatal("optionalQueryValue() accepted duplicate values")
	}
}

func TestOptionalQueryValueTrimsSpace(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/models?status=++ACTIVE++", nil)
	value, err := optionalQueryValue(request, "status")
	if err != nil || value != "ACTIVE" {
		t.Fatalf("optionalQueryValue() = %q, %v", value, err)
	}
}

func TestListEndpointRejectsUnknownQueryBeforeLoading(t *testing.T) {
	called := false
	handler := listEndpoint(func(*http.Request, mgmt.Page) (mgmt.PageData[mgmt.Model], error) {
		called = true
		return mgmt.PageData[mgmt.Model]{}, nil
	}, func(model mgmt.Model) int64 { return model.ID })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/models?unknown=value", nil)
	handler.ServeHTTP(recorder, request)
	if called || recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown query called loader=%v, status=%d", called, recorder.Code)
	}
}

func TestListEndpointReturnsTotalAndUsesExtraRowToProbeNextPage(t *testing.T) {
	probed := false
	handler := listEndpoint(func(_ *http.Request, page mgmt.Page) (mgmt.PageData[mgmt.Model], error) {
		probed = page.ProbeNext
		return mgmt.PageData[mgmt.Model]{
			Items: []mgmt.Model{{ID: 2}, {ID: 1}},
			Total: 2,
		}, nil
	}, func(model mgmt.Model) int64 { return model.ID })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/models?limit=1", nil)
	handler.ServeHTTP(recorder, request)
	var response struct {
		Data PageResponse[mgmt.Model] `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || !probed || len(response.Data.Items) != 1 || response.Data.Total != 2 || response.Data.NextCursor == nil || *response.Data.NextCursor != "2" {
		t.Fatalf("unexpected page response: status=%d probed=%v data=%+v", recorder.Code, probed, response.Data)
	}
}

func TestBodyEndpointRejectsQueryBeforeDecoding(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/models/1/status?force=true", strings.NewReader(`{"status":"ACTIVE"}`))
	if _, ok := decodeRequest[UpdateStatusRequest](recorder, request); ok || recorder.Code != http.StatusBadRequest {
		t.Fatalf("request with query accepted=%v, status=%d", ok, recorder.Code)
	}
}
