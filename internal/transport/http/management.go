package http

import (
	"github.com/go-chi/chi/v5"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"net/http"
	"strconv"
)

func (s *SecurityHandlers) mountManagement(r chi.Router) {
	m := s.Management
	// @Summary 删除成员
	// @Tags 成员管理
	// @Description 逻辑删除成员、撤销其所有 Key 并解除分组关联；保留历史用量和操作日志。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "成员 ID"
	// @Success 200 {object} response
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/members/{id} [delete]
	r.Delete("/members/{id}", func(w http.ResponseWriter, req *http.Request) {
		err := m.DeleteMember(req.Context(), adminFrom(req), routeID(req, "id"), requestMeta(req))
		adminResult(w, req, 200, map[string]bool{"deleted": true}, err)
	})
	// @Summary 删除分组
	// @Tags 分组与授权
	// @Description 逻辑删除分组，并解除其用户关系和模型授权；保留操作日志。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "分组 ID"
	// @Success 200 {object} response
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/groups/{id} [delete]
	r.Delete("/groups/{id}", func(w http.ResponseWriter, req *http.Request) {
		err := m.DeleteGroup(req.Context(), adminFrom(req), routeID(req, "id"), requestMeta(req))
		adminResult(w, req, 200, map[string]bool{"deleted": true}, err)
	})
	// @Summary 创建官方模型
	// @Tags 模型与资源
	// @Description 官方名称、编码及非空输入输出类型必填；新模型默认停用，接入映射单独配置。
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param body body mgmt.ModelInput true "模型字段"
	// @Success 201 {object} response{data=mgmt.Model}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/models [post]
	r.Post("/models", func(w http.ResponseWriter, req *http.Request) {
		var input mgmt.ModelInput
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.CreateModel(req.Context(), adminFrom(req), input, requestMeta(req))
		adminResult(w, req, 201, data, err)
	})
	// @Summary 编辑官方模型
	// @Tags 模型与资源
	// @Description 替换模型元数据，保留启停状态和 ID 关联；修改编码会改变客户端调用标识。
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "模型 ID"
	// @Param body body mgmt.ModelInput true "完整模型字段"
	// @Success 200 {object} response{data=mgmt.Model}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/models/{id} [put]
	r.Put("/models/{id}", func(w http.ResponseWriter, req *http.Request) {
		var input mgmt.ModelInput
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.UpdateModel(req.Context(), adminFrom(req), routeID(req, "id"), input, requestMeta(req))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 创建服务商
	// @Tags 模型与资源
	// @Description 新服务商默认停用；至少配置一种 HTTPS 兼容协议地址和一条模型映射，映射与服务商在同一事务创建。
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param body body mgmt.ProviderInput true "服务商字段"
	// @Success 201 {object} response{data=mgmt.Provider}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/providers [post]
	r.Post("/providers", func(w http.ResponseWriter, req *http.Request) {
		var input mgmt.ProviderInput
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.CreateProvider(req.Context(), adminFrom(req), input, requestMeta(req))
		adminResult(w, req, 201, data, err)
	})
	// @Summary 编辑服务商
	// @Tags 模型与资源
	// @Description 替换服务商接入信息和模型映射；移除的映射将停用并逻辑删除。
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "服务商 ID"
	// @Param body body mgmt.ProviderInput true "服务商字段"
	// @Success 200 {object} response{data=mgmt.Provider}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/providers/{id} [put]
	r.Put("/providers/{id}", func(w http.ResponseWriter, req *http.Request) {
		var input mgmt.ProviderInput
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.UpdateProvider(req.Context(), adminFrom(req), routeID(req, "id"), input, requestMeta(req))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 删除服务商
	// @Tags 模型与资源
	// @Description 逻辑删除服务商，并停用其模型映射和所有组织下的服务商密钥；历史用量和操作日志保留。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "服务商 ID"
	// @Success 200 {object} response
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/providers/{id} [delete]
	r.Delete("/providers/{id}", func(w http.ResponseWriter, req *http.Request) {
		err := m.DeleteProvider(req.Context(), adminFrom(req), routeID(req, "id"), requestMeta(req))
		adminResult(w, req, 200, map[string]bool{"deleted": true}, err)
	})
	// @Summary 模型详情
	// @Tags 模型与资源
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "模型 ID"
	// @Success 200 {object} response{data=mgmt.Model}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/models/{id} [get]
	r.Get("/models/{id}", func(w http.ResponseWriter, req *http.Request) {
		data, err := m.Model(req.Context(), adminFrom(req), routeID(req, "id"))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 成员列表
	// @Tags 成员管理
	// @Description 按 ID 倒序分页，最新成员在前；将 nextCursor 作为下一次请求的 after，继续查询更小的 ID。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Member]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/members [get]
	r.Get("/members", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Member, error) {
		return m.Members(req.Context(), adminFrom(req), p)
	}, func(v mgmt.Member) int64 { return v.ID }))
	// @Summary 分组列表
	// @Tags 分组与授权
	// @Description 按 ID 倒序分页，最新分组在前；将 nextCursor 作为下一次请求的 after，继续查询更小的 ID。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Param status query string false "分组状态" Enums(ACTIVE,DISABLED)
	// @Success 200 {object} response{data=PageResponse[mgmt.Group]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/groups [get]
	r.Get("/groups", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Group, error) {
		values, exists := req.URL.Query()["status"]
		if exists && len(values) != 1 {
			return nil, appsec.ErrInvalidArgument
		}
		status := ""
		if exists {
			status = values[0]
		}
		return m.GroupsByStatus(req.Context(), adminFrom(req), p, status)
	}, func(v mgmt.Group) int64 { return v.ID }))
	// @Summary 逻辑模型列表
	// @Tags 模型与资源
	// @Description 按 ID 倒序分页，最新模型在前；可通过 status 只查询启用或停用模型；将 nextCursor 作为下一次请求的 after。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Param status query string false "模型状态" Enums(ACTIVE,DISABLED)
	// @Success 200 {object} response{data=PageResponse[mgmt.Model]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/models [get]
	r.Get("/models", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Model, error) {
		values, exists := req.URL.Query()["status"]
		if exists && len(values) != 1 {
			return nil, appsec.ErrInvalidArgument
		}
		status := ""
		if exists {
			status = values[0]
		}
		return m.Models(req.Context(), adminFrom(req), p, status)
	}, func(v mgmt.Model) int64 { return v.ID }))
	// @Summary 供应商列表
	// @Tags 模型与资源
	// @Description 按 ID 倒序分页，最新供应商在前；将 nextCursor 作为下一次请求的 after，继续查询更小的 ID。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Provider]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/providers [get]
	r.Get("/providers", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Provider, error) {
		return m.Providers(req.Context(), adminFrom(req), p)
	}, func(v mgmt.Provider) int64 { return v.ID }))
	// @Summary 资源列表
	// @Tags 模型与资源
	// @Description 按 ID 倒序分页，最新资源在前；将 nextCursor 作为下一次请求的 after，继续查询更小的 ID。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Resource]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/resources [get]
	r.Get("/resources", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Resource, error) {
		return m.Resources(req.Context(), adminFrom(req), p)
	}, func(v mgmt.Resource) int64 { return v.ID }))
	// @Summary 操作日志
	// @Tags 操作日志
	// @Description 按 ID 倒序分页，最新操作在前；将 nextCursor 作为下一次请求的 after，继续查询更小的 ID。
	// @Produce json
	// @Security AdminBearer
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Operation]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/operation-logs [get]
	r.Get("/operation-logs", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Operation, error) {
		return m.Operations(req.Context(), adminFrom(req), p)
	}, func(v mgmt.Operation) int64 { return v.ID }))
	// @Summary 成员 Access Key 列表
	// @Tags 访问密钥
	// @Description 按 ID 倒序分页，最新签发的密钥在前；limit=1 获取最新一条，将 nextCursor 作为 after 查询更早的密钥。仅返回脱敏密钥，不返回完整密钥。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Key]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/members/{id}/keys [get]
	r.Get("/members/{id}/keys", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Key, error) {
		return m.Keys(req.Context(), adminFrom(req), routeID(req, "id"), p)
	}, func(v mgmt.Key) int64 { return v.ID }))
	// @Summary 用户所属分组列表
	// @Tags 成员管理
	// @Description 按分组 ID 倒序分页；用于查看和编辑用户的分组关系。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "用户 ID"
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Group]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/members/{id}/groups [get]
	r.Get("/members/{id}/groups", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Group, error) {
		return m.MemberGroups(req.Context(), adminFrom(req), routeID(req, "id"), p)
	}, func(v mgmt.Group) int64 { return v.ID }))
	// @Summary 分组成员列表
	// @Tags 分组与授权
	// @Description 按成员 ID 倒序分页；将 nextCursor 作为下一次请求的 after，继续查询更小的 ID。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Member]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/groups/{id}/members [get]
	r.Get("/groups/{id}/members", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Member, error) {
		return m.GroupMembers(req.Context(), adminFrom(req), routeID(req, "id"), p)
	}, func(v mgmt.Member) int64 { return v.ID }))
	// @Summary 分组授权模型列表
	// @Tags 分组与授权
	// @Description 按模型 ID 倒序分页；将 nextCursor 作为下一次请求的 after，继续查询更小的 ID。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Param after query string false "上一页 nextCursor，默认从头查询"
	// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
	// @Success 200 {object} response{data=PageResponse[mgmt.Model]}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/groups/{id}/models [get]
	r.Get("/groups/{id}/models", listEndpoint(func(req *http.Request, p mgmt.Page) ([]mgmt.Model, error) {
		return m.GroupModels(req.Context(), adminFrom(req), routeID(req, "id"), p)
	}, func(v mgmt.Model) int64 { return v.ID }))
	// @Summary 成员详情
	// @Tags 成员管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=mgmt.Member}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/members/{id} [get]
	r.Get("/members/{id}", func(w http.ResponseWriter, req *http.Request) {
		data, err := m.Member(req.Context(), adminFrom(req), routeID(req, "id"))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 分组详情
	// @Tags 分组与授权
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=mgmt.Group}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/groups/{id} [get]
	r.Get("/groups/{id}", func(w http.ResponseWriter, req *http.Request) {
		data, err := m.Group(req.Context(), adminFrom(req), routeID(req, "id"))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 资源详情
	// @Tags 模型与资源
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=mgmt.Resource}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Router /api/v1/resources/{id} [get]
	r.Get("/resources/{id}", func(w http.ResponseWriter, req *http.Request) {
		data, err := m.Resource(req.Context(), adminFrom(req), routeID(req, "id"))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 服务商详情
	// @Tags 模型与资源
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "服务商 ID"
	// @Success 200 {object} response{data=mgmt.ProviderDetail}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/providers/{id} [get]
	r.Get("/providers/{id}", func(w http.ResponseWriter, req *http.Request) {
		data, err := m.Provider(req.Context(), adminFrom(req), routeID(req, "id"))
		adminResult(w, req, 200, data, err)
	})

	// @Summary 创建成员
	// @Tags 成员管理
	// @Produce json
	// @Security AdminBearer
	// @Accept json
	// @Param body body CreateMemberRequest true "请求参数"
	// @Success 201 {object} response{data=mgmt.Member}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/members [post]
	r.Post("/members", func(w http.ResponseWriter, req *http.Request) {
		var input CreateMemberRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		groupIDs, err := requestIDs(input.GroupIDs)
		if err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.CreateMemberWithGroups(req.Context(), adminFrom(req), input.Name, input.Remark, groupIDs, requestMeta(req))
		adminResult(w, req, 201, data, err)
	})
	// @Summary 编辑用户
	// @Tags 成员管理
	// @Description 更新用户名称、备注和所属分组，所有变更在同一事务内生效。
	// @Produce json
	// @Security AdminBearer
	// @Accept json
	// @Param id path string true "用户 ID"
	// @Param body body UpdateMemberRequest true "请求参数"
	// @Success 200 {object} response{data=mgmt.Member}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/members/{id} [put]
	r.Put("/members/{id}", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateMemberRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		groupIDs, err := requestIDs(input.GroupIDs)
		if err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.UpdateMemberWithGroups(req.Context(), adminFrom(req), routeID(req, "id"), input.Name, input.Remark, groupIDs, requestMeta(req))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 创建分组
	// @Tags 分组与授权
	// @Produce json
	// @Security AdminBearer
	// @Accept json
	// @Param body body CreateGroupRequest true "请求参数"
	// @Success 201 {object} response{data=mgmt.Group}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/groups [post]
	r.Post("/groups", func(w http.ResponseWriter, req *http.Request) {
		var input CreateGroupRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		modelIDs, err := requestIDs(input.ModelIDs)
		if err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.CreateGroupWithModels(req.Context(), adminFrom(req), input.Code, input.Name, input.Remark, modelIDs, requestMeta(req))
		adminResult(w, req, 201, data, err)
	})
	// @Summary 编辑分组
	// @Tags 分组与授权
	// @Description 更新分组名称、备注和允许访问的模型，所有变更在同一事务内生效。
	// @Produce json
	// @Security AdminBearer
	// @Accept json
	// @Param id path string true "分组 ID"
	// @Param body body UpdateGroupRequest true "请求参数"
	// @Success 200 {object} response{data=mgmt.Group}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/groups/{id} [put]
	r.Put("/groups/{id}", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateGroupRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		modelIDs, err := requestIDs(input.ModelIDs)
		if err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.UpdateGroupWithModels(req.Context(), adminFrom(req), routeID(req, "id"), input.Name, input.Remark, modelIDs, requestMeta(req))
		adminResult(w, req, 200, data, err)
	})
	// @Summary 创建资源
	// @Tags 模型与资源
	// @Description 凭证加密保存并默认启用；服务商停用时不会参与实际调用。
	// @Produce json
	// @Security AdminBearer
	// @Accept json
	// @Param body body CreateResourceRequest true "请求参数"
	// @Success 201 {object} response{data=mgmt.Resource}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/resources [post]
	r.Post("/resources", func(w http.ResponseWriter, req *http.Request) {
		var input CreateResourceRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		data, err := m.CreateResource(req.Context(), adminFrom(req), input.ProviderID, input.Name, input.Credential, requestMeta(req))
		input.Credential = ""
		adminResult(w, req, 201, data, err)
	})
	// @Summary 更新资源凭证
	// @Tags 模型与资源
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Accept json
	// @Param body body UpdateCredentialRequest true "请求参数"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/resources/{id}/credential [put]
	r.Put("/resources/{id}/credential", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateCredentialRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		err := m.UpdateCredential(req.Context(), adminFrom(req), routeID(req, "id"), input.Credential, requestMeta(req))
		input.Credential = ""
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})
	// @Summary 测试资源连接
	// @Tags 模型与资源
	// @Description 使用已保存凭证访问上游模型列表；请求会产生外部网络调用。HTTP 200 后仍需检查 data.ok 和 data.code。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=mgmt.ConnectionResult}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/resources/{id}/test-connection [post]
	r.Post("/resources/{id}/test-connection", func(w http.ResponseWriter, req *http.Request) {
		data, err := m.TestResource(req.Context(), adminFrom(req), routeID(req, "id"), requestMeta(req))
		adminResult(w, req, 200, data, err)
	})

	// @Summary 修改分组状态
	// @Tags 分组与授权
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Accept json
	// @Param body body UpdateStatusRequest true "请求参数"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/groups/{id}/status [patch]
	r.Patch("/groups/{id}/status", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateStatusRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		err := m.SetGroupStatus(req.Context(), adminFrom(req), routeID(req, "id"), input.Status, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 修改成员状态
	// @Tags 成员管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Accept json
	// @Param body body UpdateStatusRequest true "请求参数"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/members/{id}/status [patch]
	r.Patch("/members/{id}/status", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateStatusRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		err := m.SetMemberStatus(req.Context(), adminFrom(req), routeID(req, "id"), input.Status, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 修改模型状态
	// @Tags 模型与资源
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Accept json
	// @Param body body UpdateStatusRequest true "请求参数"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/models/{id}/status [patch]
	r.Patch("/models/{id}/status", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateStatusRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		err := m.SetModelStatus(req.Context(), adminFrom(req), routeID(req, "id"), input.Status, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 修改服务商状态
	// @Tags 模型与资源
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "服务商 ID"
	// @Accept json
	// @Param body body UpdateStatusRequest true "请求参数"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Failure 400 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 503 {object} response
	// @Router /api/v1/providers/{id}/status [patch]
	r.Patch("/providers/{id}/status", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateStatusRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		err := m.SetProviderStatus(req.Context(), adminFrom(req), routeID(req, "id"), input.Status, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 修改资源状态
	// @Tags 模型与资源
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Accept json
	// @Param body body UpdateStatusRequest true "请求参数"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/resources/{id}/status [patch]
	r.Patch("/resources/{id}/status", func(w http.ResponseWriter, req *http.Request) {
		var input UpdateStatusRequest
		if err := decodeBody(w, req, &input); err != nil {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		err := m.SetResourceStatus(req.Context(), adminFrom(req), routeID(req, "id"), input.Status, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 添加分组成员
	// @Tags 分组与授权
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Param memberId path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/groups/{id}/members/{memberId} [put]
	r.Put("/groups/{id}/members/{memberId}", func(w http.ResponseWriter, req *http.Request) {
		err := m.SetGroupMember(req.Context(), adminFrom(req), routeID(req, "id"), routeID(req, "memberId"), true, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 移除分组成员
	// @Tags 分组与授权
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Param memberId path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/groups/{id}/members/{memberId} [delete]
	r.Delete("/groups/{id}/members/{memberId}", func(w http.ResponseWriter, req *http.Request) {
		err := m.SetGroupMember(req.Context(), adminFrom(req), routeID(req, "id"), routeID(req, "memberId"), false, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 授予分组模型权限
	// @Tags 分组与授权
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Param modelId path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/groups/{id}/models/{modelId} [put]
	r.Put("/groups/{id}/models/{modelId}", func(w http.ResponseWriter, req *http.Request) {
		err := m.SetGroupModel(req.Context(), adminFrom(req), routeID(req, "id"), routeID(req, "modelId"), true, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

	// @Summary 撤销分组模型权限
	// @Tags 分组与授权
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "业务 ID（正整数字符串）"
	// @Param modelId path string true "业务 ID（正整数字符串）"
	// @Success 200 {object} response{data=UpdatedResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 400 {object} response
	// @Failure 503 {object} response
	// @Failure 401 {object} response
	// @Failure 404 {object} response
	// @Failure 409 {object} response
	// @Failure 422 {object} response
	// @Router /api/v1/groups/{id}/models/{modelId} [delete]
	r.Delete("/groups/{id}/models/{modelId}", func(w http.ResponseWriter, req *http.Request) {
		err := m.SetGroupModel(req.Context(), adminFrom(req), routeID(req, "id"), routeID(req, "modelId"), false, requestMeta(req))
		adminResult(w, req, 200, UpdatedResponse{Updated: true}, err)
	})

}
func routeID(r *http.Request, name string) int64 {
	id, err := positiveID(chi.URLParam(r, name))
	if err != nil {
		return 0
	}
	return id
}
func requestIDs(values []string) ([]int64, error) {
	ids := make([]int64, len(values))
	for index, value := range values {
		id, err := positiveID(value)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}
func adminResult(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, status, response{Code: "OK", Data: data})
}
func listEndpoint[T any](load func(*http.Request, mgmt.Page) ([]T, error), id func(T) int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := mgmt.Page{Limit: 50}
		values := r.URL.Query()
		for _, name := range []string{"after", "limit"} {
			if list, ok := values[name]; ok {
				if len(list) != 1 {
					securityError(w, r, appsec.ErrInvalidArgument)
					return
				}
				n, err := strconv.ParseInt(list[0], 10, 64)
				if err != nil || n < 0 || (name == "limit" && (n < 1 || n > 100)) {
					securityError(w, r, appsec.ErrInvalidArgument)
					return
				}
				if name == "after" {
					p.After = n
				} else {
					p.Limit = int32(n)
				}
			}
		}
		data, err := load(r, p)
		if err != nil {
			securityError(w, r, err)
			return
		}
		var next *string
		if len(data) == int(p.Limit) {
			v := strconv.FormatInt(id(data[len(data)-1]), 10)
			next = &v
		}
		adminResult(w, r, 200, PageResponse[T]{Items: data, NextCursor: next}, nil)
	}
}
