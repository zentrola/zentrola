package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// @Summary 通过授权成员 Key 获取可选的用户分组
// @Description 只返回启用分组，按 ID 倒序分页；将 nextCursor 作为下一次请求的 after。创建成员时从中选择至少一个分组。
// @Tags 成员管理
// @Security GatewayBearer
// @Param after query string false "上一页 nextCursor，默认从头查询"
// @Param limit query int false "每页数量" minimum(1) maximum(100) default(50)
// @Success 200 {object} response{data=PageResponse[appsec.ManagedGroup]}
// @Failure 400,401,403,503 {object} response
// @Router /api/v1/me/manage/groups [get]
func (s *SecurityHandlers) listManagedGroups() http.HandlerFunc {
	return listEndpoint(func(r *http.Request, p mgmt.Page) (mgmt.PageData[appsec.ManagedGroup], error) {
		limit := p.Limit
		if p.ProbeNext {
			limit++
		}
		items, total, err := s.MemberProvisioning.Groups(r.Context(), principalFrom(r), p.After, limit)
		return mgmt.PageData[appsec.ManagedGroup]{Items: items, Total: total}, err
	}, func(v appsec.ManagedGroup) int64 { return v.ID })
}

// @Summary 通过授权成员 Key 创建普通成员并签发首把 Key
// @Description groupIds 必须至少包含一个启用分组；分组不存在返回 NOT_FOUND，分组已停用返回 CONFLICT，与管理员创建成员一致。
// @Tags 成员管理
// @Security GatewayBearer
// @Param body body CreateManagedMemberRequest true "成员与首把 Key"
// @Success 201 {object} response{data=appsec.ProvisionedMember}
// @Failure 400,401,403,404,409,503 {object} response
// @Router /api/v1/me/manage/members [post]
func (s *SecurityHandlers) createManagedMember(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeRequest[CreateManagedMemberRequest](w, r)
	if !ok {
		return
	}
	groupIDs, err := requestIDs(input.GroupIDs)
	if err != nil {
		securityError(w, r, appsec.ErrInvalidArgument)
		return
	}
	created, err := s.MemberProvisioning.Create(r.Context(), principalFrom(r), input.Name, input.Remark, input.KeyName, groupIDs, requestMeta(r))
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, response{Code: "OK", Data: created})
}

// @Summary 通过授权成员 Key 为普通成员签发 Key
// @Description 可为系统内任意普通成员签发，不含操作者本人和持有管理授权的成员；不改变成员状态，已停用成员返回 MEMBER_DISABLED。
// @Tags 成员管理
// @Security GatewayBearer
// @Param id path string true "普通成员 ID"
// @Param body body IssueManagedMemberKeyRequest true "Key 名称"
// @Success 201 {object} response{data=appsec.CreatedKey}
// @Failure 400,401,403,404,409,503 {object} response
// @Router /api/v1/me/manage/members/{id}/keys [post]
func (s *SecurityHandlers) issueManagedMemberKey(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(chi.URLParam(r, "id"))
	if err != nil {
		securityError(w, r, err)
		return
	}
	input, ok := decodeRequest[IssueManagedMemberKeyRequest](w, r)
	if !ok {
		return
	}
	created, err := s.MemberProvisioning.IssueKey(r.Context(), principalFrom(r), id, input.Name, requestMeta(r))
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, response{Code: "OK", Data: created})
}

// @Summary 通过授权成员 Key 启停用普通成员
// @Description 可操作系统内任意普通成员，不含操作者本人和持有管理授权的成员。停用后该成员全部 Key 立即无法认证但不注销，重新启用后恢复可用；启用要求成员至少有一把有效 Key。
// @Tags 成员管理
// @Security GatewayBearer
// @Param id path string true "普通成员 ID"
// @Param body body UpdateManagedMemberStatusRequest true "目标状态：ACTIVE 或 DISABLED"
// @Success 200 {object} response{data=appsec.ManagedMemberStatus}
// @Failure 400,401,403,404,409,503 {object} response
// @Router /api/v1/me/manage/members/{id}/status [put]
func (s *SecurityHandlers) setManagedMemberStatus(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(chi.URLParam(r, "id"))
	if err != nil {
		securityError(w, r, err)
		return
	}
	input, ok := decodeRequest[UpdateManagedMemberStatusRequest](w, r)
	if !ok {
		return
	}
	updated, err := s.MemberProvisioning.SetStatus(r.Context(), principalFrom(r), id, input.Status, requestMeta(r))
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: updated})
}
