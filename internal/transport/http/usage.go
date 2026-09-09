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

func (s *SecurityHandlers) mountUsage(r chi.Router) {
	// @Summary 首页统计
	// @Tags 用量统计
	// @Description 返回当前组织激活用户数、启用模型和服务商数量，以及指定时间范围内的 Token 汇总、用户 Token Top 10 与模型请求 Top 10。
	// @Produce json
	// @Security AdminBearer
	// @Param from query string true "起始时间 RFC3339"
	// @Param to query string true "结束时间 RFC3339"
	// @Success 200 {object} response{data=app.Dashboard}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/usage/dashboard [get]
	r.Get("/usage/dashboard", func(w http.ResponseWriter, req *http.Request) {
		values := req.URL.Query()
		if len(values) != 2 || len(values["from"]) != 1 || len(values["to"]) != 1 {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		from, fromErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(values.Get("from")))
		to, toErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(values.Get("to")))
		if fromErr != nil || toErr != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		result, err := s.Usage.Dashboard(req.Context(), adminFrom(req), from, to)
		adminResult(w, req, http.StatusOK, result, err)
	})
	// @Summary 用量记录
	// @Tags 用量统计
	// @Description 只查询当前组织；按 ID 倒序分页，最新记录在前；时间区间最长 366 天。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Param from query string false "起始时间 RFC3339，默认 to 前 24 小时"
	// @Param to query string false "结束时间 RFC3339，默认当前时间"
	// @Param memberId query string false "成员 ID"
	// @Param modelId query string false "模型 ID"
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
				t, err := time.Parse(time.RFC3339Nano, v)
				if err != nil {
					securityError(w, req, appsec.ErrInvalidArgument)
					return
				}
				if key == "from" {
					f.From = t
				} else {
					f.To = t
				}
			case "memberId", "modelId", "resourceId", "after", "limit":
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
