package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	app "github.com/zentrola/zentrola/internal/application/usage"
)

// getMyUsage 查询 Access Key 所属成员指定时间范围的 Token 用量。
// @Summary 查询我的 Token 使用量
// @Tags 用量统计
// @Description 返回 Access Key 所属成员指定时间范围内的 Token 使用量；Token 为所有真实上游调用的输入与输出 Token 之和。from 和 to 必须同时提供，均不提供时默认查询当前 UTC 自然月起至请求时刻，时间区间最长 366 天。
// @Produce json
// @Security GatewayKey
// @Security GatewayBearer
// @Param from query string false "起始时间（含），UTC RFC3339，必须以 Z 结尾并与 to 同时提供"
// @Param to query string false "结束时间（不含），UTC RFC3339，必须以 Z 结尾并与 from 同时提供"
// @Success 200 {object} response{data=app.SelfUsage}
// @Failure 400,401,503 {object} response
// @Header all {string} X-Request-ID "请求追踪 ID"
// @Router /api/v1/me/usage [get]
func (s *SecurityHandlers) getMyUsage(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := now
	values := r.URL.Query()
	if len(values) > 0 {
		if len(values) != 2 || len(values["from"]) != 1 || len(values["to"]) != 1 {
			securityError(w, r, appsec.ErrInvalidArgument)
			return
		}
		var fromErr, toErr error
		from, fromErr = parseUTCQueryTime(values.Get("from"))
		to, toErr = parseUTCQueryTime(values.Get("to"))
		if fromErr != nil || toErr != nil {
			securityError(w, r, appsec.ErrInvalidArgument)
			return
		}
	}
	result, err := s.Usage.Self(r.Context(), principalFrom(r), from, to)
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: result})
}

