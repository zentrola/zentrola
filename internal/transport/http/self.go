package http

import (
	"net/http"
	"strings"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// getMyProvider 查询当前模型按网关路由将使用的服务商。
// @Summary 查询当前模型使用的服务商
// @Tags 用量统计
// @Description 根据当前模型编码，返回 Access Key 按现有权限及网关路由顺序将使用的服务商。
// @Produce json
// @Security GatewayKey
// @Security GatewayBearer
// @Param model query string true "当前模型编码"
// @Success 200 {object} response{data=gw.Provider}
// @Failure 400,401,403,404,503 {object} response
// @Header all {string} X-Request-ID "请求追踪 ID"
// @Router /api/v1/me/provider [get]
func (s *SecurityHandlers) getMyProvider(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if len(values) != 1 || len(values["model"]) != 1 {
		securityError(w, r, appsec.ErrInvalidArgument)
		return
	}
	model := strings.TrimSpace(values.Get("model"))
	handler := s.OpenAI
	if handler == nil {
		handler = s.Gateway
	}
	if handler == nil || handler.service == nil {
		securityError(w, r, gw.ErrUnavailable)
		return
	}
	result, err := handler.service.Provider(r.Context(), principalFrom(r), model)
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: result})
}
