package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	usageapp "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type SecurityHandlers struct {
	Admin           *appsec.AdminService
	Keys            *appsec.Keys
	Management      *mgmt.Service
	Applications    *mgmt.ApplicationService
	Gateway         *GatewayHandler
	OpenAI          *GatewayHandler
	ActiveModels    gw.ActiveModelReader
	Usage           *usageapp.QueryService
	UsageWriter     *usageapp.Writer
	BodyReadTimeout time.Duration
	logger          *slog.Logger
}
type adminIdentityKey struct{}
type principalIdentityKey struct{}

func (s *SecurityHandlers) mount(r chi.Router) {
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(bodyReadDeadline(s.BodyReadTimeout))
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, r)
			})
		})
		s.mountSetup(api)
		// @Summary 管理员登录
		// @Tags 认证
		// @Description 登录后复制 data.token，在 Authorize 的 AdminBearer 中填写 Bearer + 空格 + Token。
		// @Produce json
		// @Accept json
		// @Param body body LoginRequest true "请求参数"
		// @Success 200 {object} response{data=appsec.LoginResult}
		// @Failure 429 {object} response{data=LoginLockResponse} "ACCOUNT_LOCKED：账号临时锁定"
		// @Header 429 {integer} Retry-After "距离解锁的剩余秒数"
		// @Header all {string} X-Request-ID "请求追踪 ID"
		// @Failure 400 {object} response
		// @Failure 503 {object} response
		// @Failure 401 {object} response
		// @Router /api/v1/auth/login [post]
		api.Post("/auth/login", s.login)
		api.Group(func(protected chi.Router) {
			protected.Use(s.adminAuth)
			// @Summary 修改当前管理员登录密码
			// @Tags 认证
			// @Description 验证当前密码后更新密码，使该账号所有已签发的登录凭证失效；成功后需要重新登录。
			// @Accept json
			// @Produce json
			// @Security AdminBearer
			// @Param body body ChangePasswordRequest true "请求参数"
			// @Success 200 {object} response{data=LogoutResponse}
			// @Failure 400,401,403,503 {object} response
			// @Failure 429 {object} response{data=LoginLockResponse}
			// @Router /api/v1/auth/password [post]
			protected.Post("/auth/password", s.changePassword)
			// @Summary 当前管理员
			// @Tags 认证
			// @Produce json
			// @Security AdminBearer
			// @Success 200 {object} response{data=admin.Identity}
			// @Header all {string} X-Request-ID "请求追踪 ID"
			// @Failure 400 {object} response
			// @Failure 503 {object} response
			// @Failure 401 {object} response
			// @Failure 404 {object} response
			// @Router /api/v1/me [get]
			protected.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: adminFrom(r)})
			})
			// @Summary 管理员退出
			// @Tags 认证
			// @Description 客户端清除 Token；当前实现不在服务端撤销已签发 JWT。
			// @Produce json
			// @Security AdminBearer
			// @Success 200 {object} response{data=LogoutResponse}
			// @Header all {string} X-Request-ID "请求追踪 ID"
			// @Failure 400 {object} response
			// @Failure 503 {object} response
			// @Failure 401 {object} response
			// @Failure 404 {object} response
			// @Failure 409 {object} response
			// @Failure 422 {object} response
			// @Router /api/v1/auth/logout [post]
			protected.Post("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: LogoutResponse{ClearToken: true}})
			})
			// @Summary 签发成员 Access Key
			// @Tags 访问密钥
			// @Description 完整 Key 仅在本次响应返回，请妥善保存；Gateway 使用此 Key，不使用管理员 Token。
			// @Produce json
			// @Security AdminBearer
			// @Param id path string true "业务 ID（正整数字符串）"
			// @Accept json
			// @Param body body CreateKeyRequest true "请求参数"
			// @Success 201 {object} response{data=appsec.CreatedKey}
			// @Header all {string} X-Request-ID "请求追踪 ID"
			// @Failure 400 {object} response
			// @Failure 503 {object} response
			// @Failure 401 {object} response
			// @Failure 404 {object} response
			// @Failure 409 {object} response
			// @Failure 422 {object} response
			// @Router /api/v1/members/{id}/keys [post]
			protected.Post("/members/{id}/keys", s.createKey)
			// @Summary 签发应用 App Key
			// @Tags 应用管理
			// @Description 完整 App Key 仅在创建响应中展示一次；仅允许为 APPLICATION 签发。
			// @Accept json
			// @Produce json
			// @Security AdminBearer
			// @Param id path string true "应用 ID"
			// @Param body body CreateKeyRequest true "请求参数"
			// @Success 201 {object} response{data=appsec.CreatedKey}
			// @Failure 400,401,404,503 {object} response
			// @Router /api/v1/applications/{id}/keys [post]
			protected.Post("/applications/{id}/keys", s.createApplicationKey)
			// @Summary 撤销 Access Key
			// @Tags 访问密钥
			// @Produce json
			// @Security AdminBearer
			// @Param id path string true "业务 ID（正整数字符串）"
			// @Success 200 {object} response{data=RevokedResponse}
			// @Header all {string} X-Request-ID "请求追踪 ID"
			// @Failure 400 {object} response
			// @Failure 503 {object} response
			// @Failure 401 {object} response
			// @Failure 404 {object} response
			// @Failure 409 {object} response
			// @Failure 422 {object} response
			// @Router /api/v1/access-keys/{id}/revoke [post]
			protected.Post("/access-keys/{id}/revoke", s.revokeKey)
			if s.Management != nil {
				s.mountManagement(protected)
			}
			if s.Usage != nil {
				s.mountUsage(protected)
			}
			if s.ActiveModels != nil {
				s.mountActiveModels(protected)
			}
		})
		api.Group(func(member chi.Router) {
			member.Use(s.memberAuth)
			member.Get("/me/provider", s.getMyProvider)
			if s.Usage != nil {
				member.Get("/me/usage", s.getMyUsage)
			}
		})
	})
	r.Route("/anthropic", func(gateway chi.Router) {
		gateway.Use(s.gatewayAuth)
		gateway.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
			if s.Gateway != nil {
				s.Gateway.ServeHTTP(w, r)
				return
			}
			writeProtocolError(w, http.StatusNotImplemented, "api_error", "Gateway forwarding is not implemented yet.")
		})
	})
	r.Route("/v1", func(api chi.Router) {
		api.Use(s.openaiAuth)
		api.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
			if s.OpenAI == nil {
				writeOpenAIError(w, gw.ErrUnavailable)
				return
			}
			s.OpenAI.ServeHTTP(w, r)
		})
	})
}

