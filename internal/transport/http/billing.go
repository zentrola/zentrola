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

	// @Summary 成本单据列表
	// @Tags 成本管理
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "账期起始时间（含），UTC RFC3339 Z"
	// @Param to query string true "账期结束时间（不含），UTC RFC3339 Z"
	// @Param billingType query string false "计费类型" Enums(SUBSCRIPTION,API_KEY)
	// @Param documentType query string false "单据类型" Enums(CHARGE,ADJUSTMENT)
	// @Param currency query string false "币种" Enums(CNY,USD)
	// @Param after query int false "上一页末尾单据 ID"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[app.DocumentSummary]}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/billing/documents [get]
	r.Get("/billing/documents", func(w http.ResponseWriter, req *http.Request) {
		filter := app.DocumentFilter{Limit: 50}
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
			case "documentType":
				filter.DocumentType = value
			case "currency":
				filter.Currency = value
			case "after", "limit":
				n, err := billingPageNumber(key, value)
				if err != nil {
					securityError(w, req, err)
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
		page, err := s.Billing.Documents(req.Context(), adminFrom(req), filter)
		if err != nil {
			securityError(w, req, err)
			return
		}
		items, next := billingDocumentPage(page.Items, filter.Limit)
		adminResult(w, req, http.StatusOK, PageResponse[app.DocumentSummary]{
			Items: items, NextCursor: next, Total: page.Total,
		}, nil)
	})

	// @Summary 成本单据详情
	// @Tags 成本管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "单据 ID"
	// @Success 200 {object} response{data=app.DocumentDetail}
	// @Failure 400,401,404,503 {object} response
	// @Router /api/v1/billing/documents/{id} [get]
	r.Get("/billing/documents/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, err := routeID(req, "id")
		if err != nil || req.URL.RawQuery != "" {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		result, err := s.Billing.Document(req.Context(), adminFrom(req), id)
		adminResult(w, req, http.StatusOK, result, err)
	})

	// @Summary 待处理核算异常
	// @Tags 成本管理
	// @Description 返回 API Key 成功调用中 Token 不完整、缺少有效价格或等待按当前价格核算的记录。
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "调用起始时间（含），UTC RFC3339 Z"
	// @Param to query string true "调用结束时间（不含），UTC RFC3339 Z"
	// @Param reason query string false "异常原因" Enums(INCOMPLETE_TOKENS,MISSING_PRICE,PENDING_RATING)
	// @Param after query int false "上一页末尾用量记录 ID"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[app.UnratedUsage]}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/billing/unrated-usage [get]
	r.Get("/billing/unrated-usage", func(w http.ResponseWriter, req *http.Request) {
		filter := app.UnratedUsageFilter{Limit: 50}
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
			case "reason":
				filter.Reason = value
			case "after", "limit":
				n, err := billingPageNumber(key, value)
				if err != nil {
					securityError(w, req, err)
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
		page, err := s.Billing.Unrated(req.Context(), adminFrom(req), filter)
		if err != nil {
			securityError(w, req, err)
			return
		}
		items, next := billingUnratedPage(page.Items, filter.Limit)
		adminResult(w, req, http.StatusOK, PageResponse[app.UnratedUsage]{
			Items: items, NextCursor: next, Total: page.Total,
		}, nil)
	})
}

func billingPageNumber(name, value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 || name == "limit" && (n < 1 || n > 100) {
		return 0, appsec.ErrInvalidArgument
	}
	return n, nil
}

func billingDocumentPage(items []app.DocumentSummary, limit int32) ([]app.DocumentSummary, *string) {
	if len(items) <= int(limit) {
		return items, nil
	}
	items = items[:limit]
	next := strconv.FormatInt(items[len(items)-1].ID, 10)
	return items, &next
}

func billingUnratedPage(items []app.UnratedUsage, limit int32) ([]app.UnratedUsage, *string) {
	if len(items) <= int(limit) {
		return items, nil
	}
	items = items[:limit]
	next := strconv.FormatInt(items[len(items)-1].UsageRecordID, 10)
	return items, &next
}
