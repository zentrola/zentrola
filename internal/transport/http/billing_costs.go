package http

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	app "github.com/zentrola/zentrola/internal/application/billing"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func (s *SecurityHandlers) mountBillingCosts(r chi.Router) {
	// @Summary 当月预计费用分摊
	// @Tags 成本管理
	// @Description 对当前时间范围内生效的个人订阅按成功调用 Token 预计分摊，并合并已完成核算的 API Key 实时成本；历史账期的最终归属仍使用费用统计接口。
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "起始时间（含），UTC RFC3339 Z"
	// @Param to query string true "结束时间（不含），UTC RFC3339 Z；范围必须包含当前时间"
	// @Param after query int false "归属统计偏移量" minimum(0) default(0)
	// @Param limit query int false "归属统计数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=app.Statistics}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/billing/current-attribution [get]
	r.Get("/billing/current-attribution", func(w http.ResponseWriter, req *http.Request) {
		filter, err := currentAttributionFilter(req)
		if err != nil {
			securityError(w, req, err)
			return
		}
		result, err := s.BillingCosts.CurrentAttribution(req.Context(), adminFrom(req), filter)
		adminResult(w, req, http.StatusOK, result, err)
	})

	// @Summary 用量成本汇总
	// @Tags 成本管理
	// @Description 按同一组筛选条件汇总请求、上游尝试、成功调用、Token、核算状态和分币种按量成本。订阅调用只计入共享账期数量，不伪造单次成本。
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "起始时间（含），UTC RFC3339 Z"
	// @Param to query string true "结束时间（不含），UTC RFC3339 Z"
	// @Success 200 {object} response{data=app.UsageCostSummary}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/billing/usage-costs/summary [get]
	r.Get("/billing/usage-costs/summary", func(w http.ResponseWriter, req *http.Request) {
		filter, _, err := usageCostFilter(req, "summary")
		if err != nil {
			securityError(w, req, err)
			return
		}
		result, err := s.BillingCosts.Summary(req.Context(), adminFrom(req), filter)
		adminResult(w, req, http.StatusOK, result, err)
	})

	// @Summary 单次用量成本列表
	// @Tags 成本管理
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "起始时间（含），UTC RFC3339 Z"
	// @Param to query string true "结束时间（不含），UTC RFC3339 Z"
	// @Param principalType query string false "主体类型" Enums(MEMBER,APPLICATION)
	// @Param principalId query string false "成员或应用 ID"
	// @Param groupId query string false "当前所属分组 ID"
	// @Param modelId query string false "逻辑模型 ID"
	// @Param providerId query string false "服务商 ID"
	// @Param resourceId query string false "资源 ID"
	// @Param clientProtocol query string false "客户端协议" Enums(OPENAI_CHAT,OPENAI_RESPONSES,OPENAI_IMAGES,ANTHROPIC_MESSAGES)
	// @Param status query string false "调用状态" Enums(SUCCESS,FAILED,CANCELLED)
	// @Param ratingStatus query string false "核算状态" Enums(RATED,SUBSCRIPTION_SHARED,INCOMPLETE_TOKENS,MISSING_PRICE,PENDING_RATING,NOT_BILLABLE)
	// @Param billingType query string false "计费类型" Enums(API_KEY,SUBSCRIPTION)
	// @Param currency query string false "币种" Enums(CNY,USD)
	// @Param sort query string false "排序字段" Enums(startedAt,totalCost,tokens,latency)
	// @Param order query string false "排序方向" Enums(asc,desc)
	// @Param after query int false "排行偏移量" minimum(0) default(0)
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[app.UsageCostRow]}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/billing/usage-costs [get]
	r.Get("/billing/usage-costs", func(w http.ResponseWriter, req *http.Request) {
		filter, limit, err := usageCostFilter(req, "list")
		if err != nil {
			securityError(w, req, err)
			return
		}
		page, err := s.BillingCosts.Costs(req.Context(), adminFrom(req), filter)
		if err != nil {
			securityError(w, req, err)
			return
		}
		items := page.Items
		var next *string
		if len(items) > int(limit) {
			items = items[:limit]
			value := strconv.FormatInt(filter.After+int64(limit), 10)
			next = &value
		}
		adminResult(w, req, http.StatusOK, PageResponse[app.UsageCostRow]{
			Items: items, NextCursor: next, Total: page.Total,
		}, nil)
	})

	// @Summary 导出单次用量成本
	// @Tags 成本管理
	// @Produce text/csv
	// @Security AdminBearer
	// @Success 200 {file} binary
	// @Failure 400,401,422,503 {object} response
	// @Router /api/v1/billing/usage-costs/export [get]
	r.Get("/billing/usage-costs/export", func(w http.ResponseWriter, req *http.Request) {
		filter, _, err := usageCostFilter(req, "export")
		if err != nil {
			securityError(w, req, err)
			return
		}
		rows, err := s.BillingCosts.Export(req.Context(), adminFrom(req), filter)
		if err != nil {
			securityError(w, req, err)
			return
		}
		writeUsageCostCSV(w, filter, rows)
	})

	// @Summary 单次用量成本详情
	// @Tags 成本管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "Usage Record ID"
	// @Success 200 {object} response{data=app.UsageCostDetail}
	// @Failure 400,401,404,503 {object} response
	// @Router /api/v1/billing/usage-costs/{id} [get]
	r.Get("/billing/usage-costs/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, err := routeID(req, "id")
		if err != nil || req.URL.RawQuery != "" {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		result, err := s.BillingCosts.Cost(req.Context(), adminFrom(req), id)
		adminResult(w, req, http.StatusOK, result, err)
	})
}