func bodyReadDeadline(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if timeout <= 0 || r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}
			controller := http.NewResponseController(w)
			if err := controller.SetReadDeadline(time.Now().Add(timeout)); err == nil {
				defer controller.SetReadDeadline(time.Time{})
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *SecurityHandlers) login(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeRequest[LoginRequest](w, r)
	if !ok {
		return
	}
	result, err := s.Admin.Login(r.Context(), input.Username, input.Password, requestMeta(r))
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: result})
}
func (s *SecurityHandlers) changePassword(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeRequest[ChangePasswordRequest](w, r)
	if !ok {
		return
	}
	if err := s.Admin.ChangePassword(r.Context(), adminFrom(r), input.CurrentPassword, input.NewPassword, requestMeta(r)); err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: LogoutResponse{ClearToken: true}})
}
func (s *SecurityHandlers) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" || len(r.Header.Values("X-Api-Key")) != 0 {
			securityError(w, r, appsec.ErrUnauthenticated)
			return
		}
		identity, err := s.Admin.Authenticate(r.Context(), token)
		if err != nil {
			securityError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminIdentityKey{}, identity)))
	})
}

func (s *SecurityHandlers) memberAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Keys == nil {
			securityError(w, r, appsec.ErrUnavailable)
			return
		}
		keys := r.Header.Values("X-Api-Key")
		auth := r.Header.Values("Authorization")
		var key string
		if len(keys) == 1 && len(auth) == 0 {
			key = keys[0]
		} else if len(keys) == 0 && len(auth) == 1 {
			key = bearer(r)
		}
		identity, err := s.Keys.Authenticate(r.Context(), key)
		if err != nil {
			securityError(w, r, err)
			return
		}
		if identity.Type != "MEMBER" {
			securityError(w, r, appsec.ErrUnauthenticated)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalIdentityKey{}, identity)))
	})
}

