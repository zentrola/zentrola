// Package http 提供 HTTP 路由和中间件，不承载业务规则。
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/zentrola/zentrola/internal/infrastructure/logging"
)

type response struct {
	Code      string `json:"code"`
	Data      any    `json:"data,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"requestId"`
}

type LoginLockResponse struct {
	LockedUntil       time.Time `json:"lockedUntil"`
	RetryAfterSeconds int64     `json:"retryAfterSeconds" example:"900"`
}

type PageResponse[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
	Total      int64   `json:"total"`
}

type UpdatedResponse struct {
	Updated bool `json:"updated" example:"true"`
}

type RevokedResponse struct {
	Revoked bool `json:"revoked" example:"true"`
}

type LogoutResponse struct {
	ClearToken bool `json:"clearToken" example:"true"`
}

type LiveResponse struct {
	Status string `json:"status" example:"LIVE"`
}

// writeJSON 仅用于管理面及健康检查。未来 Gateway 不得复用此包装。
func writeJSON(w http.ResponseWriter, r *http.Request, status int, body response) {
	body.RequestID = logging.RequestID(r.Context())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
