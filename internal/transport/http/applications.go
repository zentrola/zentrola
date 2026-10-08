package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

func (s *SecurityHandlers) mountApplicationRoutes(r chi.Router) {
	a := s.Applications
	// @Summary 应用列表
	// @Tags 应用管理
	// @Produce json
	// @Security AdminBearer
	// @Success 200 {object} response{data=PageResponse[mgmt.Application]}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/applications [get]
	r.Get("/applications", listEndpoint(func(req *http.Request, page mgmt.Page) (mgmt.PageData[mgmt.Application], error) {
		return a.Applications(req.Context(), adminFrom(req), page)
	}, func(value mgmt.Application) int64 { return value.ID }))
	// @Summary 搜索应用
	// @Tags 应用管理
	// @Produce json
	// @Security AdminBearer
	// @Param name query string true "应用名称片段"
	// @Success 200 {object} response{data=PageResponse[mgmt.Application]}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/applications/suggestions [get]
	r.Get("/applications/suggestions", listEndpoint(func(req *http.Request, page mgmt.Page) (mgmt.PageData[mgmt.Application], error) {
		name, err := optionalQueryValue(req, "name")
		if err != nil {
			return mgmt.PageData[mgmt.Application]{}, err
		}
		return a.Suggestions(req.Context(), adminFrom(req), page, name)
	}, func(value mgmt.Application) int64 { return value.ID }, "name"))
	// @Summary 应用详情
	// @Tags 应用管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Success 200 {object} response{data=mgmt.Application}
	// @Failure 400,401,404,503 {object} response
	// @Router /api/v1/applications/{id} [get]
	r.Get("/applications/{id}", detailEndpoint(a.Application))
	// @Summary 应用所属分组
	// @Tags 应用管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Success 200 {object} response{data=PageResponse[mgmt.Group]}
	// @Failure 400,401,404,503 {object} response
	// @Router /api/v1/applications/{id}/groups [get]
	r.Get("/applications/{id}/groups", listEndpoint(func(req *http.Request, page mgmt.Page) (mgmt.PageData[mgmt.Group], error) {
		id, err := positiveID(chi.URLParam(req, "id"))
		if err != nil {
			return mgmt.PageData[mgmt.Group]{}, err
		}
		return a.ApplicationGroups(req.Context(), adminFrom(req), id, page)
	}, func(value mgmt.Group) int64 { return value.ID }))
	// @Summary 应用 App Key 列表
	// @Tags 应用管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Success 200 {object} response{data=PageResponse[mgmt.Key]}
	// @Failure 400,401,404,503 {object} response
	// @Router /api/v1/applications/{id}/keys [get]
	r.Get("/applications/{id}/keys", listEndpoint(func(req *http.Request, page mgmt.Page) (mgmt.PageData[mgmt.Key], error) {
		id, err := positiveID(chi.URLParam(req, "id"))
		if err != nil {
			return mgmt.PageData[mgmt.Key]{}, err
		}
		return a.Keys(req.Context(), adminFrom(req), id, page)
	}, func(value mgmt.Key) int64 { return value.ID }))
	// @Summary 应用操作记录
	// @Tags 应用管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Success 200 {object} response{data=PageResponse[mgmt.Operation]}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/applications/{id}/operations [get]
	r.Get("/applications/{id}/operations", listEndpoint(func(req *http.Request, page mgmt.Page) (mgmt.PageData[mgmt.Operation], error) {
		id, err := positiveID(chi.URLParam(req, "id"))
		if err != nil {
			return mgmt.PageData[mgmt.Operation]{}, err
		}
		return a.Operations(req.Context(), adminFrom(req), id, page)
	}, func(value mgmt.Operation) int64 { return value.ID }))
	// @Summary 创建应用
	// @Tags 应用管理
	// @Description 新应用默认停用；可同时加入已启用的分组。
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param body body CreateMemberRequest true "请求参数"
	// @Success 201 {object} response{data=mgmt.Application}
	// @Failure 400,401,404,409,503 {object} response
	// @Router /api/v1/applications [post]
	r.Post("/applications", func(w http.ResponseWriter, req *http.Request) {
		input, ok := decodeRequest[CreateMemberRequest](w, req)
		if !ok {
			return
		}
		groupIDs, err := requestIDs(input.GroupIDs)
		if err != nil {
			securityError(w, req, err)
			return
		}
		value, err := a.Create(req.Context(), adminFrom(req), input.Name, input.Remark, groupIDs, requestMeta(req))
		adminResult(w, req, http.StatusCreated, value, err)
	})
	// @Summary 编辑应用
	// @Tags 应用管理
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Param body body UpdateMemberRequest true "请求参数"
	// @Success 200 {object} response{data=mgmt.Application}
	// @Failure 400,401,404,409,503 {object} response
	// @Router /api/v1/applications/{id} [put]
	r.Put("/applications/{id}", func(w http.ResponseWriter, req *http.Request) {
		input, ok := decodeRequest[UpdateMemberRequest](w, req)
		if !ok {
			return
		}
		id, err := positiveID(chi.URLParam(req, "id"))
		if err != nil {
			securityError(w, req, err)
			return
		}
		groupIDs, err := requestIDs(input.GroupIDs)
		if err != nil {
			securityError(w, req, err)
			return
		}
		value, err := a.Update(req.Context(), adminFrom(req), id, input.Name, input.Remark, groupIDs, requestMeta(req))
		adminResult(w, req, http.StatusOK, value, err)
	})
	// @Summary 增加应用月度 Token 配额
	// @Tags 应用管理
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Param body body AddTokenQuotaRequest true "增加额度和可选原因"
	// @Success 200 {object} response{data=mgmt.Application}
	// @Failure 400,401,404,503 {object} response
	// @Router /api/v1/applications/{id}/token-quota/add [post]
	r.Post("/applications/{id}/token-quota/add", func(w http.ResponseWriter, req *http.Request) {
		input, ok := decodeRequest[AddTokenQuotaRequest](w, req)
		if !ok {
			return
		}
		id, err := positiveID(chi.URLParam(req, "id"))
		if err != nil {
			securityError(w, req, err)
			return
		}
		value, err := a.AddTokenQuota(req.Context(), adminFrom(req), id, input.TokenAmount(), input.Reason, requestMeta(req))
		adminResult(w, req, http.StatusOK, value, err)
	})
	// @Summary 取消应用月度 Token 配额限制
	// @Tags 应用管理
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Success 200 {object} response{data=mgmt.Application}
	// @Failure 400,401,404,409,503 {object} response
	// @Router /api/v1/applications/{id}/token-quota [delete]
	r.Delete("/applications/{id}/token-quota", func(w http.ResponseWriter, req *http.Request) {
		id, err := positiveID(chi.URLParam(req, "id"))
		if err != nil {
			securityError(w, req, err)
			return
		}
		value, err := a.RemoveTokenQuota(req.Context(), adminFrom(req), id, requestMeta(req))
		adminResult(w, req, http.StatusOK, value, err)
	})
	// @Summary 设置应用状态
	// @Tags 应用管理
	// @Accept json
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Param body body UpdateStatusRequest true "状态"
	// @Success 200 {object} response
	// @Failure 400,401,404,409,503 {object} response
	// @Router /api/v1/applications/{id}/status [patch]
	r.Patch("/applications/{id}/status", statusEndpoint(a.SetStatus))
	// @Summary 删除应用
	// @Tags 应用管理
	// @Description 逻辑删除应用、撤销其所有 App Key 并解除分组关联。
	// @Produce json
	// @Security AdminBearer
	// @Param id path string true "应用 ID"
	// @Success 200 {object} response
	// @Failure 400,401,404,503 {object} response
	// @Router /api/v1/applications/{id} [delete]
	r.Delete("/applications/{id}", deleteEndpoint(a.Delete))
}