func (s *SecurityHandlers) gatewayAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := s.gatewayLogger()
		logger.InfoContext(r.Context(), "gateway request received",
			"gateway_protocol", "anthropic", "method", r.Method, "path", r.URL.Path)
		keys := r.Header.Values("X-Api-Key")
		auth := r.Header.Values("Authorization")
		var key string
		credentialSource := "invalid"
		if len(keys) == 1 && len(auth) == 0 {
			key = keys[0]
			credentialSource = "x-api-key"
		} else if len(keys) == 0 && len(auth) == 1 {
			key = bearer(r)
			credentialSource = "bearer"
		}
		identity, err := s.Keys.Authenticate(r.Context(), key)
		if err != nil {
			errorCode := "DEPENDENCY_UNAVAILABLE"
			if errors.Is(err, appsec.ErrUnauthenticated) {
				errorCode = "UNAUTHENTICATED"
			}
			logger.WarnContext(r.Context(), "gateway authentication failed",
				"gateway_protocol", "anthropic", "credential_source", credentialSource,
				"request_id", requestIDFromContext(r.Context()),
				"error_code", errorCode)
			if errors.Is(err, appsec.ErrUnauthenticated) {
				writeGatewayError(w, gw.ErrAuthentication)
			} else {
				writeGatewayError(w, gw.ErrUnavailable)
			}
			return
		}
		logger.InfoContext(r.Context(), "gateway authentication succeeded",
			"gateway_protocol", "anthropic", "credential_source", credentialSource,
			"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalIdentityKey{}, identity)))
	})
}

func (s *SecurityHandlers) openaiAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := s.gatewayLogger()
		logger.InfoContext(r.Context(), "gateway request received",
			"gateway_protocol", "openai", "method", r.Method, "path", r.URL.Path)
		key := bearer(r)
		credentialSource := "bearer"
		if len(r.Header.Values("X-Api-Key")) > 0 {
			key = ""
			credentialSource = "invalid"
		}
		identity, err := s.Keys.Authenticate(r.Context(), key)
		if err != nil {
			failure := gw.ErrUnavailable
			if errors.Is(err, appsec.ErrUnauthenticated) {
				failure = gw.ErrAuthentication
			}
			logger.WarnContext(r.Context(), "gateway authentication failed",
				"gateway_protocol", "openai", "credential_source", credentialSource,
				"request_id", requestIDFromContext(r.Context()),
				"error_code", failure.Code)
			writeOpenAIError(w, failure)
			return
		}
		logger.InfoContext(r.Context(), "gateway authentication succeeded",
			"gateway_protocol", "openai", "credential_source", credentialSource,
			"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalIdentityKey{}, identity)))
	})
}

