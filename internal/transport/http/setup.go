package http

import (
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

type SetupResponse struct {
	Initialized bool `json:"initialized"`
}

func (s *SecurityHandlers) mountSetup(r chi.Router) {
	// @Summary 查询是否需要初始化管理员
	// @Tags 认证
	// @Description 只返回是否尚未创建管理员；已有任何管理员记录均不重新开放初始化。
	// @Produce json
	// @Success 200 {object} response{data=appsec.SetupStatus}
	// @Failure 503 {object} response
	// @Router /api/v1/auth/setup [get]
	r.Get("/auth/setup", func(w http.ResponseWriter, req *http.Request) {
		status, err := s.Admin.SetupStatus(req.Context())
		adminResult(w, req, http.StatusOK, status, err)
	})
	// @Summary 创建系统首位管理员
	// @Tags 认证
	// @Description 仅未初始化时可用；账号 1～64 bytes、密码 12～72 bytes；创建成功后使用登录接口。已有管理员时返回 ALREADY_INITIALIZED。
	// @Accept json
	// @Produce json
	// @Param body body LoginRequest true "初始管理员账号与密码"
	// @Success 201 {object} response{data=SetupResponse}
	// @Failure 400 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/auth/setup [post]
	r.Post("/auth/setup", func(w http.ResponseWriter, req *http.Request) {
		kind, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if err != nil || kind != "application/json" || req.URL.RawQuery != "" {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		var input LoginRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		err = s.Admin.Initialize(req.Context(), input.Username, input.Password, requestMeta(req))
		adminResult(w, req, http.StatusCreated, SetupResponse{Initialized: true}, err)
	})
}
