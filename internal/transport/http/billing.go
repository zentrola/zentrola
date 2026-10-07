package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	app "github.com/zentrola/zentrola/internal/application/billing"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func (s *SecurityHandlers) mountBilling(r chi.Router) {
	// @Summary 费用统计
	// @Tags 计费账单
	// @Description 按 UTC 账期开始时间统计已确认单据；结果按计费类型、币种和用户分组，返回总成本、已分摊与未分摊成本。个人订阅调整单自动计入对应计费类型，Token 不重复累计。
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "起始时间（含），UTC RFC3339，必须以 Z 结尾"
	// @Param to query string true "结束时间（不含），UTC RFC3339，必须以 Z 结尾"
	// @Param billingType query string false "计费类型；不传表示全部" Enums(SUBSCRIPTION,API_KEY)
	// @Param after query int false "用户统计偏移量" minimum(0) default(0)
	// @Param limit query int false "用户统计数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=app.Statistics}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/billing/statistics [get]
	r.Get("/billing/statistics", func(w http.ResponseWriter, req *http.Request) {
		filter := app.StatisticsFilter{Limit: 50}
		values := req.URL.Query()
		for key, list := range values {
			if len(list) != 1 {
				securityError(w, req, appsec.ErrInvalidArgument)
				return
			}
			value := strings.TrimSpace(list[0])
			switch key {
			case "from", "to":
				parsed, err := parseUTCQueryTime(value)
				if err != nil {
					securityError(w, req, appsec.ErrInvalidArgument)
					return
				}
				if key == "from" {
					filter.From = parsed
				} else {
					filter.To = parsed
				}
			case "billingType":
				filter.BillingType = value
			case "after", "limit":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n < 0 || key == "limit" && (n < 1 || n > 100) {
					securityError(w, req, appsec.ErrInvalidArgument)
					return
				}
				if key == "after" {
					filter.After = n
				} else {
					filter.Limit = int32(n)
				}
			default:
				securityError(w, req, appsec.ErrInvalidArgument)
				return
			}
		}
		result, err := s.Billing.Statistics(req.Context(), adminFrom(req), filter)
		adminResult(w, req, http.StatusOK, result, err)
	})
}