func (s *SecurityHandlers) gatewayLogger() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}
func (s *SecurityHandlers) createKey(w http.ResponseWriter, r *http.Request) {
	s.createPrincipalKey(w, r, false)
}
func (s *SecurityHandlers) createApplicationKey(w http.ResponseWriter, r *http.Request) {
	s.createPrincipalKey(w, r, true)
}
func (s *SecurityHandlers) createPrincipalKey(w http.ResponseWriter, r *http.Request, application bool) {
	id, err := positiveID(chi.URLParam(r, "id"))
	if err != nil {
		securityError(w, r, err)
		return
	}
	input, ok := decodeRequest[CreateKeyRequest](w, r)
	if !ok {
		return
	}
	var created appsec.CreatedKey
	if application {
		created, err = s.Keys.CreateApplication(r.Context(), adminFrom(r), id, input.Name, input.ExpiresAt, requestMeta(r))
	} else {
		created, err = s.Keys.Create(r.Context(), adminFrom(r), id, input.Name, input.ExpiresAt, requestMeta(r))
	}
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, response{Code: "OK", Data: created})
}
func (s *SecurityHandlers) revokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(chi.URLParam(r, "id"))
	if err != nil {
		securityError(w, r, err)
		return
	}
	if err := s.Keys.Revoke(r.Context(), adminFrom(r), id, requestMeta(r)); err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: RevokedResponse{Revoked: true}})
}
func positiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, appsec.ErrInvalidArgument
	}
	return id, nil
}
func adminFrom(r *http.Request) admin.Identity {
	identity, _ := r.Context().Value(adminIdentityKey{}).(admin.Identity)
	return identity
}
func principalFrom(r *http.Request) appsec.PrincipalIdentity {
	identity, _ := r.Context().Value(principalIdentityKey{}).(appsec.PrincipalIdentity)
	return identity
}
func bearer(r *http.Request) string {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return ""
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
func requestMeta(r *http.Request) appsec.RequestMeta {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return appsec.RequestMeta{RequestID: requestIDFromContext(r.Context()), Method: r.Method, Path: r.URL.Path, IP: ip, UserAgent: r.UserAgent()}
}

type requestPayload interface {
	Normalize()
	Valid() bool
}

var (
	_ requestPayload = (*ChangePasswordRequest)(nil)
	_ requestPayload = (*LoginRequest)(nil)
	_ requestPayload = (*CreateKeyRequest)(nil)
	_ requestPayload = (*CreateMemberRequest)(nil)
	_ requestPayload = (*UpdateMemberRequest)(nil)
	_ requestPayload = (*CreateGroupRequest)(nil)
	_ requestPayload = (*UpdateGroupRequest)(nil)
	_ requestPayload = (*CreateResourceRequest)(nil)
	_ requestPayload = (*UpdateCredentialRequest)(nil)
	_ requestPayload = (*UpdateStatusRequest)(nil)
	_ requestPayload = (*ModelRequest)(nil)
	_ requestPayload = (*ProviderRequest)(nil)
	_ requestPayload = (*ProviderInitializeRequest)(nil)
)

func decodeRequest[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var input T
	if r.URL.RawQuery != "" {
		securityError(w, r, appsec.ErrInvalidArgument)
		return input, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, requestBodyLimit(any(&input))))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		securityError(w, r, appsec.ErrInvalidArgument)
		return input, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		securityError(w, r, appsec.ErrInvalidArgument)
		return input, false
	}
	payload, ok := any(&input).(requestPayload)
	if !ok {
		panic("http: request payload does not implement normalization and validation")
	}
	payload.Normalize()
	if field := missingRequiredParameter(reflect.ValueOf(payload), ""); field != "" {
		securityError(w, r, &missingRequiredParameterError{Field: field})
		return input, false
	}
	if !payload.Valid() {
		securityError(w, r, appsec.ErrInvalidArgument)
		return input, false
	}
	return input, true
}

func requestBodyLimit(payload any) int64 {
	const defaultLimit int64 = 16 << 10
	const credentialLimit int64 = 512 << 10
	switch payload.(type) {
	case *CreateResourceRequest, *UpdateCredentialRequest:
		// Credential 解码后最多允许 64 KiB；JSON 转义可能显著放大线上字节数。
		return credentialLimit
	default:
		return defaultLimit
	}
}

func writeProtocolError(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": message}})
}