func (s *SecurityHandlers) mountUsage(r chi.Router) {
	// @Summary 首页统计
	// @Tags 用量统计
	// @Description 返回当前部署实例的激活用户数、启用模型和服务商数量，以及指定时间范围内的 Token 汇总、用户 Token Top 10、客户端逻辑模型请求 Top 10 与服务商调用 Top 10。
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "起始时间，UTC RFC3339，必须以 Z 结尾"
	// @Param to query string true "结束时间，UTC RFC3339，必须以 Z 结尾"
	// @Success 200 {object} response{data=app.Dashboard}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/usage/dashboard [get]
	r.Get("/usage/dashboard", func(w http.ResponseWriter, req *http.Request) {
		values := req.URL.Query()
		if len(values) != 2 || len(values["from"]) != 1 || len(values["to"]) != 1 {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		from, fromErr := parseUTCQueryTime(values.Get("from"))
		to, toErr := parseUTCQueryTime(values.Get("to"))
		if fromErr != nil || toErr != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		result, err := s.Usage.Dashboard(req.Context(), adminFrom(req), from, to)
		adminResult(w, req, http.StatusOK, result, err)
	})
	// @Summary 用量统计排行
	// @Tags 用量统计
	// @Description 按用户、客户端逻辑模型或服务商聚合指定时间范围内的请求、Token、成功率和平均耗时，并按维度主指标倒序分页。
	// @Produce json
	// @Security AdminBearer
	// @Param dimension query string true "统计维度" Enums(member,model,provider)
	// @Param after query int false "上一页 nextCursor，表示排行偏移量" minimum(0) default(0)
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Param from query string true "起始时间，UTC RFC3339，必须以 Z 结尾"
	// @Param to query string true "结束时间，UTC RFC3339，必须以 Z 结尾"
	// @Success 200 {object} response{data=PageResponse[app.StatisticRow]}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/usage/statistics [get]
	r.Get("/usage/statistics", func(w http.ResponseWriter, req *http.Request) {
		f := app.StatisticFilter{Limit: 50, ProbeNext: true}
		values := req.URL.Query()
		for key, list := range values {
			if len(list) != 1 {
				securityError(w, req, appsec.ErrInvalidArgument)
				return
			}
			value := strings.TrimSpace(list[0])
			switch key {
			case "dimension":
				f.Dimension = app.StatisticDimension(value)
			case "from", "to":
				parsed, err := parseUTCQueryTime(value)
				if err != nil {
					securityError(w, req, appsec.ErrInvalidArgument)
					return
				}
				if key == "from" {
					f.From = parsed
				} else {
					f.To = parsed
				}
			case "after", "limit":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n < 0 || (key == "limit" && (n == 0 || n > 100)) {
					securityError(w, req, appsec.ErrInvalidArgument)
					return
				}
				if key == "after" {
					f.After = n
				} else {
					f.Limit = int32(n)
				}
			default:
				securityError(w, req, appsec.ErrInvalidArgument)
				return
			}
		}
		page, err := s.Usage.Statistics(req.Context(), adminFrom(req), f)
		rows := page.Items
		var next *string
		if len(rows) > int(f.Limit) {
			rows = rows[:f.Limit]
			value := strconv.FormatInt(f.After+int64(f.Limit), 10)
			next = &value
		}
		adminResult(w, req, http.StatusOK, PageResponse[app.StatisticRow]{Items: rows, NextCursor: next, Total: page.Total}, err)
	})
	// @Summary 用量记录
	// @Tags 用量统计
	// @Description 查询当前部署实例；按 ID 倒序分页，最新记录在前；时间区间最长 366 天。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Param from query string false "起始时间，UTC RFC3339，必须以 Z 结尾；默认 to 前 24 小时"
	// @Param to query string false "结束时间，UTC RFC3339，必须以 Z 结尾；默认当前时间"
	// @Param memberId query string false "成员 ID"
	// @Param modelId query string false "模型 ID"
	// @Param providerId query string false "服务商 ID"
	// @Param resourceId query string false "资源 ID"
	// @Success 200 {object} response{data=PageResponse[app.Row]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/usage [get]
	r.Get("/usage", func(w http.ResponseWriter, req *http.Request) {
		f := app.Filter{To: time.Now().UTC(), Limit: 50}
		values := req.URL.Query()
		for key, list := range values {
			if len(list) != 1 {
				securityError(w, req, appsec.ErrInvalidArgument)
				return
			}
			v := strings.TrimSpace(list[0])
			switch key {
			case "from", "to":
				t, err := parseUTCQueryTime(v)
				if err != nil {
					securityError(w, req, appsec.ErrInvalidArgument)
					return
				}
				if key == "from" {
					f.From = t
				} else {
					f.To = t
				}
			case "memberId", "modelId", "providerId", "resourceId", "after", "limit":
				n, err := strconv.ParseInt(v, 10, 64)
				if err != nil || n < 0 || (key != "after" && n == 0) || (key == "limit" && n > 100) {
					securityError(w, req, appsec.ErrInvalidArgument)
					return
				}
				switch key {
				case "memberId":
					f.PrincipalID = &n
				case "modelId":
					f.ModelID = &n
				case "providerId":
					f.ProviderID = &n
				case "resourceId":
					f.ResourceID = &n
				case "after":
					f.After = n
				case "limit":
					f.Limit = int32(n)
				}
			default:
				securityError(w, req, appsec.ErrInvalidArgument)
				return
			}
		}
		if f.From.IsZero() {
			f.From = f.To.Add(-24 * time.Hour)
		}
		f.ProbeNext = true
		page, err := s.Usage.Query(req.Context(), adminFrom(req), f)
		rows := page.Items
		var next *string
		if len(rows) > int(f.Limit) {
			rows = rows[:f.Limit]
			n := strconv.FormatInt(rows[len(rows)-1].ID, 10)
			next = &n
		}
		adminResult(w, req, 200, PageResponse[app.Row]{Items: rows, NextCursor: next, Total: page.Total}, err)
	})
	if s.UsageWriter != nil {
		// @Summary 用量写入队列指标
		// @Tags 用量统计
		// @Produce json
		// @Security AdminBearer
		// @Success 200 {object} response{data=app.Metrics}
		// @Header all {string} X-Request-ID "请求追踪 ID"
		// @Failure 400 {object} response
		// @Failure 503 {object} response
		// @Failure 401 {object} response
		// @Failure 404 {object} response
		// @Router /api/v1/usage/writer [get]
		r.Get("/usage/writer", func(w http.ResponseWriter, req *http.Request) { adminResult(w, req, 200, s.UsageWriter.Metrics(), nil) })
	}
}

func parseUTCQueryTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, appsec.ErrInvalidArgument
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, appsec.ErrInvalidArgument
	}
	return parsed.UTC(), nil
}
