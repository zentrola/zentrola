package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// @Summary 授权单把成员 Key 通过 MCP 管理普通成员
// @Tags 访问密钥
// @Security AdminBearer
// @Param id path string true "Access Key ID"
// @Param body body UpdateKeyMemberManagementRequest true "授权状态"
// @Success 200 {object} response{data=UpdateKeyMemberManagementRequest}
// @Router /api/v1/access-keys/{id}/member-management-permission [put]
func (s *SecurityHandlers) setKeyMemberManagementPermission(w http.ResponseWriter, r *http.Request) {
	id, err := positiveID(chi.URLParam(r, "id"))
	if err != nil {
		securityError(w, r, err)
		return
	}
	input, ok := decodeRequest[UpdateKeyMemberManagementRequest](w, r)
	if !ok {
		return
	}
	if err := s.Keys.SetMemberManagementPermission(r.Context(), adminFrom(r), id, *input.CanManageMembers, requestMeta(r)); err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: input})
}

// @Summary 通过授权成员 Key 创建普通成员并签发首把 Key
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

// @Summary 通过授权成员 Key 为普通成员签发 Key 并自动启用成员
// @Tags 成员管理
// @Security GatewayBearer
// @Param id path string true "普通成员 ID"
// @Param body body IssueManagedMemberKeyRequest true "Key 名称"
// @Success 201 {object} response{data=appsec.CreatedKey}
// @Failure 400,401,403,404,503 {object} response
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