func currentAttributionFilter(req *http.Request) (app.CurrentAttributionFilter, error) {
	filter := app.CurrentAttributionFilter{Limit: 50}
	for key, list := range req.URL.Query() {
		if len(list) != 1 {
			return filter, appsec.ErrInvalidArgument
		}
		value := strings.TrimSpace(list[0])
		switch key {
		case "from", "to":
			parsed, err := parseUTCQueryTime(value)
			if err != nil {
				return filter, appsec.ErrInvalidArgument
			}
			if key == "from" {
				filter.From = parsed
			} else {
				filter.To = parsed
			}
		case "after", "limit":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 || key == "limit" && (n < 1 || n > 100) {
				return filter, appsec.ErrInvalidArgument
			}
			if key == "after" {
				filter.After = n
			} else {
				filter.Limit = int32(n)
			}
		default:
			return filter, appsec.ErrInvalidArgument
		}
	}
	return filter, nil
}

func usageCostFilter(req *http.Request, mode string) (app.UsageCostFilter, int32, error) {
	filter := app.UsageCostFilter{Limit: 50}
	var requestedLimit int32 = 50
	for key, list := range req.URL.Query() {
		if len(list) != 1 {
			return filter, 0, appsec.ErrInvalidArgument
		}
		value := strings.TrimSpace(list[0])
		switch key {
		case "from", "to":
			parsed, err := parseUTCQueryTime(value)
			if err != nil {
				return filter, 0, appsec.ErrInvalidArgument
			}
			if key == "from" {
				filter.From = parsed
			} else {
				filter.To = parsed
			}
		case "principalId", "groupId", "modelId", "providerId", "resourceId":
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil || id <= 0 {
				return filter, 0, appsec.ErrInvalidArgument
			}
			switch key {
			case "principalId":
				filter.PrincipalID = &id
			case "groupId":
				filter.GroupID = &id
			case "modelId":
				filter.ModelID = &id
			case "providerId":
				filter.ProviderID = &id
			case "resourceId":
				filter.ResourceID = &id
			}
		case "principalType":
			filter.PrincipalType = value
		case "clientProtocol":
			filter.ClientProtocol = value
		case "status":
			filter.Status = value
		case "ratingStatus":
			filter.RatingStatus = value
		case "billingType":
			filter.BillingType = value
		case "currency":
			filter.Currency = value
		case "sort", "order":
			if mode == "summary" {
				return filter, 0, appsec.ErrInvalidArgument
			}
			if key == "sort" {
				filter.Sort = value
			} else {
				filter.Order = value
			}
		case "after", "limit":
			if mode != "list" {
				return filter, 0, appsec.ErrInvalidArgument
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 || key == "limit" && (n < 1 || n > 100) {
				return filter, 0, appsec.ErrInvalidArgument
			}
			if key == "after" {
				filter.After = n
			} else {
				requestedLimit = int32(n)
				filter.Limit = requestedLimit
			}
		default:
			return filter, 0, appsec.ErrInvalidArgument
		}
	}
	if mode == "list" {
		filter.Limit = requestedLimit + 1
	}
	return filter, requestedLimit, nil
}

func writeUsageCostCSV(w http.ResponseWriter, filter app.UsageCostFilter, rows []app.UsageCostRow) {
	filename := "zentrola-usage-costs-" + filter.From.UTC().Format("20060102") + "-" + filter.To.UTC().Format("20060102") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{
		"usage_record_id", "request_id", "attempt_no", "started_at", "principal_type", "principal_id",
		"principal_name", "model_id", "model_name", "provider_id", "provider_name", "resource_id",
		"resource_name", "client_protocol", "status", "input_tokens", "cached_input_tokens",
		"output_tokens", "billing_type", "rating_status", "rating_revision", "currency", "total_cost",
	})
	for _, row := range rows {
		_ = writer.Write([]string{
			strconv.FormatInt(row.ID, 10), safeCSVText(row.RequestID), strconv.FormatInt(row.AttemptNo, 10),
			row.StartedAt.UTC().Format(time.RFC3339Nano), row.PrincipalType, strconv.FormatInt(row.PrincipalID, 10),
			safeCSVText(row.PrincipalName), strconv.FormatInt(row.ModelID, 10), safeCSVText(row.ModelName),
			strconv.FormatInt(row.ProviderID, 10), safeCSVText(row.ProviderName), strconv.FormatInt(row.ResourceID, 10),
			safeCSVText(row.ResourceName), row.ClientProtocol, row.Status, optionalInt64(row.InputTokens),
			optionalInt64(row.CachedInputTokens), optionalInt64(row.OutputTokens), row.BillingType, row.RatingStatus,
			optionalInt32(row.RatingRevision), optionalString(row.Currency), optionalString(row.TotalCost),
		})
	}
	writer.Flush()
}

func safeCSVText(value string) string {
	if value != "" && strings.ContainsRune("=+-@", rune(value[0])) {
		return "'" + value
	}
	return value
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalInt64(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func optionalInt32(value *int32) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(int64(*value), 10)
}
