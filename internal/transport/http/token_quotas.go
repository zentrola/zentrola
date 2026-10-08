package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/quota"
)

func (s *SecurityHandlers) mountTokenQuotas(r chi.Router) {
	// @Summary 批量查询月度 Token 配额状态
	// @Tags 用量配额
	// @Produce json
	// @Security AdminBearer
	// @Param scopeType query string true "额度范围" Enums(PRINCIPAL,GROUP)
	// @Param scopeIds query string true "逗号分隔的用户、应用或分组 ID，最多 100 个"
	// @Success 200 {object} response{data=[]quota.Status}
	// @Failure 400,401,503 {object} response
	// @Router /api/v1/token-quotas [get]
	r.Get("/token-quotas", func(w http.ResponseWriter, req *http.Request) {
		if s.TokenQuotas == nil {
			securityError(w, req, appsec.ErrUnavailable)
			return
		}
		scopeType := quota.ScopeType(strings.TrimSpace(req.URL.Query().Get("scopeType")))
		rawIDs := strings.Split(strings.TrimSpace(req.URL.Query().Get("scopeIds")), ",")
		if len(rawIDs) == 0 || len(rawIDs) > 100 || (scopeType != quota.Principal && scopeType != quota.Group) {
			securityError(w, req, appsec.ErrInvalidArgument)
			return
		}
		ids := make([]int64, 0, len(rawIDs))
		seen := make(map[int64]struct{}, len(rawIDs))
		for _, rawID := range rawIDs {
			id, err := positiveID(strings.TrimSpace(rawID))
			if err != nil {
				securityError(w, req, appsec.ErrInvalidArgument)
				return
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		statuses, err := s.TokenQuotas.Statuses(req.Context(), scopeType, ids, time.Now().UTC())
		if err != nil {
			securityError(w, req, appsec.ErrUnavailable)
			return
		}
		writeJSON(w, req, http.StatusOK, response{Code: "OK", Data: statuses})
	})
}
